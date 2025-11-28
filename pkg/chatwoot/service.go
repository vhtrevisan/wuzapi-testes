package chatwoot

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
)

// Service manages the business logic between WhatsApp and Chatwoot
type Service struct {
	db                *sqlx.DB
	dedupeCache       sync.Map
	conversationCache sync.Map // Cache for conversation lookups
}

// NewService creates a new Chatwoot service instance
func NewService(db *sqlx.DB) *Service {
	service := &Service{
		db:                db,
		dedupeCache:       sync.Map{},
		conversationCache: sync.Map{},
	}

	// Start background cleanup goroutine for dedupe cache
	go service.cleanupDedupeCache()

	return service
}

// cleanupDedupeCache removes old entries from the dedupe cache every 10 minutes
func (s *Service) cleanupDedupeCache() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		s.dedupeCache.Range(func(key, value interface{}) bool {
			if timestamp, ok := value.(time.Time); ok {
				// Remove entries older than 30 minutes
				if now.Sub(timestamp) > 30*time.Minute {
					s.dedupeCache.Delete(key)
					log.Debug().Str("message_id", key.(string)).Msg("Removed old message from dedupe cache")
				}
			}
			return true
		})
	}
}

// InitializeInbox creates a new inbox in Chatwoot and sets up the bot contact
func (s *Service) InitializeInbox(config *Config, webhookURL string) (int, error) {
	client := NewClient(config)

	// 1. Create inbox
	log.Info().
		Str("name", config.NameInbox).
		Str("webhook_url", webhookURL).
		Msg("Creating Chatwoot inbox")

	inboxID, err := client.CreateInbox(config.NameInbox, webhookURL)
	if err != nil {
		return 0, fmt.Errorf("failed to create inbox: %w", err)
	}

	log.Info().Int("inbox_id", inboxID).Msg("Chatwoot inbox created successfully")

	// 2. Create bot contact (identifier: 123456)
	organization := config.Organization
	if organization == "" {
		organization = "Wuzapi"
	}

	logo := config.Logo
	if logo == "" {
		logo = "https://avatars.githubusercontent.com/u/70125501"
	}

	log.Info().Msg("Creating bot contact in Chatwoot")

	botContactID, err := client.CreateContact(
		inboxID,
		organization,
		"",       // no phone for bot contact
		"123456", // identifier
		logo,
	)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to create bot contact (may already exist)")
		// Don't fail the whole process if bot contact creation fails
	} else {
		log.Info().Int("contact_id", botContactID).Msg("Bot contact created")
	}

	return inboxID, nil
}

// HandleIncomingMessage processes an incoming WhatsApp message and forwards it to Chatwoot
func (s *Service) HandleIncomingMessage(userID string, evt *events.Message, waClient *whatsmeow.Client) error {
	// 1. Deduplication check
	if _, loaded := s.dedupeCache.LoadOrStore(evt.Info.ID, time.Now()); loaded {
		log.Debug().Str("message_id", evt.Info.ID).Msg("Message already processed, skipping")
		return nil
	}

	// 2. Noise filters - skip protocol messages, reactions, polls, etc.
	if s.shouldSkipMessage(evt) {
		log.Debug().
			Str("message_id", evt.Info.ID).
			Str("type", evt.Info.Type).
			Msg("Skipping noise message")
		return nil
	}

	// 3. Load Chatwoot configuration for this user
	config, err := s.getConfig(userID)
	if err != nil {
		if err == sql.ErrNoRows {
			// No Chatwoot config, silently skip
			return nil
		}
		return fmt.Errorf("failed to load chatwoot config: %w", err)
	}

	if !config.Enabled {
		log.Debug().Str("user_id", userID).Msg("Chatwoot disabled for user")
		return nil
	}

	// 4. Initialize Chatwoot client
	client := NewClient(config)

	// 5. Determine sender information
	senderJID := evt.Info.Sender.String()
	chatJID := evt.Info.Chat.String()

	if evt.Info.IsGroup {
		// For groups, contact is the participant, conversation is for the group
		senderJID = evt.Info.Sender.String()
	} else {
		// For 1-on-1, sender and chat are the same
		senderJID = chatJID
	}

	contactName := evt.Info.PushName
	if contactName == "" {
		contactName = strings.Split(senderJID, "@")[0]
	}

	// 6. Determine message type based on IsFromMe
	msgType := "incoming"
	if evt.Info.IsFromMe {
		msgType = "outgoing"
	}

	log.Info().
		Str("user_id", userID).
		Str("message_id", evt.Info.ID).
		Str("sender_jid", senderJID).
		Str("chat_jid", chatJID).
		Str("msg_type", msgType).
		Bool("is_group", evt.Info.IsGroup).
		Msg("Processing message for Chatwoot")

	// 7. Format phone to E.164
	phoneNumber := formatToE164(strings.Split(senderJID, "@")[0])

	// 8. Ensure contact exists in Chatwoot
	contactID, err := s.ensureContact(client, config, phoneNumber, contactName, senderJID)
	if err != nil {
		return fmt.Errorf("failed to ensure contact: %w", err)
	}

	// 9. Ensure conversation exists
	conversationID, err := s.ensureConversation(userID, client, config, contactID, chatJID)
	if err != nil {
		return fmt.Errorf("failed to ensure conversation: %w", err)
	}

	// 10. Extract message content and send to Chatwoot
	if err := s.sendMessageToChatwoot(client, waClient, evt, conversationID, msgType); err != nil {
		return fmt.Errorf("failed to send message to chatwoot: %w", err)
	}

	log.Info().
		Str("message_id", evt.Info.ID).
		Int("conversation_id", conversationID).
		Msg("Message successfully forwarded to Chatwoot")

	return nil
}

