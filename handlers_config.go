package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"

	"wuzapi/pkg/chatwoot"
)

// ChatwootConfigRequest represents the request body for Chatwoot configuration
type ChatwootConfigRequest struct {
	AccountID           string `json:"account_id"`
	Token               string `json:"token"`
	URL                 string `json:"url"`
	NameInbox           string `json:"name_inbox,omitempty"`
	Enabled             bool   `json:"enabled"`
	AutoCreate          bool   `json:"auto_create,omitempty"`
	SignMsg             bool   `json:"sign_msg,omitempty"`
	SignDelimiter       string `json:"sign_delimiter,omitempty"`
	ReopenConversation  bool   `json:"reopen_conversation,omitempty"`
	ConversationPending bool   `json:"conversation_pending,omitempty"`
	MergeBrazilContacts bool   `json:"merge_brazil_contacts,omitempty"`
	Organization        string `json:"organization,omitempty"`
	Logo                string `json:"logo,omitempty"`
}

// ChatwootConfigResponse represents the response for Chatwoot configuration
type ChatwootConfigResponse struct {
	UserID              string `json:"user_id"`
	AccountID           string `json:"account_id"`
	Token               string `json:"token"` // Will be masked
	URL                 string `json:"url"`
	InboxID             *int64 `json:"inbox_id,omitempty"`
	NameInbox           string `json:"name_inbox"`
	Enabled             bool   `json:"enabled"`
	AutoCreate          bool   `json:"auto_create"`
	SignMsg             bool   `json:"sign_msg"`
	SignDelimiter       string `json:"sign_delimiter,omitempty"`
	ReopenConversation  bool   `json:"reopen_conversation"`
	ConversationPending bool   `json:"conversation_pending"`
	MergeBrazilContacts bool   `json:"merge_brazil_contacts"`
	Organization        string `json:"organization,omitempty"`
	Logo                string `json:"logo,omitempty"`
	WebhookURL          string `json:"webhook_url"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
}

// GetChatwootConfig retrieves the current Chatwoot configuration
func (s *server) GetChatwootConfig() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.Context().Value("userinfo").(Values).Get("Id")
		token := r.Context().Value("userinfo").(Values).Get("Token")

		var config chatwoot.Config
		query := `SELECT * FROM chatwoot_config WHERE user_id = $1`
		if s.db.DriverName() == "sqlite" {
			query = strings.Replace(query, "$1", "?", 1)
		}

		err := s.db.Get(&config, query, userID)
		if err != nil {
			if err == sql.ErrNoRows {
				s.Respond(w, r, http.StatusNotFound, "Chatwoot not configured")
				return
			}
			log.Error().Err(err).Str("user_id", userID).Msg("Failed to get Chatwoot config")
			s.Respond(w, r, http.StatusInternalServerError, "Database error")
			return
		}

		// Build webhook URL
		baseURL := s.getBaseURL(r)
		webhookURL := fmt.Sprintf("%s/chatwoot/webhook/%s", baseURL, token)

		// Mask token for security (show only last 4 chars)
		maskedToken := config.Token
		if len(maskedToken) > 4 {
			maskedToken = "****" + maskedToken[len(maskedToken)-4:]
		}

		var inboxID *int64
		if config.InboxID.Valid {
			inboxID = &config.InboxID.Int64
		}

		response := ChatwootConfigResponse{
			UserID:              config.UserID,
			AccountID:           config.AccountID,
			Token:               maskedToken,
			URL:                 config.URL,
			InboxID:             inboxID,
			NameInbox:           config.NameInbox,
			Enabled:             config.Enabled,
			AutoCreate:          config.AutoCreate,
			SignMsg:             config.SignMsg,
			SignDelimiter:       config.SignDelimiter,
			ReopenConversation:  config.ReopenConversation,
			ConversationPending: config.ConversationPending,
			MergeBrazilContacts: config.MergeBrazilContacts,
			Organization:        config.Organization,
			Logo:                config.Logo,
			WebhookURL:          webhookURL,
			CreatedAt:           config.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:           config.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}

		s.RespondJSON(w, r, http.StatusOK, response)
	}
}

// SetChatwootConfig creates or updates Chatwoot configuration
func (s *server) SetChatwootConfig() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.Context().Value("userinfo").(Values).Get("Id")
		token := r.Context().Value("userinfo").(Values).Get("Token")

		var req ChatwootConfigRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Error().Err(err).Msg("Failed to decode Chatwoot config request")
			s.Respond(w, r, http.StatusBadRequest, "Invalid request body")
			return
		}

		// Validate required fields
		if req.AccountID == "" || req.Token == "" || req.URL == "" {
			s.Respond(w, r, http.StatusBadRequest, "account_id, token, and url are required")
			return
		}

		// Set defaults
		if req.NameInbox == "" {
			req.NameInbox = "Wuzapi Inbox"
		}
		if req.SignDelimiter == "" {
			req.SignDelimiter = "\\n"
		}

		log.Info().
			Str("user_id", userID).
			Str("account_id", req.AccountID).
			Str("url", req.URL).
			Bool("auto_create", req.AutoCreate).
			Msg("Saving Chatwoot configuration")

		// Check if config already exists
		var existingConfig chatwoot.Config
		checkQuery := `SELECT * FROM chatwoot_config WHERE user_id = $1`
		if s.db.DriverName() == "sqlite" {
			checkQuery = strings.Replace(checkQuery, "$1", "?", 1)
		}

		err := s.db.Get(&existingConfig, checkQuery, userID)
		configExists := err == nil

		var inboxID sql.NullInt64

		// Auto-create inbox if requested and not already created
		if req.AutoCreate && (!configExists || !existingConfig.InboxID.Valid) {
			log.Info().Str("user_id", userID).Msg("Auto-creating Chatwoot inbox")

			// Build webhook URL
			baseURL := s.getBaseURL(r)
			webhookURL := fmt.Sprintf("%s/chatwoot/webhook/%s", baseURL, token)

			// Create temporary config for auto-setup
			tempConfig := &chatwoot.Config{
				UserID:       userID,
				AccountID:    req.AccountID,
				Token:        req.Token,
				URL:          req.URL,
				NameInbox:    req.NameInbox,
				Organization: req.Organization,
				Logo:         req.Logo,
			}

			// Initialize service and create inbox
			cwService := chatwoot.NewService(s.db)
			createdInboxID, err := cwService.InitializeInbox(tempConfig, webhookURL)
			if err != nil {
				log.Error().Err(err).Msg("Failed to auto-create Chatwoot inbox")
				s.Respond(w, r, http.StatusInternalServerError, fmt.Sprintf("Failed to create inbox: %v", err))
				return
			}

			inboxID = sql.NullInt64{Int64: int64(createdInboxID), Valid: true}
			log.Info().Int("inbox_id", createdInboxID).Msg("Chatwoot inbox created successfully")
		} else if configExists {
			inboxID = existingConfig.InboxID
		}

		// Save or update configuration
		if configExists {
			// Update existing config
			updateQuery := `UPDATE chatwoot_config SET 
				account_id = $2, 
				token = $3, 
				url = $4, 
				inbox_id = $5, 
				name_inbox = $6, 
				enabled = $7, 
				auto_create = $8, 
				sign_msg = $9, 
				sign_delimiter = $10, 
				reopen_conversation = $11, 
				conversation_pending = $12, 
				merge_brazil_contacts = $13, 
				organization = $14, 
				logo = $15, 
				updated_at = CURRENT_TIMESTAMP 
				WHERE user_id = $1`

			if s.db.DriverName() == "sqlite" {
				for i := 1; i <= 15; i++ {
					updateQuery = strings.Replace(updateQuery, fmt.Sprintf("$%d", i), "?", 1)
				}
			}

			_, err = s.db.Exec(updateQuery,
				userID,
				req.AccountID,
				req.Token,
				req.URL,
				inboxID,
				req.NameInbox,
				req.Enabled,
				req.AutoCreate,
				req.SignMsg,
				req.SignDelimiter,
				req.ReopenConversation,
				req.ConversationPending,
				req.MergeBrazilContacts,
				req.Organization,
				req.Logo,
			)
		} else {
			// Insert new config
			insertQuery := `INSERT INTO chatwoot_config 
				(user_id, account_id, token, url, inbox_id, name_inbox, enabled, auto_create, 
				sign_msg, sign_delimiter, reopen_conversation, conversation_pending, 
				merge_brazil_contacts, organization, logo) 
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`

			if s.db.DriverName() == "sqlite" {
				for i := 1; i <= 15; i++ {
					insertQuery = strings.Replace(insertQuery, fmt.Sprintf("$%d", i), "?", 1)
				}
			}

			_, err = s.db.Exec(insertQuery,
				userID,
				req.AccountID,
				req.Token,
				req.URL,
				inboxID,
				req.NameInbox,
				req.Enabled,
				req.AutoCreate,
				req.SignMsg,
				req.SignDelimiter,
				req.ReopenConversation,
				req.ConversationPending,
				req.MergeBrazilContacts,
				req.Organization,
				req.Logo,
			)
		}

		if err != nil {
			log.Error().Err(err).Msg("Failed to save Chatwoot config")
			s.Respond(w, r, http.StatusInternalServerError, "Failed to save configuration")
			return
		}

		log.Info().Str("user_id", userID).Msg("Chatwoot configuration saved successfully")

		// Return success with inbox_id if created
		response := map[string]interface{}{
			"status":  "success",
			"message": "Chatwoot configuration saved successfully",
		}
		if inboxID.Valid {
			response["inbox_id"] = inboxID.Int64
		}

		s.RespondJSON(w, r, http.StatusOK, response)
	}
}

// DeleteChatwootConfig removes Chatwoot configuration
func (s *server) DeleteChatwootConfig() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.Context().Value("userinfo").(Values).Get("Id")

		log.Info().Str("user_id", userID).Msg("Deleting Chatwoot configuration")

		deleteQuery := `DELETE FROM chatwoot_config WHERE user_id = $1`
		if s.db.DriverName() == "sqlite" {
			deleteQuery = strings.Replace(deleteQuery, "$1", "?", 1)
		}

		result, err := s.db.Exec(deleteQuery, userID)
		if err != nil {
			log.Error().Err(err).Msg("Failed to delete Chatwoot config")
			s.Respond(w, r, http.StatusInternalServerError, "Failed to delete configuration")
			return
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			s.Respond(w, r, http.StatusNotFound, "Configuration not found")
			return
		}

		log.Info().Str("user_id", userID).Msg("Chatwoot configuration deleted successfully")
		s.RespondJSON(w, r, http.StatusOK, map[string]string{
			"status":  "success",
			"message": "Chatwoot configuration deleted successfully",
		})
	}
}

// getBaseURL extracts the base URL from the request
func (s *server) getBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}

	host := r.Host
	if forwardedHost := r.Header.Get("X-Forwarded-Host"); forwardedHost != "" {
		host = forwardedHost
	}

	return fmt.Sprintf("%s://%s", scheme, host)
}
