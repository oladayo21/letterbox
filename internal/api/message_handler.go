package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/oladayo21/letterbox/internal/domain"
	"github.com/oladayo21/letterbox/internal/repository"
)

const (
	defaultLimit = 50
	maxLimit     = 100
)

type MessageHandler struct {
	accountRepo *repository.AccountRepository
	emailRepo   *repository.EmailRepository
}

func NewMessageHandler(
	accountRepo *repository.AccountRepository,
	emailRepo *repository.EmailRepository,
) *MessageHandler {

	return &MessageHandler{
		accountRepo: accountRepo,
		emailRepo:   emailRepo,
	}
}

type messageListItem struct {
	ID        uuid.UUID            `json:"id"`
	UID       int64                `json:"uid"`
	MessageID string               `json:"message_id,omitempty"`
	Folder    string               `json:"folder"`
	Subject   string               `json:"subject"`
	From      domain.EmailAddress  `json:"from"`
	To        []domain.EmailAddress `json:"to"`
	Date      time.Time            `json:"date"`
	Flags     []string             `json:"flags"`
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
	limit = defaultLimit
	offset = 0

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		limit, err = strconv.Atoi(limitStr)

		if err != nil || limit < 1 {
			return 0, 0, nil, nil, errors.New("invalid limit")
		}

		if limit > maxLimit {
			limit = maxLimit
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		offset, err = strconv.Atoi(offsetStr)

		if err != nil || offset < 0 {
			return 0, 0, nil, nil, errors.New("invalid offset")
		}
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
