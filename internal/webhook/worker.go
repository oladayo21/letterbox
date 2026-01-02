package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/oladayo21/letterbox/internal/db"
	"github.com/oladayo21/letterbox/internal/repository"
)

const (
	// DefaultPollInterval is the default interval between queue polls.
	DefaultPollInterval = 5 * time.Second

	// DefaultBatchSize is the default number of items to fetch per poll.
	DefaultBatchSize = 10

	// DefaultHTTPTimeout is the default timeout for webhook HTTP requests.
	DefaultHTTPTimeout = 30 * time.Second

	// DefaultMaxRetries is the default maximum number of delivery attempts.
	DefaultMaxRetries = 5

	// DefaultBaseBackoff is the base duration for exponential backoff.
	DefaultBaseBackoff = 30 * time.Second

	// SignatureHeader is the header name for the webhook signature.
	SignatureHeader = "X-Letterbox-Signature"

	// TimestampHeader is the header name for the webhook timestamp.
	TimestampHeader = "X-Letterbox-Timestamp"
)

// WorkerConfig contains configuration for the webhook worker.
type WorkerConfig struct {
	PollInterval time.Duration
	BatchSize    int32
	HTTPTimeout  time.Duration
	MaxRetries   int32
	BaseBackoff  time.Duration
}

// Worker polls the webhook queue and delivers payloads.
type Worker struct {
	queries     *db.Queries
	webhookRepo *repository.WebhookRepository
	httpClient  *http.Client
	config      WorkerConfig

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu      sync.Mutex
	started bool
	closed  bool
}

// NewWorker creates a new webhook delivery worker.
func NewWorker(
	queries *db.Queries,
	webhookRepo *repository.WebhookRepository,
	config WorkerConfig,
) *Worker {
	if config.PollInterval == 0 {
		config.PollInterval = DefaultPollInterval
	}

	if config.BatchSize == 0 {
		config.BatchSize = DefaultBatchSize
	}

	if config.HTTPTimeout == 0 {
		config.HTTPTimeout = DefaultHTTPTimeout
	}

	if config.MaxRetries == 0 {
		config.MaxRetries = DefaultMaxRetries
	}

	if config.BaseBackoff == 0 {
		config.BaseBackoff = DefaultBaseBackoff
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Worker{
		queries:     queries,
		webhookRepo: webhookRepo,
		httpClient: &http.Client{
			Timeout: config.HTTPTimeout,
		},
		config: config,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start begins the worker polling loop.
func (w *Worker) Start() {
	w.mu.Lock()

	if w.started {
		w.mu.Unlock()

		return
	}

	w.started = true
	w.mu.Unlock()

	w.wg.Add(1)

	go w.run()

	slog.Info("webhook worker started",
		"poll_interval", w.config.PollInterval,
		"batch_size", w.config.BatchSize,
	)
}

// Stop gracefully shuts down the worker.
func (w *Worker) Stop() error {
	w.mu.Lock()

	if w.closed {
		w.mu.Unlock()

		return nil
	}

	w.closed = true
	w.mu.Unlock()

	w.cancel()
	w.wg.Wait()

	slog.Info("webhook worker stopped")

	return nil
}

// run is the main polling loop.
func (w *Worker) run() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.config.PollInterval)
	defer ticker.Stop()

	// Process immediately on start
	w.processBatch()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.processBatch()
		}
	}
}

// processBatch fetches and processes a batch of pending webhook deliveries.
func (w *Worker) processBatch() {
	items, err := w.queries.GetPendingWebhookQueueItems(w.ctx, w.config.BatchSize)

	if err != nil {
		slog.Error("failed to fetch pending webhooks", "error", err)

		return
	}

	if len(items) == 0 {
		return
	}

	slog.Debug("processing webhook batch", "count", len(items))

	for _, item := range items {
		// Check for shutdown
		select {
		case <-w.ctx.Done():
			return
		default:
		}

		w.deliverWebhook(item)
	}
}

