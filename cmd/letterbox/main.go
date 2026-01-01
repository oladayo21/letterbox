package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/oladayo21/letterbox/internal/api"
	"github.com/oladayo21/letterbox/internal/config"
	"github.com/oladayo21/letterbox/internal/db"
	"github.com/oladayo21/letterbox/internal/repository"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()

	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)

	if err != nil {
		log.Fatalf("database connection error: %v", err)
	}

	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("database ping error: %v", err)
	}

	queries := db.New(pool)

	if cfg.APIKey == "" {
		log.Fatal("LETTERBOX_API_KEY must not be empty")
	}

	accountRepo, err := repository.NewAccountRepository(queries, cfg.EncryptionKeyBytes())

	if err != nil {
		log.Fatalf("repository error: %v", err)
	}

	emailRepo := repository.NewEmailRepository(queries)

	router := api.NewRouter(cfg.APIKey, accountRepo, emailRepo)

	slog.Info("letterbox starting", "port", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, router))
}
