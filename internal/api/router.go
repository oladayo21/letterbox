package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/oladayo21/letterbox/internal/ingest"
	"github.com/oladayo21/letterbox/internal/repository"
	"github.com/oladayo21/letterbox/internal/storage"
)

func NewRouter(
	apiKey string,
	accountRepo *repository.AccountRepository,
	emailRepo *repository.EmailRepository,
	attachmentRepo *repository.AttachmentRepository,
	webhookRepo *repository.WebhookRepository,
	ingester *ingest.Ingester,
	s3 *storage.S3Storage,
) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(APIKeyAuth(apiKey))

	accountHandler := NewAccountHandler(accountRepo)
	folderHandler := NewFolderHandler(accountRepo)
	messageHandler := NewMessageHandler(accountRepo, emailRepo, attachmentRepo, ingester, s3)
	webhookHandler := NewWebhookHandler(webhookRepo)

	r.Route("/accounts", func(r chi.Router) {
		r.Post("/", accountHandler.Create)
		r.Get("/", accountHandler.List)
		r.Get("/{id}", accountHandler.Get)
		r.Delete("/{id}", accountHandler.Delete)
		r.Get("/{id}/folders", folderHandler.List)
		r.Get("/{id}/folders/{name}/messages", messageHandler.ListMessages)
		r.Get("/{id}/messages/{uid}", messageHandler.GetMessage)
	})

	r.Route("/webhooks", func(r chi.Router) {
		r.Post("/", webhookHandler.Create)
		r.Get("/", webhookHandler.List)
		r.Delete("/{id}", webhookHandler.Delete)
	})

	return r
}
