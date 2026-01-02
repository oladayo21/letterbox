package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/oladayo21/letterbox/internal/domain"
	"github.com/oladayo21/letterbox/internal/repository"
)

type WebhookHandler struct {
	repo *repository.WebhookRepository
}

func NewWebhookHandler(repo *repository.WebhookRepository) *WebhookHandler {

	return &WebhookHandler{repo: repo}
}

type createWebhookRequest struct {
	AccountID string `json:"account_id" validate:"required,uuid"`
	URL       string `json:"url" validate:"required,url,max=2048"`
	Secret    string `json:"secret" validate:"required,min=16,max=256"`
}

type webhookResponse struct {
	ID        uuid.UUID `json:"id"`
	AccountID uuid.UUID `json:"account_id"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

func toWebhookResponse(w *domain.Webhook) webhookResponse {

	return webhookResponse{
		ID:        w.ID,
		AccountID: w.AccountID,
		URL:       w.URL,
		CreatedAt: w.CreatedAt,
	}
}

func (h *WebhookHandler) Create(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

	var req createWebhookRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")

		return
	}

	if err := validate.Struct(req); err != nil {
		writeError(w, http.StatusBadRequest, formatWebhookValidationError(err))

		return
	}

	accountID, err := uuid.Parse(req.AccountID)

	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account_id format")

		return
	}

	input := domain.CreateWebhookInput{
		AccountID: accountID,
		URL:       req.URL,
		Secret:    req.Secret,
	}

	webhook, err := h.repo.Create(r.Context(), input)

	if err != nil {
		if errors.Is(err, repository.ErrEmptyAccountID) {
			writeError(w, http.StatusBadRequest, "account_id is required")

			return
		}

		if errors.Is(err, repository.ErrEmptyURL) {
			writeError(w, http.StatusBadRequest, "url is required")

			return
		}

		if errors.Is(err, repository.ErrEmptySecret) {
			writeError(w, http.StatusBadRequest, "secret is required")

			return
		}

		// Check for FK violation (non-existent account)
		var pgErr *pgconn.PgError

		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			writeError(w, http.StatusBadRequest, "account not found")

			return
		}

		slog.Error("failed to create webhook", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create webhook")

		return
	}

	writeJSON(w, http.StatusCreated, toWebhookResponse(webhook))
}

func (h *WebhookHandler) List(w http.ResponseWriter, r *http.Request) {
	webhooks, err := h.repo.List(r.Context())

	if err != nil {
		slog.Error("failed to list webhooks", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list webhooks")

		return
	}

	response := make([]webhookResponse, len(webhooks))

	for i, wh := range webhooks {
		response[i] = toWebhookResponse(&wh)
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *WebhookHandler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)

	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid webhook ID")

		return
	}

	err = h.repo.Delete(r.Context(), id)

	if errors.Is(err, repository.ErrWebhookNotFound) {
		writeError(w, http.StatusNotFound, "webhook not found")

		return
	}

	if err != nil {
		slog.Error("failed to delete webhook", "webhook_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete webhook")

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

var webhookFieldNames = map[string]string{
	"AccountID": "account_id",
	"URL":       "url",
	"Secret":    "secret",
}

func formatWebhookValidationError(err error) string {
	var ve validator.ValidationErrors

	if !errors.As(err, &ve) {
		return "validation failed"
	}

	var msgs []string

	for _, fe := range ve {
		field := webhookFieldNames[fe.Field()]

		if field == "" {
			field = fe.Field()
		}

		msgs = append(msgs, formatWebhookFieldError(field, fe.Tag(), fe.Param()))
	}

	return strings.Join(msgs, "; ")
}

func formatWebhookFieldError(field, tag, param string) string {

	switch tag {
	case "required":
		return field + " is required"
	case "uuid":
		return field + " must be a valid UUID"
	case "url":
		return field + " must be a valid URL"
	case "max":
		return field + " must be " + param + " characters or less"
	case "min":
		return field + " must be at least " + param + " characters"
	default:
		return field + " is invalid"
	}
}
