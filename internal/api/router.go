package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/oladayo21/letterbox/internal/ingest"
	"github.com/oladayo21/letterbox/internal/logging"
	"github.com/oladayo21/letterbox/internal/repository"
	"github.com/oladayo21/letterbox/internal/storage"
)

// RouterConfig contains dependencies for the router.
type RouterConfig struct {
	APIKey         string
	AccountRepo    *repository.AccountRepository
	EmailRepo      *repository.EmailRepository
	AttachmentRepo *repository.AttachmentRepository
	WebhookRepo    *repository.WebhookRepository
	Ingester       *ingest.Ingester
	S3             *storage.S3Storage
	DB             HealthChecker
	Sync           SyncStatusProvider
}

func NewRouter(cfg RouterConfig) *chi.Mux {
	r := chi.NewRouter()

	// Global middleware
	r.Use(logging.RequestLogger)
	r.Use(middleware.Recoverer)

	// Health endpoints (unauthenticated)
	healthHandler := NewHealthHandler(cfg.DB, cfg.S3, cfg.Sync)
	r.Get("/health", healthHandler.Health)
	r.Get("/ready", healthHandler.Ready)

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(APIKeyAuth(cfg.APIKey))

		accountHandler := NewAccountHandler(cfg.AccountRepo)
		folderHandler := NewFolderHandler(cfg.AccountRepo)
		messageHandler := NewMessageHandler(cfg.AccountRepo, cfg.EmailRepo, cfg.AttachmentRepo, cfg.Ingester, cfg.S3)
		webhookHandler := NewWebhookHandler(cfg.WebhookRepo)
		searchHandler := NewSearchHandler(cfg.AccountRepo, cfg.EmailRepo)

		r.Route("/accounts", func(r chi.Router) {
			r.Post("/", accountHandler.Create)
			r.Get("/", accountHandler.List)
			r.Get("/{id}", accountHandler.Get)
			r.Delete("/{id}", accountHandler.Delete)
			r.Get("/{id}/folders", folderHandler.List)
			r.Get("/{id}/folders/{name}/messages", messageHandler.ListMessages)
			r.Get("/{id}/messages/{uid}", messageHandler.GetMessage)
			r.Post("/{id}/messages", messageHandler.SendMessage)
		})

		r.Route("/webhooks", func(r chi.Router) {
			r.Post("/", webhookHandler.Create)
			r.Get("/", webhookHandler.List)
			r.Delete("/{id}", webhookHandler.Delete)
		})

		r.Get("/search", searchHandler.Search)
	})

	return r
}
