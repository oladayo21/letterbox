package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/oladayo21/letterbox/internal/composer"
	"github.com/oladayo21/letterbox/internal/domain"
	"github.com/oladayo21/letterbox/internal/imap"
	"github.com/oladayo21/letterbox/internal/ingest"
	"github.com/oladayo21/letterbox/internal/repository"
	"github.com/oladayo21/letterbox/internal/smtp"
	"github.com/oladayo21/letterbox/internal/storage"
)

const presignedURLExpiry = 1 * time.Hour

type MessageHandler struct {
	accountRepo    *repository.AccountRepository
	emailRepo      *repository.EmailRepository
	attachmentRepo *repository.AttachmentRepository
	ingester       *ingest.Ingester
	storage        *storage.S3Storage
}

func NewMessageHandler(
	accountRepo *repository.AccountRepository,
	emailRepo *repository.EmailRepository,
	attachmentRepo *repository.AttachmentRepository,
	ingester *ingest.Ingester,
	s3 *storage.S3Storage,
) *MessageHandler {

	return &MessageHandler{
		accountRepo:    accountRepo,
		emailRepo:      emailRepo,
		attachmentRepo: attachmentRepo,
		ingester:       ingester,
		storage:        s3,
	}
}

type messageListItem struct {
	ID        uuid.UUID             `json:"id"`
	UID       int64                 `json:"uid"`
	MessageID string                `json:"message_id,omitempty"`
	Folder    string                `json:"folder"`
	Subject   string                `json:"subject"`
	From      domain.EmailAddress   `json:"from"`
	To        []domain.EmailAddress `json:"to"`
	Date      time.Time             `json:"date"`
	Flags     []string              `json:"flags"`
}

func toMessageListItem(e *domain.Email) messageListItem {

	return messageListItem{
		ID:        e.ID,
		UID:       e.UID,
		MessageID: e.MessageID,
		Folder:    e.Folder,
		Subject:   e.Subject,
		From: domain.EmailAddress{
			Name:  e.FromName,
			Email: e.FromEmail,
		},
		To:    e.To,
		Date:  e.Date,
		Flags: e.Flags,
	}
}

type listMessagesResponse struct {
	Messages []messageListItem `json:"messages"`
	Total    int64             `json:"total"`
	Limit    int               `json:"limit"`
	Offset   int               `json:"offset"`
}

