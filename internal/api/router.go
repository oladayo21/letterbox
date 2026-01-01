package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/oladayo21/letterbox/internal/repository"
)

func NewRouter(
	apiKey string,
	accountRepo *repository.AccountRepository,
	emailRepo *repository.EmailRepository,
) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(APIKeyAuth(apiKey))

	accountHandler := NewAccountHandler(accountRepo)
	folderHandler := NewFolderHandler(accountRepo)
	messageHandler := NewMessageHandler(accountRepo, emailRepo)

	r.Route("/accounts", func(r chi.Router) {
		r.Post("/", accountHandler.Create)
		r.Get("/", accountHandler.List)
		r.Get("/{id}", accountHandler.Get)
		r.Delete("/{id}", accountHandler.Delete)
		r.Get("/{id}/folders", folderHandler.List)
		r.Get("/{id}/folders/{name}/messages", messageHandler.ListMessages)
	})

	return r
}
