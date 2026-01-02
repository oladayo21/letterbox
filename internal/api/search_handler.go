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
	Data       []messageListItem `json:"data"`
	Pagination paginationInfo    `json:"pagination"`
}

type paginationInfo struct {
	Limit  int   `json:"limit"`
	Offset int   `json:"offset"`
	Total  int64 `json:"total"`
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
	limit, offset := parseSearchParams(r)

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
		slog.Error("search failed", "account_id", accountID, "query", query, "error", err)
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}

	data := make([]messageListItem, len(result.Emails))

	for i, e := range result.Emails {
		data[i] = toMessageListItem(&e)
	}

	writeJSON(w, http.StatusOK, searchResponse{
		Data: data,
		Pagination: paginationInfo{
			Limit:  result.Limit,
			Offset: result.Offset,
			Total:  result.Total,
		},
	})
}

func parseSearchParams(r *http.Request) (limit, offset int) {
	limit = defaultLimit
	offset = 0

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l

			if limit > maxLimit {
				limit = maxLimit
			}
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	return limit, offset
}
