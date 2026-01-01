package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/oladayo21/letterbox/internal/repository"
)

func NewRouter(apiKey string, accountRepo *repository.AccountRepository) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(APIKeyAuth(apiKey))

	accountHandler := NewAccountHandler(accountRepo)

	r.Route("/accounts", func(r chi.Router) {
		r.Post("/", accountHandler.Create)
		r.Get("/", accountHandler.List)
		r.Get("/{id}", accountHandler.Get)
		r.Delete("/{id}", accountHandler.Delete)
	})

	return r
}
