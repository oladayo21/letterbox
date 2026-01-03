package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/oladayo21/letterbox/internal/db"
	"github.com/oladayo21/letterbox/internal/domain"
	"github.com/oladayo21/letterbox/internal/repository"
)

// mockQueueReader implements QueueReader for testing.
type mockQueueReader struct {
	items         []db.WebhookQueue
	getPendingErr error

	deliveredIDs []pgtype.UUID
	failedIDs    []pgtype.UUID
	updatedItems []db.UpdateWebhookQueueStatusParams

	callCount int
}

func (m *mockQueueReader) GetPendingWebhookQueueItems(ctx context.Context, limit int32) ([]db.WebhookQueue, error) {
	m.callCount++

	if m.getPendingErr != nil {
		return nil, m.getPendingErr
	}

	// Only return items on first call to avoid re-processing
	if m.callCount == 1 {
		return m.items, nil
	}

	return nil, nil
}

func (m *mockQueueReader) MarkWebhookQueueDelivered(ctx context.Context, id pgtype.UUID) (int64, error) {
	m.deliveredIDs = append(m.deliveredIDs, id)

	return 1, nil
}

func (m *mockQueueReader) MarkWebhookQueueFailed(ctx context.Context, id pgtype.UUID) (int64, error) {
	m.failedIDs = append(m.failedIDs, id)

	return 1, nil
}

func (m *mockQueueReader) UpdateWebhookQueueStatus(ctx context.Context, arg db.UpdateWebhookQueueStatusParams) (int64, error) {
	m.updatedItems = append(m.updatedItems, arg)

	return 1, nil
}

// mockWebhookGetter implements WebhookGetter for testing.
type mockWebhookGetter struct {
	webhooks map[uuid.UUID]*domain.Webhook
	err      error
}

func (m *mockWebhookGetter) Get(ctx context.Context, id uuid.UUID) (*domain.Webhook, error) {
	if m.err != nil {
		return nil, m.err
	}

	if wh, ok := m.webhooks[id]; ok {
		return wh, nil
	}

	return nil, repository.ErrWebhookNotFound
}

func TestComputeSignature(t *testing.T) {
	payload := []byte(`{"event":"email.received"}`)
	secret := "test-secret"
	timestamp := int64(1704067200)

	signature := computeSignature(payload, secret, timestamp)

	// Verify manually
	message := fmt.Sprintf("%d.%s", timestamp, payload)
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(message))
	expected := hex.EncodeToString(h.Sum(nil))

	if signature != expected {
		t.Errorf("signature mismatch: expected %s, got %s", expected, signature)
	}
}

func TestComputeSignature_DifferentInputs(t *testing.T) {
	payload := []byte(`{"event":"email.received"}`)
	secret := "test-secret"
	timestamp := int64(1704067200)

	sig1 := computeSignature(payload, secret, timestamp)
	sig2 := computeSignature(payload, secret, timestamp+1)
	sig3 := computeSignature(payload, "different-secret", timestamp)

	if sig1 == sig2 {
		t.Error("different timestamps should produce different signatures")
	}

	if sig1 == sig3 {
		t.Error("different secrets should produce different signatures")
	}
}

func TestWorker_StartStop(t *testing.T) {
	queue := &mockQueueReader{}
	webhookRepo := &mockWebhookGetter{}

	worker := NewWorker(queue, webhookRepo, WorkerConfig{
		PollInterval: 100 * time.Millisecond,
	})

	worker.Start()

	// Give it time to start
	time.Sleep(50 * time.Millisecond)

	err := worker.Stop()

	if err != nil {
		t.Errorf("expected no error on stop, got %v", err)
	}

	// Calling Stop again should be safe
	err = worker.Stop()

	if err != nil {
		t.Errorf("expected no error on second stop, got %v", err)
	}
}