// deliverWebhook attempts to deliver a single webhook.
func (w *Worker) deliverWebhook(item db.WebhookQueue) {
	itemID := pgtypeToUUID(item.ID)
	webhookID := pgtypeToUUID(item.WebhookID)

	attempts := int32(0)

	if item.Attempts != nil {
		attempts = *item.Attempts
	}

	// Get webhook details (URL and secret)
	webhook, err := w.webhookRepo.Get(w.ctx, webhookID)

	if err != nil {
		// Context cancelled during shutdown - leave as pending for next worker
		if errors.Is(err, context.Canceled) {
			return
		}

		// Webhook not found is permanent failure
		if errors.Is(err, repository.ErrWebhookNotFound) {
			slog.Error("webhook not found, marking as failed",
				"queue_id", itemID,
				"webhook_id", webhookID,
			)

			w.markFailed(item.ID)

			return
		}

		slog.Error("failed to get webhook for delivery",
			"queue_id", itemID,
			"webhook_id", webhookID,
			"error", err,
		)

		w.scheduleRetry(item.ID, attempts)

		return
	}

	// Build and send request
	err = w.sendWebhook(webhook.URL, webhook.Secret, item.Payload)

	if err != nil {
		// Context cancelled during shutdown - leave as pending for next worker
		if errors.Is(err, context.Canceled) {
			return
		}

		slog.Warn("webhook delivery failed",
			"queue_id", itemID,
			"webhook_id", webhookID,
			"url", webhook.URL,
			"attempt", attempts+1,
			"error", err,
		)

		w.scheduleRetry(item.ID, attempts)

		return
	}

	// Success - mark delivered
	if !w.markDelivered(item.ID) {
		slog.Error("CRITICAL: webhook delivered but failed to update status",
			"queue_id", itemID,
			"webhook_id", webhookID,
			"url", webhook.URL,
		)

		return
	}

	slog.Info("webhook delivered",
		"queue_id", itemID,
		"webhook_id", webhookID,
		"url", webhook.URL,
		"attempts", attempts+1,
	)
}

// sendWebhook sends the HTTP POST request with signature.
func (w *Worker) sendWebhook(url, secret string, payload []byte) error {
	timestamp := time.Now().Unix()
	signature := computeSignature(payload, secret, timestamp)

	req, err := http.NewRequestWithContext(w.ctx, http.MethodPost, url, bytes.NewReader(payload))

	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TimestampHeader, fmt.Sprintf("%d", timestamp))
	req.Header.Set(SignatureHeader, signature)

	resp, err := w.httpClient.Do(req)

	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}

	defer func() {
		// Drain body for connection reuse
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	// Consider 2xx as success
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

// computeSignature generates HMAC-SHA256 signature for webhook verification.
// Format: HMAC-SHA256(timestamp.payload, secret)
func computeSignature(payload []byte, secret string, timestamp int64) string {
	message := fmt.Sprintf("%d.%s", timestamp, payload)

	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(message))

	return hex.EncodeToString(h.Sum(nil))
}

// markDelivered marks a queue item as successfully delivered.
// Returns true if the update succeeded.
func (w *Worker) markDelivered(id pgtype.UUID) bool {
	_, err := w.queries.MarkWebhookQueueDelivered(w.ctx, id)

	if err != nil {
		slog.Error("failed to mark webhook as delivered",
			"queue_id", pgtypeToUUID(id),
			"error", err,
		)

		return false
	}

	return true
}

// markFailed marks a queue item as permanently failed.
// Returns true if the update succeeded.
func (w *Worker) markFailed(id pgtype.UUID) bool {
	_, err := w.queries.MarkWebhookQueueFailed(w.ctx, id)

	if err != nil {
		slog.Error("failed to mark webhook as failed",
			"queue_id", pgtypeToUUID(id),
			"error", err,
		)

		return false
	}

	return true
}

// scheduleRetry schedules a retry with exponential backoff, or marks as failed if max retries exceeded.
func (w *Worker) scheduleRetry(id pgtype.UUID, currentAttempts int32) {
	newAttempts := currentAttempts + 1

	// Check if max retries exceeded
	if newAttempts >= w.config.MaxRetries {
		slog.Error("max retries exceeded, marking as failed",
			"queue_id", pgtypeToUUID(id),
			"attempts", newAttempts,
			"max_retries", w.config.MaxRetries,
		)

		w.markFailed(id)

		return
	}

	// Calculate exponential backoff: base * 2^attempts
	// e.g., 30s, 60s, 120s, 240s, 480s for base=30s
	backoff := w.config.BaseBackoff * time.Duration(1<<currentAttempts)
	nextAttempt := time.Now().Add(backoff)

	params := db.UpdateWebhookQueueStatusParams{
		ID:       id,
		Status:   "pending",
		Attempts: &newAttempts,
		NextAttempt: pgtype.Timestamptz{
			Time:  nextAttempt,
			Valid: true,
		},
	}

	_, err := w.queries.UpdateWebhookQueueStatus(w.ctx, params)

	if err != nil {
		slog.Error("failed to schedule retry",
			"queue_id", pgtypeToUUID(id),
			"error", err,
		)

		return
	}

	slog.Info("scheduled webhook retry",
		"queue_id", pgtypeToUUID(id),
		"attempt", newAttempts,
		"next_attempt", nextAttempt,
		"backoff", backoff,
	)
}

// pgtypeToUUID converts pgtype.UUID to uuid.UUID.
func pgtypeToUUID(p pgtype.UUID) uuid.UUID {
	if !p.Valid {
		return uuid.Nil
	}

	return uuid.UUID(p.Bytes)
}
