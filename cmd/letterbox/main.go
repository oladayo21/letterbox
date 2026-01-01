package main

import (
	"context"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/oladayo21/letterbox/internal/api"
	"github.com/oladayo21/letterbox/internal/config"
	"github.com/oladayo21/letterbox/internal/db"
	"github.com/oladayo21/letterbox/internal/repository"
)

func main() {
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

	accountRepo, err := repository.NewAccountRepository(queries, cfg.EncryptionKeyBytes())

	if err != nil {
		log.Fatalf("repository error: %v", err)
	}

	router := api.NewRouter(cfg.APIKey, accountRepo)

	log.Printf("letterbox starting on :%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, router))
}
