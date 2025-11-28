package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// ChatwootWebhookPayload represents the incoming webhook from Chatwoot
type ChatwootWebhookPayload struct {
	Event        string                 `json:"event"`
	MessageType  string                 `json:"message_type"`
	ID           int                    `json:"id"`
	Content      string                 `json:"content"`
	Private      bool                   `json:"private"`
	ContentAttrs map[string]interface{} `json:"content_attributes"`
	Conversation struct {
		ID     int    `json:"id"`
		Status string `json:"status"`
		Meta   struct {
			Sender struct {
				Identifier  string `json:"identifier"`
				PhoneNumber string `json:"phone_number"`
			} `json:"sender"`
		} `json:"meta"`
		Messages []struct {
			ID          int    `json:"id"`
			SourceID    string `json:"source_id"`
			Attachments []struct {
				DataURL  string `json:"data_url"`
				FileType string `json:"file_type"`
			} `json:"attachments"`
		} `json:"messages"`
	} `json:"conversation"`
	Inbox struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"inbox"`
	Sender struct {
		Name          string `json:"name"`
		AvailableName string `json:"available_name"`
	} `json:"sender"`
}

// HandleChatwootWebhook processes incoming webhooks from Chatwoot
func (s *server) HandleChatwootWebhook() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Extract and validate token from query or path
		vars := mux.Vars(r)
		token := vars["token"]

		if token == "" {
			token = r.URL.Query().Get("token")
		}

		if token == "" {
			log.Warn().Msg("Chatwoot webhook called without token")
			s.Respond(w, r, http.StatusUnauthorized, "Missing token")
			return
		}

		// 2. Find user by token
		var userID string
		userinfo, found := userinfocache.Get(token)
		if !found {
			// Try database
			err := s.db.Get(&userID, "SELECT id FROM users WHERE token=$1 LIMIT 1", token)
			if err != nil {
				log.Warn().Str("token", token).Msg("Chatwoot webhook: user not found")
				s.Respond(w, r, http.StatusUnauthorized, "Invalid token")
				return
			}
		} else {
			userID = userinfo.(Values).Get("Id")
		}

		// 3. Parse webhook payload
		var payload ChatwootWebhookPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			log.Error().Err(err).Msg("Failed to parse Chatwoot webhook payload")
			s.Respond(w, r, http.StatusBadRequest, "Invalid payload")
			return
		}

		log.Info().
			Str("user_id", userID).
			Str("event", payload.Event).
			Str("message_type", payload.MessageType).
			Int("conversation_id", payload.Conversation.ID).
			Msg("Chatwoot webhook received")

		// 4. Filter events - only process outgoing messages from agents
		if payload.Event != "message_created" {
			log.Debug().Str("event", payload.Event).Msg("Ignoring non-message_created event")
			s.RespondJSON(w, r, http.StatusOK, map[string]string{"status": "ignored", "reason": "not message_created"})
			return
		}

		if payload.MessageType != "outgoing" {
			log.Debug().Str("message_type", payload.MessageType).Msg("Ignoring non-outgoing message")
			s.RespondJSON(w, r, http.StatusOK, map[string]string{"status": "ignored", "reason": "not outgoing"})
			return
		}

		if payload.Private {
			log.Debug().Msg("Ignoring private note")
			s.RespondJSON(w, r, http.StatusOK, map[string]string{"status": "ignored", "reason": "private note"})
			return
		}

		// 5. Prevent loop - check if message came from Wuzapi
		if len(payload.Conversation.Messages) > 0 {
			firstMsg := payload.Conversation.Messages[0]
			if strings.HasPrefix(firstMsg.SourceID, "WAID:") && firstMsg.ID == payload.ID {
				log.Debug().Int("message_id", payload.ID).Msg("Ignoring message sent by Wuzapi (loop prevention)")
				s.RespondJSON(w, r, http.StatusOK, map[string]string{"status": "ignored", "reason": "loop prevention"})
				return
			}
		}

		// 6. Extract destination phone number
		chatID := payload.Conversation.Meta.Sender.Identifier
		if chatID == "" {
			chatID = payload.Conversation.Meta.Sender.PhoneNumber
			chatID = strings.TrimPrefix(chatID, "+")
		} else {
			// If identifier is already a JID, extract phone number
			chatID = strings.Split(chatID, "@")[0]
		}

		if chatID == "" {
			log.Error().Msg("Could not extract destination phone from Chatwoot webhook")
			s.RespondJSON(w, r, http.StatusBadRequest, map[string]string{"status": "error", "reason": "no destination"})
			return
		}

		// 7. Convert to WhatsApp JID
		recipientJID := types.NewJID(chatID, types.DefaultUserServer)

		log.Info().
			Str("user_id", userID).
			Str("recipient_jid", recipientJID.String()).
			Str("content", payload.Content).
			Msg("Sending message from Chatwoot to WhatsApp")

		// 8. Send message to WhatsApp (in goroutine to not block Chatwoot)
		go func() {
			waClient := clientManager.GetWhatsmeowClient(userID)
			if waClient == nil {
				log.Error().Str("user_id", userID).Msg("WhatsApp client not found")
				return
			}

			if !waClient.IsLoggedIn() {
				log.Error().Str("user_id", userID).Msg("WhatsApp client not logged in")
				return
			}

			// Check for attachments
			if len(payload.Conversation.Messages) > 0 {
				for _, msg := range payload.Conversation.Messages {
					if msg.ID == payload.ID && len(msg.Attachments) > 0 {
						// Has attachments - send media
						for _, attachment := range msg.Attachments {
							log.Info().
								Str("attachment_url", attachment.DataURL).
								Str("file_type", attachment.FileType).
								Msg("Sending media from Chatwoot to WhatsApp")

							// TODO: Download attachment.DataURL and send as media
							// For now, send the URL as text
							caption := payload.Content
							if caption == "" {
								caption = attachment.DataURL
							} else {
								caption = fmt.Sprintf("%s\n\n%s", payload.Content, attachment.DataURL)
							}

							_, err := waClient.SendMessage(r.Context(), recipientJID, &waE2E.Message{
								Conversation: proto.String(caption),
							})

							if err != nil {
								log.Error().Err(err).Msg("Failed to send media message to WhatsApp")
							}
						}
						return
					}
				}
			}

			// Send text message
			if payload.Content != "" {
				_, err := waClient.SendMessage(r.Context(), recipientJID, &waE2E.Message{
					Conversation: proto.String(payload.Content),
				})

				if err != nil {
					log.Error().Err(err).Msg("Failed to send text message to WhatsApp")
					return
				}

				log.Info().
					Str("recipient_jid", recipientJID.String()).
					Int("chatwoot_message_id", payload.ID).
					Msg("Message sent from Chatwoot to WhatsApp successfully")
			}
		}()

		// 9. Return success immediately (don't wait for WhatsApp send)
		s.RespondJSON(w, r, http.StatusOK, map[string]string{"status": "success"})
	}
}

// RespondJSON sends a JSON response
func (s *server) RespondJSON(w http.ResponseWriter, r *http.Request, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}
