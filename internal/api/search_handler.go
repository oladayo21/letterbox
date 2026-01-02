package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/oladayo21/letterbox/internal/domain"
	"github.com/oladayo21/letterbox/internal/repository"
)

type SearchHandler struct {
	accountRepo *repository.AccountRepository
	emailRepo   *repository.EmailRepository
}

func NewSearchHandler(accountRepo *repository.AccountRepository, emailRepo *repository.EmailRepository) *SearchHandler {
	return &SearchHandler{
		accountRepo: accountRepo,
		emailRepo:   emailRepo,
	}
}

type searchResponse struct {
	Messages []messageListItem `json:"messages"`
	Total    int64             `json:"total"`
	Limit    int               `json:"limit"`
	Offset   int               `json:"offset"`
}

func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))

	if query == "" {
		writeError(w, http.StatusBadRequest, "query parameter 'q' is required")

		return
	}

	accountIDStr := r.URL.Query().Get("account_id")

	if accountIDStr == "" {
		writeError(w, http.StatusBadRequest, "query parameter 'account_id' is required")

		return
	}

	accountID, err := uuid.Parse(accountIDStr)

	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account_id")

		return
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

	folder := r.URL.Query().Get("folder")

	limit, offset, err := parsePaginationParams(r)

	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())

		return
	}

	filter := domain.SearchEmailsFilter{
		AccountID: accountID,
		Query:     query,
		Folder:    folder,
		Limit:     limit,
		Offset:    offset,
	}

	emails, err := h.emailRepo.Search(r.Context(), filter)

	if errors.Is(err, repository.ErrInvalidSearchQuery) {
		writeError(w, http.StatusBadRequest, "invalid search query syntax")

		return
	}

	if err != nil {
		slog.Error("search failed",
			"account_id", accountID,
			"query", query,
			"folder", folder,
			"limit", limit,
			"offset", offset,
			"error", err)
		writeError(w, http.StatusInternalServerError, "search failed")

		return
	}

	total, err := h.emailRepo.CountSearch(r.Context(), accountID, query, folder)

	if err != nil {
		slog.Error("count search failed",
			"account_id", accountID,
			"query", query,
			"folder", folder,
			"error", err)
		writeError(w, http.StatusInternalServerError, "search failed")

		return
	}

	messages := make([]messageListItem, len(emails))

	for i, e := range emails {
		messages[i] = toMessageListItem(&e)
	}

	writeJSON(w, http.StatusOK, searchResponse{
		Messages: messages,
		Total:    total,
		Limit:    limit,
		Offset:   offset,
	})
}