// shouldSkipMessage determines if a message should be skipped (noise filter)
func (s *Service) shouldSkipMessage(evt *events.Message) bool {
	// Skip protocol messages
	if evt.Message.ProtocolMessage != nil {
		return true
	}

	// Skip reactions
	if evt.Message.ReactionMessage != nil {
		return true
	}

	// Skip polls
	if evt.Message.PollCreationMessage != nil || evt.Message.PollUpdateMessage != nil {
		return true
	}

	// Skip keep in chat messages
	if evt.Message.KeepInChatMessage != nil {
		return true
	}

	// Check if message has any actual content
	hasText := evt.Message.GetConversation() != "" ||
		evt.Message.GetExtendedTextMessage().GetText() != ""

	hasMedia := evt.Message.GetImageMessage() != nil ||
		evt.Message.GetVideoMessage() != nil ||
		evt.Message.GetAudioMessage() != nil ||
		evt.Message.GetDocumentMessage() != nil ||
		evt.Message.GetStickerMessage() != nil

	if !hasText && !hasMedia {
		return true
	}

	return false
}

// getConfig retrieves the Chatwoot configuration for a user
func (s *Service) getConfig(userID string) (*Config, error) {
	var config Config
	query := `SELECT * FROM chatwoot_config WHERE user_id = $1`
	if s.db.DriverName() == "sqlite" {
		query = strings.Replace(query, "$1", "?", 1)
	}

	err := s.db.Get(&config, query, userID)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

// ensureContact ensures a contact exists in Chatwoot, creates if not found
func (s *Service) ensureContact(client *Client, config *Config, phoneNumber, name, identifier string) (int, error) {
	// Try to find existing contact
	contactID, err := client.FindContactByPhone(phoneNumber)
	if err == nil {
		log.Debug().Int("contact_id", contactID).Str("phone", phoneNumber).Msg("Contact found in Chatwoot")
		return contactID, nil
	}

	// Contact not found, create new one
	log.Info().Str("phone", phoneNumber).Str("name", name).Msg("Creating new contact in Chatwoot")

	inboxID := int(config.InboxID.Int64)
	if inboxID == 0 {
		return 0, fmt.Errorf("inbox_id not configured")
	}

	contactID, err = client.CreateContact(inboxID, name, phoneNumber, identifier, "")
	if err != nil {
		return 0, fmt.Errorf("failed to create contact: %w", err)
	}

	return contactID, nil
}

// ensureConversation ensures a conversation exists, uses cache for performance
func (s *Service) ensureConversation(userID string, client *Client, config *Config, contactID int, chatJID string) (int, error) {
	cacheKey := fmt.Sprintf("%s:%s", userID, chatJID)

	log.Debug().
		Str("user_id", userID).
		Str("chat_jid", chatJID).
		Str("cache_key", cacheKey).
		Int("contact_id", contactID).
		Msg("ensureConversation: Starting conversation lookup")

	// Check cache first
	if cached, ok := s.conversationCache.Load(cacheKey); ok {
		if convID, ok := cached.(int); ok {
			log.Info().
				Int("conversation_id", convID).
				Str("cache_key", cacheKey).
				Msg("✓ Conversation found in MEMORY cache")
			return convID, nil
		}
	}

	log.Debug().Str("cache_key", cacheKey).Msg("Conversation NOT in memory cache, checking database...")

	// Check database cache
	var conv ConversationCache
	query := `SELECT * FROM chatwoot_conversations WHERE user_id = $1 AND chat_jid = $2`
	if s.db.DriverName() == "sqlite" {
		query = strings.Replace(query, "$1", "?", 1)
		query = strings.Replace(query, "$2", "?", 1)
	}

	err := s.db.Get(&conv, query, userID, chatJID)
	if err == nil {
		// Found in database, cache it
		s.conversationCache.Store(cacheKey, int(conv.ChatwootConversationID))
		log.Info().
			Int64("conversation_id", conv.ChatwootConversationID).
			Str("cache_key", cacheKey).
			Msg("✓ Conversation found in DATABASE cache, stored in memory")
		return int(conv.ChatwootConversationID), nil
	}

	if err != sql.ErrNoRows {
		log.Error().Err(err).Str("cache_key", cacheKey).Msg("Database error during conversation lookup")
		return 0, fmt.Errorf("database error: %w", err)
	}

	// Conversation not found, create new one
	inboxID := int(config.InboxID.Int64)
	sourceID := fmt.Sprintf("wa:%s", chatJID)

	log.Warn().
		Str("user_id", userID).
		Str("chat_jid", chatJID).
		Int("contact_id", contactID).
		Int("inbox_id", inboxID).
		Str("source_id", sourceID).
		Msg("⚠ Conversation NOT found in any cache - CREATING NEW conversation in Chatwoot")

	conversationID, err := client.CreateConversation(contactID, inboxID, sourceID, config.ConversationPending)
	if err != nil {
		log.Error().
			Err(err).
			Str("source_id", sourceID).
			Int("contact_id", contactID).
			Msg("Failed to create conversation in Chatwoot API")
		return 0, fmt.Errorf("failed to create conversation: %w", err)
	}

	log.Info().
		Int("conversation_id", conversationID).
		Str("source_id", sourceID).
		Int("contact_id", contactID).
		Msg("✓ NEW conversation created in Chatwoot successfully")

	// Save to database cache
	insertQuery := `INSERT INTO chatwoot_conversations 
		(user_id, chat_jid, chatwoot_conversation_id, chatwoot_contact_id, chatwoot_inbox_id) 
		VALUES ($1, $2, $3, $4, $5)`

	if s.db.DriverName() == "sqlite" {
		insertQuery = strings.Replace(insertQuery, "$1", "?", 1)
		insertQuery = strings.Replace(insertQuery, "$2", "?", 1)
		insertQuery = strings.Replace(insertQuery, "$3", "?", 1)
		insertQuery = strings.Replace(insertQuery, "$4", "?", 1)
		insertQuery = strings.Replace(insertQuery, "$5", "?", 1)
	}

	_, err = s.db.Exec(insertQuery, userID, chatJID, conversationID, contactID, inboxID)
	if err != nil {
		log.Error().
			Err(err).
			Int("conversation_id", conversationID).
			Str("chat_jid", chatJID).
			Msg("⚠ Failed to save conversation to database cache - may cause duplicates on restart!")
		// Don't fail the whole operation if cache save fails
	} else {
		log.Debug().
			Int("conversation_id", conversationID).
			Str("chat_jid", chatJID).
			Msg("✓ Conversation saved to database cache")
	}

	// Store in memory cache
	s.conversationCache.Store(cacheKey, conversationID)
	log.Debug().
		Int("conversation_id", conversationID).
		Str("cache_key", cacheKey).
		Msg("✓ Conversation stored in memory cache")

	return conversationID, nil
}

// sendMessageToChatwoot extracts message content and sends it to Chatwoot
func (s *Service) sendMessageToChatwoot(client *Client, waClient *whatsmeow.Client, evt *events.Message, conversationID int, msgType string) error {
	sourceID := fmt.Sprintf("WAID:%s", evt.Info.ID)

	// Check for text content
	textContent := evt.Message.GetConversation()
	if textContent == "" && evt.Message.GetExtendedTextMessage() != nil {
		textContent = evt.Message.GetExtendedTextMessage().GetText()
	}

	// Check for media
	if img := evt.Message.GetImageMessage(); img != nil {
		return s.sendMediaMessage(client, waClient, evt, conversationID, msgType, sourceID, img.GetMimetype(), img.GetCaption(), "image")
	}

	if vid := evt.Message.GetVideoMessage(); vid != nil {
		return s.sendMediaMessage(client, waClient, evt, conversationID, msgType, sourceID, vid.GetMimetype(), vid.GetCaption(), "video")
	}

	if aud := evt.Message.GetAudioMessage(); aud != nil {
		return s.sendMediaMessage(client, waClient, evt, conversationID, msgType, sourceID, aud.GetMimetype(), "", "audio")
	}

	if doc := evt.Message.GetDocumentMessage(); doc != nil {
		return s.sendMediaMessage(client, waClient, evt, conversationID, msgType, sourceID, doc.GetMimetype(), doc.GetCaption(), "document")
	}

	// Send as text message
	if textContent != "" {
		_, err := client.CreateMessage(conversationID, msgType, textContent, false, sourceID)
		return err
	}

	return nil
}

// sendMediaMessage downloads media from WhatsApp and sends to Chatwoot
func (s *Service) sendMediaMessage(client *Client, waClient *whatsmeow.Client, evt *events.Message, conversationID int, msgType, sourceID, mimeType, caption, mediaType string) error {
	var downloadable whatsmeow.DownloadableMessage
	var fileName string

	switch mediaType {
	case "image":
		downloadable = evt.Message.GetImageMessage()
		fileName = fmt.Sprintf("%s.jpg", evt.Info.ID)
		if mimeType == "image/png" {
			fileName = fmt.Sprintf("%s.png", evt.Info.ID)
		}
	case "video":
		downloadable = evt.Message.GetVideoMessage()
		fileName = fmt.Sprintf("%s.mp4", evt.Info.ID)
	case "audio":
		downloadable = evt.Message.GetAudioMessage()
		fileName = fmt.Sprintf("%s.ogg", evt.Info.ID)
		if mimeType == "audio/mpeg" {
			fileName = fmt.Sprintf("%s.mp3", evt.Info.ID)
		}
	case "document":
		doc := evt.Message.GetDocumentMessage()
		downloadable = doc
		fileName = doc.GetFileName()
		if fileName == "" {
			fileName = fmt.Sprintf("%s.pdf", evt.Info.ID)
		}
	default:
		return fmt.Errorf("unsupported media type: %s", mediaType)
	}

	// Download media from WhatsApp
	log.Debug().Str("media_type", mediaType).Str("filename", fileName).Msg("Downloading media from WhatsApp")

	data, err := waClient.Download(context.Background(), downloadable)
	if err != nil {
		return fmt.Errorf("failed to download media: %w", err)
	}

	log.Info().
		Str("media_type", mediaType).
		Int("size_bytes", len(data)).
		Str("filename", fileName).
		Msg("Media downloaded, sending to Chatwoot")

	// Send to Chatwoot
	_, err = client.SendMediaMessage(conversationID, msgType, data, fileName, mimeType, caption, sourceID)
	if err != nil {
		return fmt.Errorf("failed to send media to chatwoot: %w", err)
	}

	return nil
}

// formatToE164 formats a phone number to E.164 format
func formatToE164(phone string) string {
	// Remove any existing + prefix
	phone = strings.TrimPrefix(phone, "+")

	// Remove any non-digit characters
	phone = strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, phone)

	// Add + prefix
	return "+" + phone
}
