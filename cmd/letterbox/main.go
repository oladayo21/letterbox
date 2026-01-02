package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/oladayo21/letterbox/internal/api"
	"github.com/oladayo21/letterbox/internal/config"
	"github.com/oladayo21/letterbox/internal/db"
	"github.com/oladayo21/letterbox/internal/ingest"
	"github.com/oladayo21/letterbox/internal/logging"
	"github.com/oladayo21/letterbox/internal/repository"
	"github.com/oladayo21/letterbox/internal/storage"
	"github.com/oladayo21/letterbox/internal/sync"
	"github.com/oladayo21/letterbox/internal/webhook"
)

const (
	shutdownTimeout = 30 * time.Second
)

func main() {
	cfg, err := config.Load()

	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	logging.Setup(logging.Config{
		Level:  cfg.LogLevel,
		Format: cfg.LogFormat,
	})

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)

	if err != nil {
		log.Fatalf("database connection error: %v", err)
	}

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
		log.Fatalf("account repository error: %v", err)
	}

	emailRepo := repository.NewEmailRepository(queries)
	attachmentRepo := repository.NewAttachmentRepository(queries)

	webhookRepo, err := repository.NewWebhookRepository(queries, cfg.EncryptionKeyBytes())

	if err != nil {
		log.Fatalf("webhook repository error: %v", err)
	}

	// Initialize ingester
	ingester := ingest.NewIngester(accountRepo, emailRepo, attachmentRepo, s3Storage)

	// Initialize webhook producer and worker
	webhookProducer := webhook.NewProducer(queries, webhookRepo, s3Storage)
	webhookWorker := webhook.NewWorker(queries, webhookRepo, webhook.WorkerConfig{})

	// Initialize sync coordinator with webhook handler
	coordinator := sync.NewCoordinator(ingester, accountRepo, sync.CoordinatorConfig{
		EventHandler: webhookProducer.EventHandler(),
	})

	router := api.NewRouter(api.RouterConfig{
		APIKey:         cfg.APIKey,
		AccountRepo:    accountRepo,
		EmailRepo:      emailRepo,
		AttachmentRepo: attachmentRepo,
		WebhookRepo:    webhookRepo,
		Ingester:       ingester,
		S3:             s3Storage,
		DB:             pool,
	})

	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	// Start background services
	coordinator.Start()
	webhookWorker.Start()

	// Create cancellable context for startup tasks
	startupCtx, startupCancel := context.WithCancel(context.Background())

	// Load existing accounts into sync coordinator
	go loadExistingAccounts(startupCtx, accountRepo, coordinator)

	// Start HTTP server in goroutine
	go func() {
		slog.Info("letterbox starting", "port", cfg.Port)

		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTP server error", "error", err)
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	slog.Info("shutdown signal received", "signal", sig.String())

	// Cancel any in-progress startup tasks
	startupCancel()

	// Create shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// Shutdown components in order
	slog.Info("stopping sync coordinator...")

	if err := coordinator.Close(); err != nil {
		slog.Error("coordinator shutdown error", "error", err)
	}

	slog.Info("stopping webhook worker...")

	if err := webhookWorker.Stop(); err != nil {
		slog.Error("webhook worker shutdown error", "error", err)
	}

	slog.Info("stopping HTTP server...")

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}

	slog.Info("closing database connections...")
	pool.Close()

	slog.Info("letterbox stopped")
}

// loadExistingAccounts loads all accounts into the sync coordinator on startup.
func loadExistingAccounts(ctx context.Context, accountRepo *repository.AccountRepository, coordinator *sync.Coordinator) {
	accounts, err := accountRepo.List(ctx)

	if err != nil {
		slog.Error("failed to load accounts for sync", "error", err)

		return
	}

	for _, account := range accounts {
		// Check for context cancellation (shutdown)
		select {
		case <-ctx.Done():
			slog.Debug("account loading cancelled", "remaining", len(accounts))

			return
		default:
		}

		if err := coordinator.AddAccount(ctx, account.ID); err != nil {
			slog.Error("failed to add account to sync",
				"account_id", account.ID,
				"account_name", account.Name,
				"error", err,
			)
		} else {
			slog.Info("account added to sync",
				"account_id", account.ID,
				"account_name", account.Name,
			)
		}
	}
}
