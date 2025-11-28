package chatwoot

import (
	"database/sql"
	"time"
)

// Config represents the Chatwoot integration configuration for a user
type Config struct {
	// Database fields
	UserID                string         `db:"user_id" json:"user_id"`
	AccountID             string         `db:"account_id" json:"account_id"`
	Token                 string         `db:"token" json:"token"`
	URL                   string         `db:"url" json:"url"`
	InboxID               sql.NullInt64  `db:"inbox_id" json:"inbox_id,omitempty"`
	NameInbox             string         `db:"name_inbox" json:"name_inbox"`
	
	// Feature flags
	Enabled               bool           `db:"enabled" json:"enabled"`
	AutoCreate            bool           `db:"auto_create" json:"auto_create"`
	SignMsg               bool           `db:"sign_msg" json:"sign_msg"`
	ReopenConversation    bool           `db:"reopen_conversation" json:"reopen_conversation"`
	ConversationPending   bool           `db:"conversation_pending" json:"conversation_pending"`
	MergeBrazilContacts   bool           `db:"merge_brazil_contacts" json:"merge_brazil_contacts"`
	
	// Customization
	SignDelimiter         string         `db:"sign_delimiter" json:"sign_delimiter"`
	Organization          string         `db:"organization" json:"organization"`
	Logo                  string         `db:"logo" json:"logo"`
	
	// Metadata
	CreatedAt             time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt             time.Time      `db:"updated_at" json:"updated_at"`
}

// ConversationCache represents a cached Chatwoot conversation mapping
type ConversationCache struct {
	ID                    int64          `db:"id" json:"id"`
	UserID                string         `db:"user_id" json:"user_id"`
	ChatJID               string         `db:"chat_jid" json:"chat_jid"`
	ChatwootConversationID int64         `db:"chatwoot_conversation_id" json:"chatwoot_conversation_id"`
	ChatwootContactID     int64          `db:"chatwoot_contact_id" json:"chatwoot_contact_id"`
	ChatwootInboxID       int64          `db:"chatwoot_inbox_id" json:"chatwoot_inbox_id"`
	CreatedAt             time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt             time.Time      `db:"updated_at" json:"updated_at"`
}

// MessageMapping represents the mapping between WhatsApp and Chatwoot messages
type MessageMapping struct {
	ID                    int64          `db:"id" json:"id"`
	UserID                string         `db:"user_id" json:"user_id"`
	MessageID             string         `db:"message_id" json:"message_id"`
	ChatwootMessageID     int64          `db:"chatwoot_message_id" json:"chatwoot_message_id"`
	ChatwootConversationID int64         `db:"chatwoot_conversation_id" json:"chatwoot_conversation_id"`
	CreatedAt             time.Time      `db:"created_at" json:"created_at"`
}

// CreateConfigRequest represents the API request to create/update Chatwoot config
type CreateConfigRequest struct {
	AccountID             string `json:"account_id" binding:"required"`
	Token                 string `json:"token" binding:"required"`
	URL                   string `json:"url" binding:"required"`
	NameInbox             string `json:"name_inbox,omitempty"`
	Enabled               bool   `json:"enabled"`
	AutoCreate            bool   `json:"auto_create,omitempty"`
	SignMsg               bool   `json:"sign_msg,omitempty"`
	SignDelimiter         string `json:"sign_delimiter,omitempty"`
	ReopenConversation    bool   `json:"reopen_conversation,omitempty"`
	ConversationPending   bool   `json:"conversation_pending,omitempty"`
	MergeBrazilContacts   bool   `json:"merge_brazil_contacts,omitempty"`
	Organization          string `json:"organization,omitempty"`
	Logo                  string `json:"logo,omitempty"`
}

// ConfigResponse represents the API response for Chatwoot config
type ConfigResponse struct {
	UserID                string `json:"user_id"`
	AccountID             string `json:"account_id"`
	URL                   string `json:"url"`
	InboxID               *int64 `json:"inbox_id,omitempty"`
	NameInbox             string `json:"name_inbox"`
	Enabled               bool   `json:"enabled"`
	AutoCreate            bool   `json:"auto_create"`
	SignMsg               bool   `json:"sign_msg"`
	SignDelimiter         string `json:"sign_delimiter,omitempty"`
	ReopenConversation    bool   `json:"reopen_conversation"`
	ConversationPending   bool   `json:"conversation_pending"`
	MergeBrazilContacts   bool   `json:"merge_brazil_contacts"`
	Organization          string `json:"organization,omitempty"`
	Logo                  string `json:"logo,omitempty"`
	WebhookURL            string `json:"webhook_url"`
	CreatedAt             string `json:"created_at"`
	UpdatedAt             string `json:"updated_at"`
}