func (h *MessageHandler) ListMessages(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)

	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account ID")

		return
	}

	folderName := chi.URLParam(r, "name")

	if folderName == "" {
		writeError(w, http.StatusBadRequest, "folder name is required")

		return
	}

	// Verify account exists
	_, err = h.accountRepo.Get(r.Context(), id)

	if errors.Is(err, repository.ErrAccountNotFound) {
		writeError(w, http.StatusNotFound, "account not found")

		return
	}

	if err != nil {
		slog.Error("failed to get account", "account_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get account")

		return
	}

	// Parse query params
	limit, offset, before, after, err := parseListParams(r)

	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())

		return
	}

	filter := domain.ListEmailsFilter{
		AccountID: id,
		Folder:    folderName,
		Limit:     limit,
		Offset:    offset,
		Before:    before,
		After:     after,
	}

	emails, err := h.emailRepo.List(r.Context(), filter)

	if err != nil {
		slog.Error("failed to list emails", "account_id", id, "folder", folderName, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list messages")

		return
	}

	total, err := h.emailRepo.CountInFolder(r.Context(), id, folderName)

	if err != nil {
		slog.Error("failed to count emails", "account_id", id, "folder", folderName, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to count messages")

		return
	}

	if total == 0 && len(emails) == 0 {
		slog.Debug("folder returned zero results",
			"account_id", id, "folder", folderName)
	}

	messages := make([]messageListItem, len(emails))

	for i, e := range emails {
		messages[i] = toMessageListItem(&e)
	}

	writeJSON(w, http.StatusOK, listMessagesResponse{
		Messages: messages,
		Total:    total,
		Limit:    limit,
		Offset:   offset,
	})
}

func parseListParams(r *http.Request) (limit, offset int, before, after *time.Time, err error) {
	limit, offset, err = parsePaginationParams(r)

	if err != nil {
		return 0, 0, nil, nil, err
	}

	if beforeStr := r.URL.Query().Get("before"); beforeStr != "" {
		t, err := time.Parse(time.RFC3339, beforeStr)

		if err != nil {
			return 0, 0, nil, nil, errors.New("invalid before timestamp (use RFC3339)")
		}

		before = &t
	}

	if afterStr := r.URL.Query().Get("after"); afterStr != "" {
		t, err := time.Parse(time.RFC3339, afterStr)

		if err != nil {
			return 0, 0, nil, nil, errors.New("invalid after timestamp (use RFC3339)")
		}

		after = &t
	}

	return limit, offset, before, after, nil
}

type attachmentResponse struct {
	ID          uuid.UUID `json:"id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	Size        int64     `json:"size"`
	URL         string    `json:"url,omitempty"`
}

type messageResponse struct {
	ID          uuid.UUID             `json:"id"`
	UID         int64                 `json:"uid"`
	MessageID   string                `json:"message_id,omitempty"`
	Folder      string                `json:"folder"`
	Subject     string                `json:"subject"`
	From        domain.EmailAddress   `json:"from"`
	To          []domain.EmailAddress `json:"to"`
	CC          []domain.EmailAddress `json:"cc,omitempty"`
	Date        time.Time             `json:"date"`
	Parsed      parsedContent         `json:"parsed"`
	Raw         string                `json:"raw"`
	Attachments []attachmentResponse  `json:"attachments"`
	Flags       []string              `json:"flags"`
}

type parsedContent struct {
	Text string `json:"text"`
	HTML string `json:"html"`
}

func (h *MessageHandler) GetMessage(w http.ResponseWriter, r *http.Request) {
	accountIDStr := chi.URLParam(r, "id")
	accountID, err := uuid.Parse(accountIDStr)

	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account ID")

		return
	}

	uidStr := chi.URLParam(r, "uid")
	uid, err := strconv.ParseInt(uidStr, 10, 64)

	if err != nil || uid < 1 {
		writeError(w, http.StatusBadRequest, "invalid message UID")

		return
	}

	folderName := r.URL.Query().Get("folder")

	if folderName == "" {
		folderName = "INBOX"
	}

	// Verify account exists
	_, err = h.accountRepo.Get(r.Context(), accountID)

	if errors.Is(err, repository.ErrAccountNotFound) {
		writeError(w, http.StatusNotFound, "account not found")

		return
	}

	if err != nil {
		slog.Error("failed to get account", "account_id", accountID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get account")

		return
	}

	// Try to get from local DB first
	email, err := h.emailRepo.GetByUID(r.Context(), accountID, folderName, uid)

	if err != nil && !errors.Is(err, repository.ErrEmailNotFound) {
		slog.Error("failed to get email", "account_id", accountID, "uid", uid, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get message")

		return
	}

	// On-demand fetch if not in local DB
	if errors.Is(err, repository.ErrEmailNotFound) {
		slog.Info("fetching email on-demand", "account_id", accountID, "folder", folderName, "uid", uid)

		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()

		email, err = h.ingester.IngestEmail(ctx, accountID, folderName, uint32(uid))

		if errors.Is(err, ingest.ErrAccountNotFound) {
			writeError(w, http.StatusNotFound, "account not found")

			return
		}

		// Race condition: another request ingested this email
		if errors.Is(err, ingest.ErrEmailAlreadyExists) {
			email, err = h.emailRepo.GetByUID(r.Context(), accountID, folderName, uid)

			if err != nil {
				slog.Error("failed to get email after race", "account_id", accountID, "uid", uid, "error", err)
				writeError(w, http.StatusInternalServerError, "failed to get message")

				return
			}
		} else if errors.Is(err, imap.ErrMessageNotFound) || errors.Is(err, imap.ErrFolderNotFound) {
			writeError(w, http.StatusNotFound, "message not found")

			return
		} else if err != nil {
			slog.Error("failed to fetch email", "account_id", accountID, "uid", uid, "error", err)
			writeError(w, http.StatusBadGateway, "failed to fetch message from IMAP")

			return
		}
	}

	// Load attachments if not already loaded
	if email.Attachments == nil {
		attachments, err := h.attachmentRepo.GetByEmailID(r.Context(), email.ID)

		if err != nil {
			slog.Error("failed to get attachments", "email_id", email.ID, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to get attachments")

			return
		}

		email.Attachments = attachments
	}

	// Generate presigned URLs for attachments
	attachmentResponses := make([]attachmentResponse, len(email.Attachments))

	for i, att := range email.Attachments {
		url, err := h.storage.GeneratePresignedURL(r.Context(), att.S3Key, presignedURLExpiry)

		if err != nil {
			slog.Error("failed to generate presigned URL", "s3_key", att.S3Key, "error", err)
			url = "" // Continue without URL rather than failing
		}

		attachmentResponses[i] = attachmentResponse{
			ID:          att.ID,
			Filename:    att.Filename,
			ContentType: att.ContentType,
			Size:        att.Size,
			URL:         url,
		}
	}

	response := messageResponse{
		ID:        email.ID,
		UID:       email.UID,
		MessageID: email.MessageID,
		Folder:    email.Folder,
		Subject:   email.Subject,
		From: domain.EmailAddress{
			Name:  email.FromName,
			Email: email.FromEmail,
		},
		To:   email.To,
		CC:   email.CC,
		Date: email.Date,
		Parsed: parsedContent{
			Text: email.ParsedText,
			HTML: email.ParsedHTML,
		},
		Raw:         email.Raw,
		Attachments: attachmentResponses,
		Flags:       email.Flags,
	}

	writeJSON(w, http.StatusOK, response)
}

// sendMessageRequest represents a request to send an email.
type sendMessageRequest struct {
	To          []domain.EmailAddress `json:"to" validate:"required,min=1,dive"`
	CC          []domain.EmailAddress `json:"cc,omitempty"`
	BCC         []domain.EmailAddress `json:"bcc,omitempty"`
	Subject     string                `json:"subject" validate:"required"`
	Text        string                `json:"text,omitempty"`
	HTML        string                `json:"html,omitempty"`
	Attachments []sendAttachmentInput `json:"attachments,omitempty"`
}

type sendAttachmentInput struct {
	Filename    string `json:"filename" validate:"required"`
	ContentType string `json:"content_type" validate:"required"`
	Data        string `json:"data" validate:"required"` // Base64 encoded
	IsInline    bool   `json:"is_inline,omitempty"`
	ContentID   string `json:"content_id,omitempty"`
}

type sendMessageResponse struct {
	MessageID string `json:"message_id"`
	Success   bool   `json:"success"`
}

func (h *MessageHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	accountIDStr := chi.URLParam(r, "id")
	accountID, err := uuid.Parse(accountIDStr)

	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account ID")

		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 25<<20) // 25MB limit for attachments

	var req sendMessageRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")

		return
	}

	if err := validate.Struct(req); err != nil {
		writeError(w, http.StatusBadRequest, formatValidationError(err, sendMessageFieldNames))

		return
	}

	// Require at least text or html
	if req.Text == "" && req.HTML == "" {
		writeError(w, http.StatusBadRequest, "at least text or html body is required")

		return
	}

	// Get account with SMTP config
	account, err := h.accountRepo.Get(r.Context(), accountID)

	if errors.Is(err, repository.ErrAccountNotFound) {
		writeError(w, http.StatusNotFound, "account not found")

		return
	}

	if err != nil {
		slog.Error("failed to get account", "account_id", accountID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get account")

		return
	}

	// Check SMTP is configured
	if account.SmtpHost == "" {
		writeError(w, http.StatusBadRequest, "SMTP not configured for this account")

		return
	}

	// Build attachments
	attachments, err := parseAttachments(req.Attachments)

	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())

		return
	}

	// Compose the email
	email := composer.ComposeEmail{
		From: domain.EmailAddress{
			Name:  account.Name,
			Email: account.ImapUser, // Use IMAP user as from address
		},
		To:          req.To,
		CC:          req.CC,
		BCC:         req.BCC,
		Subject:     req.Subject,
		Text:        req.Text,
		HTML:        req.HTML,
		Attachments: attachments,
	}

	rawMessage, err := email.Build()

	if err != nil {
		slog.Error("failed to build email", "error", err)
		writeError(w, http.StatusBadRequest, "failed to build email: "+err.Error())

		return
	}

	// Send via SMTP
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	smtpCfg := smtp.Config{
		Host:     account.SmtpHost,
		Port:     account.SmtpPort,
		Username: account.SmtpUser,
		Password: account.SmtpPassword,
	}

	// Use IMAP user if SMTP user not set
	if smtpCfg.Username == "" {
		smtpCfg.Username = account.ImapUser
		smtpCfg.Password = account.ImapPassword
	}

	err = smtp.SendEmail(ctx, smtpCfg, account.ImapUser, email.AllRecipients(), rawMessage)

	if err != nil {
		slog.Error("failed to send email", "account_id", accountID, "error", err)
		writeError(w, http.StatusBadGateway, "failed to send email: "+classifySmtpError(err))

		return
	}

	// Extract Message-ID from raw message for response
	msgID := extractMessageID(rawMessage)

	// Store sent email in local DB
	go h.storeSentEmail(accountID, rawMessage, req, attachments)

	slog.Info("email sent successfully", "account_id", accountID, "message_id", msgID, "recipients", len(email.AllRecipients()))

	writeJSON(w, http.StatusOK, sendMessageResponse{
		MessageID: msgID,
		Success:   true,
	})
}

func parseAttachments(inputs []sendAttachmentInput) ([]composer.Attachment, error) {
	if len(inputs) == 0 {
		return nil, nil
	}

	attachments := make([]composer.Attachment, len(inputs))

	for i, input := range inputs {
		data, err := decodeBase64(input.Data)

		if err != nil {
			return nil, errors.New("invalid base64 data for attachment: " + input.Filename)
		}

		attachments[i] = composer.Attachment{
			Filename:    input.Filename,
			ContentType: input.ContentType,
			Data:        data,
			IsInline:    input.IsInline,
			ContentID:   input.ContentID,
		}
	}

	return attachments, nil
}

func extractMessageID(raw []byte) string {
	// Simple extraction - look for Message-Id header
	lines := string(raw)

	for _, prefix := range []string{"Message-Id: ", "Message-ID: ", "message-id: "} {
		start := 0

		for {
			idx := indexAt(lines, prefix, start)

			if idx == -1 {
				break
			}

			end := idx + len(prefix)

			for end < len(lines) && lines[end] != '\r' && lines[end] != '\n' {
				end++
			}

			return lines[idx+len(prefix) : end]
		}
	}

	return ""
}

func indexAt(s, substr string, start int) int {
	if start >= len(s) {
		return -1
	}

	idx := len(s[:start]) + len(substr)

	if idx > len(s) {
		idx = start
	}

	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}

	return -1
}

func (h *MessageHandler) storeSentEmail(accountID uuid.UUID, raw []byte, req sendMessageRequest, _ []composer.Attachment) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Parse the raw message to get Message-ID
	msgID := extractMessageID(raw)

	input := domain.CreateEmailInput{
		AccountID:  accountID,
		UID:        0, // Sent emails don't have IMAP UID
		MessageID:  msgID,
		Folder:     "Sent",
		Subject:    req.Subject,
		FromEmail:  "", // Will be set from account
		FromName:   "",
		To:         req.To,
		CC:         req.CC,
		Date:       time.Now(),
		ParsedText: req.Text,
		ParsedHTML: req.HTML,
		Raw:        string(raw),
		Flags:      []string{"\\Seen"},
	}

	// Get account for from address
	account, err := h.accountRepo.Get(ctx, accountID)

	if err == nil {
		input.FromEmail = account.ImapUser
		input.FromName = account.Name
	}

	_, err = h.emailRepo.Create(ctx, input)

	if err != nil {
		slog.Error("failed to store sent email", "account_id", accountID, "message_id", msgID, "error", err)
	} else {
		slog.Debug("stored sent email", "account_id", accountID, "message_id", msgID)
	}
}

var sendMessageFieldNames = map[string]string{
	"To":          "to",
	"CC":          "cc",
	"BCC":         "bcc",
	"Subject":     "subject",
	"Text":        "text",
	"HTML":        "html",
	"Attachments": "attachments",
	"Filename":    "filename",
	"ContentType": "content_type",
	"Data":        "data",
}

func decodeBase64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}
