package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/oladayo21/letterbox/internal/imap"
	"github.com/oladayo21/letterbox/internal/repository"
)

type FolderHandler struct {
	accountRepo *repository.AccountRepository
}

func NewFolderHandler(accountRepo *repository.AccountRepository) *FolderHandler {

	return &FolderHandler{accountRepo: accountRepo}
}

func (h *FolderHandler) List(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)

	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account ID")

		return
	}

	account, err := h.accountRepo.Get(r.Context(), id)

	if errors.Is(err, repository.ErrAccountNotFound) {
		writeError(w, http.StatusNotFound, "account not found")

		return
	}

	if err != nil {
		slog.Error("failed to get account", "account_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get account")

		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	folders, err := imap.ListFolders(
		ctx,
		account.ImapHost,
		account.ImapPort,
		account.ImapUser,
		account.ImapPassword,
	)

	if err != nil {
		slog.Error("failed to list folders", "account_id", id, "error", err)
		writeError(w, http.StatusBadGateway, "failed to list folders: "+classifyImapError(err))

		return
	}

	writeJSON(w, http.StatusOK, folders)
}
