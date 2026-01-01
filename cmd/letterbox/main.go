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
	"github.com/oladayo21/letterbox/internal/ingest"
	"github.com/oladayo21/letterbox/internal/repository"
	"github.com/oladayo21/letterbox/internal/storage"
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

	// Initialize S3 storage
	s3Storage, err := storage.NewS3Storage(storage.S3Config{
		Endpoint:  cfg.S3Endpoint,
		Bucket:    cfg.S3Bucket,
		AccessKey: cfg.S3AccessKey,
		SecretKey: cfg.S3SecretKey,
		Region:    cfg.S3Region,
	})

	if err != nil {
		log.Fatalf("S3 storage error: %v", err)
	}

	// Initialize repositories
	accountRepo, err := repository.NewAccountRepository(queries, cfg.EncryptionKeyBytes())

	if err != nil {
		log.Fatalf("repository error: %v", err)
	}

	emailRepo := repository.NewEmailRepository(queries)
	attachmentRepo := repository.NewAttachmentRepository(queries)

	// Initialize ingester
	ingester := ingest.NewIngester(accountRepo, emailRepo, attachmentRepo, s3Storage)

	router := api.NewRouter(cfg.APIKey, accountRepo, emailRepo, attachmentRepo, ingester, s3Storage)

	slog.Info("letterbox starting", "port", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, router))
}
