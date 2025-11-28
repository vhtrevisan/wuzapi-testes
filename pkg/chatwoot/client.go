package chatwoot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

// Client represents a Chatwoot API client
type Client struct {
	config     *Config
	httpClient *http.Client
	baseURL    string
	accountID  string
	token      string
}

// NewClient creates a new Chatwoot API client
func NewClient(config *Config) *Client {
	return &Client{
		config:     config,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    config.URL,
		accountID:  config.AccountID,
		token:      config.Token,
	}
}

// InboxChannel represents the channel configuration for an inbox
type InboxChannel struct {
	Type       string `json:"type"`
	WebhookURL string `json:"webhook_url"`
}

// CreateInboxRequest represents the request to create an inbox
type CreateInboxRequest struct {
	Name    string       `json:"name"`
	Channel InboxChannel `json:"channel"`
}

// InboxResponse represents the Chatwoot inbox response
type InboxResponse struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	ChannelType string `json:"channel_type"`
	WebhookURL  string `json:"webhook_url"`
}

// ContactSearchResponse represents the search contact response
type ContactSearchResponse struct {
	Payload []ContactPayload `json:"payload"`
}

// ContactPayload represents a contact in the response
type ContactPayload struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	PhoneNumber string `json:"phone_number"`
	Identifier  string `json:"identifier"`
	Thumbnail   string `json:"thumbnail"`
}

