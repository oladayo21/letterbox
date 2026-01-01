package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/oladayo21/letterbox/internal/domain"
	"github.com/oladayo21/letterbox/internal/imap"
	"github.com/oladayo21/letterbox/internal/repository"
)

const maxBodySize = 1 << 20 // 1MB

var validate = validator.New(validator.WithRequiredStructEnabled())

type AccountHandler struct {
	repo *repository.AccountRepository
}

func NewAccountHandler(repo *repository.AccountRepository) *AccountHandler {

	return &AccountHandler{repo: repo}
}

type createAccountRequest struct {
	Name         string `json:"name" validate:"required,max=255"`
	ImapHost     string `json:"imap_host" validate:"required,max=255"`
	ImapPort     int    `json:"imap_port" validate:"required,min=1,max=65535"`
	ImapUser     string `json:"imap_user" validate:"required,max=255"`
	ImapPassword string `json:"imap_password" validate:"required"`
	SmtpHost     string `json:"smtp_host,omitempty" validate:"max=255"`
	SmtpPort     int    `json:"smtp_port,omitempty" validate:"omitempty,min=1,max=65535"`
	SmtpUser     string `json:"smtp_user,omitempty" validate:"max=255"`
	SmtpPassword string `json:"smtp_password,omitempty"`
}

type accountResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	ImapHost  string    `json:"imap_host"`
	ImapPort  int       `json:"imap_port"`
	ImapUser  string    `json:"imap_user"`
	SmtpHost  string    `json:"smtp_host,omitempty"`
	SmtpPort  int       `json:"smtp_port,omitempty"`
	SmtpUser  string    `json:"smtp_user,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func toAccountResponse(a *domain.Account) accountResponse {

	return accountResponse{
		ID:        a.ID,
		Name:      a.Name,
		ImapHost:  a.ImapHost,
		ImapPort:  a.ImapPort,
		ImapUser:  a.ImapUser,
		SmtpHost:  a.SmtpHost,
		SmtpPort:  a.SmtpPort,
		SmtpUser:  a.SmtpUser,
		CreatedAt: a.CreatedAt,
	}
}

func (h *AccountHandler) Create(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

	var req createAccountRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")

		return
	}

	if err := validate.Struct(req); err != nil {
		writeError(w, http.StatusBadRequest, formatValidationError(err))

		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	_, err := imap.TestConnection(ctx, req.ImapHost, req.ImapPort, req.ImapUser, req.ImapPassword)

	if err != nil {
		slog.Warn("IMAP validation failed", "host", req.ImapHost, "user", req.ImapUser, "error", err)
		writeError(w, http.StatusBadRequest, "IMAP validation failed: "+classifyImapError(err))

		return
	}

	input := domain.CreateAccountInput{
		Name:         req.Name,
		ImapHost:     req.ImapHost,
		ImapPort:     req.ImapPort,
		ImapUser:     req.ImapUser,
		ImapPassword: req.ImapPassword,
		SmtpHost:     req.SmtpHost,
		SmtpPort:     req.SmtpPort,
		SmtpUser:     req.SmtpUser,
		SmtpPassword: req.SmtpPassword,
	}

	account, err := h.repo.Create(ctx, input)

	if err != nil {
		slog.Error("failed to create account", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create account")

		return
	}

	writeJSON(w, http.StatusCreated, toAccountResponse(account))
}

func (h *AccountHandler) List(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.repo.List(r.Context())

	if err != nil {
		slog.Error("failed to list accounts", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list accounts")

		return
	}

	response := make([]accountResponse, len(accounts))

	for i, a := range accounts {
		response[i] = toAccountResponse(&a)
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *AccountHandler) Get(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)

	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account ID")

		return
	}

	account, err := h.repo.Get(r.Context(), id)

	if errors.Is(err, repository.ErrAccountNotFound) {
		writeError(w, http.StatusNotFound, "account not found")

		return
	}

	if err != nil {
		slog.Error("failed to get account", "account_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get account")

		return
	}

	writeJSON(w, http.StatusOK, toAccountResponse(account))
}

func (h *AccountHandler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)

	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account ID")

		return
	}

	err = h.repo.Delete(r.Context(), id)

	if errors.Is(err, repository.ErrAccountNotFound) {
		writeError(w, http.StatusNotFound, "account not found")

		return
	}

	if err != nil {
		slog.Error("failed to delete account", "account_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete account")

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

var fieldNames = map[string]string{
	"Name":         "name",
	"ImapHost":     "imap_host",
	"ImapPort":     "imap_port",
	"ImapUser":     "imap_user",
	"ImapPassword": "imap_password",
	"SmtpHost":     "smtp_host",
	"SmtpPort":     "smtp_port",
	"SmtpUser":     "smtp_user",
	"SmtpPassword": "smtp_password",
}

func formatValidationError(err error) string {
	var ve validator.ValidationErrors

	if !errors.As(err, &ve) {
		return "validation failed"
	}

	var msgs []string

	for _, fe := range ve {
		field := fieldNames[fe.Field()]

		if field == "" {
			field = fe.Field()
		}

		msgs = append(msgs, formatFieldError(field, fe.Tag(), fe.Param()))
	}

	return strings.Join(msgs, "; ")
}

func formatFieldError(field, tag, param string) string {

	switch tag {
	case "required":
		return field + " is required"
	case "max":
		return field + " must be " + param + " characters or less"
	case "min":
		return field + " must be at least " + param
	default:
		return field + " is invalid"
	}
}

func classifyImapError(err error) string {

	if errors.Is(err, imap.ErrAuthFailed) {
		return "authentication failed"
	}

	if errors.Is(err, imap.ErrTimeout) {
		return "connection timed out"
	}

	if errors.Is(err, imap.ErrTLSFailed) {
		return "TLS handshake failed"
	}

	if errors.Is(err, imap.ErrConnectionFailed) {
		return "connection failed"
	}

	return "unknown error"
}
