package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/oladayo21/letterbox/internal/repository"
	"github.com/oladayo21/letterbox/internal/search"
)

type SearchHandler struct {
	accountRepo *repository.AccountRepository
	searchSvc   *search.Service
}

func NewSearchHandler(accountRepo *repository.AccountRepository, searchSvc *search.Service) *SearchHandler {
	return &SearchHandler{
		accountRepo: accountRepo,
		searchSvc:   searchSvc,
	}
}

type searchResponse struct {
	Messages []messageListItem `json:"messages"`
	Total    int64             `json:"total"`
	Limit    int               `json:"limit"`
	Offset   int               `json:"offset"`
}

func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")

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

	limit, offset, err := parseSearchParams(r)

	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())

		return
	}

	result, err := h.searchSvc.Search(r.Context(), accountID, query, folder, limit, offset)

	if errors.Is(err, search.ErrEmptyQuery) {
		writeError(w, http.StatusBadRequest, "search query cannot be empty")

		return
	}

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

	messages := make([]messageListItem, len(result.Emails))

	for i, e := range result.Emails {
		messages[i] = toMessageListItem(&e)
	}

	writeJSON(w, http.StatusOK, searchResponse{
		Messages: messages,
		Total:    result.Total,
		Limit:    result.Limit,
		Offset:   result.Offset,
	})
}

func parseSearchParams(r *http.Request) (limit, offset int, err error) {
	limit = defaultLimit
	offset = 0

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		limit, err = strconv.Atoi(limitStr)

		if err != nil || limit < 1 {
			return 0, 0, errors.New("invalid limit")
		}

		if limit > maxLimit {
			limit = maxLimit
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		offset, err = strconv.Atoi(offsetStr)

		if err != nil || offset < 0 {
			return 0, 0, errors.New("invalid offset")
		}
	}

	return limit, offset, nil
}