func TestWorker_DeliversWebhook(t *testing.T) {
	// Create a test server that records requests
	var receivedPayload []byte
	var receivedSignature string
	var receivedTimestamp string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPayload, _ = io.ReadAll(r.Body)
		receivedSignature = r.Header.Get(SignatureHeader)
		receivedTimestamp = r.Header.Get(TimestampHeader)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	webhookID := uuid.New()
	queueItemID := uuid.New()

	queue := &mockQueueReader{
		items: []db.WebhookQueue{
			{
				ID:        pgtype.UUID{Bytes: queueItemID, Valid: true},
				WebhookID: pgtype.UUID{Bytes: webhookID, Valid: true},
				Payload:   []byte(`{"event":"email.received"}`),
			},
		},
	}

	webhookRepo := &mockWebhookGetter{
		webhooks: map[uuid.UUID]*domain.Webhook{
			webhookID: {
				ID:     webhookID,
				URL:    server.URL,
				Secret: "test-secret",
			},
		},
	}

	worker := NewWorker(queue, webhookRepo, WorkerConfig{
		PollInterval: 10 * time.Millisecond,
	})

	worker.Start()
	time.Sleep(50 * time.Millisecond)
	worker.Stop()

	// Verify webhook was delivered
	if len(queue.deliveredIDs) != 1 {
		t.Fatalf("expected 1 delivered item, got %d", len(queue.deliveredIDs))
	}

	if string(receivedPayload) != `{"event":"email.received"}` {
		t.Errorf("unexpected payload: %s", receivedPayload)
	}

	if receivedSignature == "" {
		t.Error("expected signature header to be set")
	}

	if receivedTimestamp == "" {
		t.Error("expected timestamp header to be set")
	}
}

func TestWorker_RetriesOnFailure(t *testing.T) {
	// Create a test server that returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	webhookID := uuid.New()
	queueItemID := uuid.New()

	queue := &mockQueueReader{
		items: []db.WebhookQueue{
			{
				ID:        pgtype.UUID{Bytes: queueItemID, Valid: true},
				WebhookID: pgtype.UUID{Bytes: webhookID, Valid: true},
				Payload:   []byte(`{"event":"email.received"}`),
			},
		},
	}

	webhookRepo := &mockWebhookGetter{
		webhooks: map[uuid.UUID]*domain.Webhook{
			webhookID: {
				ID:     webhookID,
				URL:    server.URL,
				Secret: "test-secret",
			},
		},
	}

	worker := NewWorker(queue, webhookRepo, WorkerConfig{
		PollInterval: 10 * time.Millisecond,
		MaxRetries:   3,
	})

	worker.Start()
	time.Sleep(50 * time.Millisecond)
	worker.Stop()

	// Should have scheduled a retry, not marked as delivered
	if len(queue.deliveredIDs) != 0 {
		t.Errorf("expected 0 delivered items, got %d", len(queue.deliveredIDs))
	}

	if len(queue.updatedItems) != 1 {
		t.Fatalf("expected 1 retry scheduled, got %d", len(queue.updatedItems))
	}

	// Verify retry was scheduled with backoff
	update := queue.updatedItems[0]

	if update.Status != "pending" {
		t.Errorf("expected status 'pending', got %q", update.Status)
	}

	if update.Attempts == nil || *update.Attempts != 1 {
		t.Errorf("expected attempts=1, got %v", update.Attempts)
	}
}

func TestWorker_MarksFailedWhenWebhookNotFound(t *testing.T) {
	webhookID := uuid.New()
	queueItemID := uuid.New()

	queue := &mockQueueReader{
		items: []db.WebhookQueue{
			{
				ID:        pgtype.UUID{Bytes: queueItemID, Valid: true},
				WebhookID: pgtype.UUID{Bytes: webhookID, Valid: true},
				Payload:   []byte(`{"event":"email.received"}`),
			},
		},
	}

	// No webhooks configured - will return ErrWebhookNotFound
	webhookRepo := &mockWebhookGetter{
		webhooks: map[uuid.UUID]*domain.Webhook{},
	}

	worker := NewWorker(queue, webhookRepo, WorkerConfig{
		PollInterval: 10 * time.Millisecond,
	})

	worker.Start()
	time.Sleep(50 * time.Millisecond)
	worker.Stop()

	// Should mark as failed, not retry
	if len(queue.failedIDs) != 1 {
		t.Errorf("expected 1 failed item, got %d", len(queue.failedIDs))
	}

	if len(queue.updatedItems) != 0 {
		t.Errorf("expected 0 retry updates, got %d", len(queue.updatedItems))
	}
}