// CreateContactPayloadRequest represents the contact creation request
type CreateContactPayloadRequest struct {
	InboxID     int    `json:"inbox_id"`
	Name        string `json:"name"`
	Identifier  string `json:"identifier,omitempty"`
	PhoneNumber string `json:"phone_number,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
}

// ContactResponse represents the contact creation response
type ContactResponse struct {
	Payload struct {
		Contact ContactPayload `json:"contact"`
	} `json:"payload"`
}

// ConversationRequest represents the conversation creation request
type ConversationRequest struct {
	ContactID string `json:"contact_id"`
	InboxID   string `json:"inbox_id"`
	Status    string `json:"status,omitempty"`
	SourceID  string `json:"source_id,omitempty"`
}

// ConversationResponse represents the conversation response
type ConversationResponse struct {
	ID        int    `json:"id"`
	AccountID int    `json:"account_id"`
	InboxID   int    `json:"inbox_id"`
	Status    string `json:"status"`
	ContactID int    `json:"contact_id"`
}

// MessageRequest represents the message creation request
type MessageRequest struct {
	Content           string                 `json:"content"`
	MessageType       string                 `json:"message_type"`
	Private           bool                   `json:"private"`
	SourceID          string                 `json:"source_id,omitempty"`
	ContentAttributes map[string]interface{} `json:"content_attributes,omitempty"`
}

// MessageResponse represents the message response
type MessageResponse struct {
	ID             int    `json:"id"`
	Content        string `json:"content"`
	MessageType    int    `json:"message_type"`
	CreatedAt      int64  `json:"created_at"`
	ConversationID int    `json:"conversation_id"`
	SourceID       string `json:"source_id"`
}

// ErrorResponse represents a Chatwoot API error
type ErrorResponse struct {
	Message string `json:"message"`
	Errors  []struct {
		Field   string `json:"field"`
		Message string `json:"message"`
	} `json:"errors,omitempty"`
}

// doRequest performs an HTTP request with authentication
func (c *Client) doRequest(method, path string, body interface{}) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	url := fmt.Sprintf("%s%s", c.baseURL, path)
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api_access_token", c.token)

	log.Debug().
		Str("method", method).
		Str("url", url).
		Msg("Chatwoot API request")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	return resp, nil
}

// handleError checks the response status and returns an error if needed
func (c *Client) handleError(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("HTTP %d: failed to read error response", resp.StatusCode)
	}

	var errResp ErrorResponse
	if err := json.Unmarshal(bodyBytes, &errResp); err != nil {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, errResp.Message)
}

// CreateInbox creates a new inbox in Chatwoot
func (c *Client) CreateInbox(name string, webhookURL string) (int, error) {
	request := CreateInboxRequest{
		Name: name,
		Channel: InboxChannel{
			Type:       "api",
			WebhookURL: webhookURL,
		},
	}

	path := fmt.Sprintf("/api/v1/accounts/%s/inboxes", c.accountID)
	resp, err := c.doRequest("POST", path, request)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if err := c.handleError(resp); err != nil {
		return 0, err
	}

	var inboxResp InboxResponse
	if err := json.NewDecoder(resp.Body).Decode(&inboxResp); err != nil {
		return 0, fmt.Errorf("failed to decode inbox response: %w", err)
	}

	log.Info().
		Int("inbox_id", inboxResp.ID).
		Str("name", name).
		Msg("Chatwoot inbox created")

	return inboxResp.ID, nil
}

// FindContactByPhone searches for a contact by phone number
func (c *Client) FindContactByPhone(phone string) (int, error) {
	// Ensure phone has + prefix
	if phone[0] != '+' {
		phone = "+" + phone
	}

	path := fmt.Sprintf("/api/v1/accounts/%s/contacts/search?q=%s", c.accountID, phone)
	resp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if err := c.handleError(resp); err != nil {
		return 0, err
	}

	var searchResp ContactSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return 0, fmt.Errorf("failed to decode search response: %w", err)
	}

	if len(searchResp.Payload) == 0 {
		return 0, fmt.Errorf("contact not found: %s", phone)
	}

	// Return first match
	contactID := searchResp.Payload[0].ID
	log.Debug().
		Int("contact_id", contactID).
		Str("phone", phone).
		Msg("Contact found")

	return contactID, nil
}

// CreateContact creates a new contact in Chatwoot
func (c *Client) CreateContact(inboxID int, name, phone, identifier, avatarURL string) (int, error) {
	request := CreateContactPayloadRequest{
		InboxID:    inboxID,
		Name:       name,
		Identifier: identifier,
		AvatarURL:  avatarURL,
	}

	// Only add phone_number if it's a valid phone (not a group)
	if phone != "" && !containsString(phone, "@g.us") {
		if phone[0] != '+' {
			phone = "+" + phone
		}
		request.PhoneNumber = phone
	}

	path := fmt.Sprintf("/api/v1/accounts/%s/contacts", c.accountID)
	resp, err := c.doRequest("POST", path, request)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if err := c.handleError(resp); err != nil {
		return 0, err
	}

	var contactResp ContactResponse
	if err := json.NewDecoder(resp.Body).Decode(&contactResp); err != nil {
		return 0, fmt.Errorf("failed to decode contact response: %w", err)
	}

	contactID := contactResp.Payload.Contact.ID
	log.Info().
		Int("contact_id", contactID).
		Str("name", name).
		Str("phone", phone).
		Msg("Chatwoot contact created")

	return contactID, nil
}

// CreateConversation creates a new conversation in Chatwoot
func (c *Client) CreateConversation(contactID int, inboxID int, sourceID string, pending bool) (int, error) {
	request := ConversationRequest{
		ContactID: fmt.Sprintf("%d", contactID),
		InboxID:   fmt.Sprintf("%d", inboxID),
		SourceID:  sourceID,
	}

	if pending {
		request.Status = "pending"
	}

	path := fmt.Sprintf("/api/v1/accounts/%s/conversations", c.accountID)
	resp, err := c.doRequest("POST", path, request)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if err := c.handleError(resp); err != nil {
		return 0, err
	}

	var convResp ConversationResponse
	if err := json.NewDecoder(resp.Body).Decode(&convResp); err != nil {
		return 0, fmt.Errorf("failed to decode conversation response: %w", err)
	}

	log.Info().
		Int("conversation_id", convResp.ID).
		Int("contact_id", contactID).
		Int("inbox_id", inboxID).
		Msg("Chatwoot conversation created")

	return convResp.ID, nil
}

// CreateMessage creates a new text message in a conversation
func (c *Client) CreateMessage(conversationID int, msgType string, content string, private bool, sourceID string) (int, error) {
	request := MessageRequest{
		Content:     content,
		MessageType: msgType,
		Private:     private,
		SourceID:    sourceID,
	}

	path := fmt.Sprintf("/api/v1/accounts/%s/conversations/%d/messages", c.accountID, conversationID)
	resp, err := c.doRequest("POST", path, request)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if err := c.handleError(resp); err != nil {
		return 0, err
	}

	var msgResp MessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&msgResp); err != nil {
		return 0, fmt.Errorf("failed to decode message response: %w", err)
	}

	log.Debug().
		Int("message_id", msgResp.ID).
		Int("conversation_id", conversationID).
		Str("type", msgType).
		Msg("Chatwoot message created")

	return msgResp.ID, nil
}

// SendMediaMessage sends a message with media attachment using multipart/form-data
func (c *Client) SendMediaMessage(conversationID int, msgType string, fileData []byte, fileName string, mimeType string, caption string, sourceID string) (int, error) {
	// Create multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add message_type field
	if err := writer.WriteField("message_type", msgType); err != nil {
		return 0, fmt.Errorf("failed to write message_type field: %w", err)
	}

	// Add caption if provided
	if caption != "" {
		if err := writer.WriteField("content", caption); err != nil {
			return 0, fmt.Errorf("failed to write content field: %w", err)
		}
	}

	// Add source_id if provided
	if sourceID != "" {
		if err := writer.WriteField("source_id", sourceID); err != nil {
			return 0, fmt.Errorf("failed to write source_id field: %w", err)
		}
	}

	// Add file attachment
	part, err := writer.CreateFormFile("attachments[]", fileName)
	if err != nil {
		return 0, fmt.Errorf("failed to create form file: %w", err)
	}

	if _, err := part.Write(fileData); err != nil {
		return 0, fmt.Errorf("failed to write file data: %w", err)
	}

	// Close the multipart writer
	if err := writer.Close(); err != nil {
		return 0, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/api/v1/accounts/%s/conversations/%d/messages", c.baseURL, c.accountID, conversationID)
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("api_access_token", c.token)

	log.Debug().
		Str("url", url).
		Str("filename", fileName).
		Str("mime_type", mimeType).
		Int("size_bytes", len(fileData)).
		Msg("Sending media to Chatwoot")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if err := c.handleError(resp); err != nil {
		return 0, err
	}

	var msgResp MessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&msgResp); err != nil {
		return 0, fmt.Errorf("failed to decode message response: %w", err)
	}

	log.Info().
		Int("message_id", msgResp.ID).
		Int("conversation_id", conversationID).
		Str("filename", fileName).
		Msg("Chatwoot media message sent")

	return msgResp.ID, nil
}

// Helper function to check if string contains substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			bytes.Contains([]byte(s), []byte(substr))))
}
