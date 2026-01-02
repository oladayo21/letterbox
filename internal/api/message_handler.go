package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/oladayo21/letterbox/internal/domain"
	"github.com/oladayo21/letterbox/internal/imap"
	"github.com/oladayo21/letterbox/internal/ingest"
	"github.com/oladayo21/letterbox/internal/repository"
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
