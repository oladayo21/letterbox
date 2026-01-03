package webhook

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/oladayo21/letterbox/internal/db"
	"github.com/oladayo21/letterbox/internal/domain"
)

// mockStorage implements AttachmentStorage for testing.
type mockStorage struct {
	downloadData map[string][]byte
	downloadErr  error
	presignedURL string
	presignedErr error

	downloadCalls int
	presignCalls  int
}

func (m *mockStorage) Download(ctx context.Context, key string) ([]byte, error) {
	m.downloadCalls++

	if m.downloadErr != nil {
		return nil, m.downloadErr
	}

	if data, ok := m.downloadData[key]; ok {
		return data, nil
	}

	return nil, errors.New("not found")
}

func (m *mockStorage) GeneratePresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	m.presignCalls++

	if m.presignedErr != nil {
		return "", m.presignedErr
	}

	return m.presignedURL, nil
}

// mockWebhookRepo implements WebhookRepository for testing.
type mockWebhookRepo struct {
	webhooks []domain.Webhook
	err      error
}

func (m *mockWebhookRepo) GetForAccount(ctx context.Context, accountID uuid.UUID) ([]domain.Webhook, error) {
	return m.webhooks, m.err
}

// mockQueueWriter implements QueueWriter for testing.
type mockQueueWriter struct {
	items []db.CreateWebhookQueueItemParams
	err   error
}

func (m *mockQueueWriter) CreateWebhookQueueItem(ctx context.Context, arg db.CreateWebhookQueueItemParams) (db.WebhookQueue, error) {
	if m.err != nil {
		return db.WebhookQueue{}, m.err
	}

	m.items = append(m.items, arg)

	return db.WebhookQueue{
		ID: pgtype.UUID{Bytes: uuid.New(), Valid: true},
	}, nil
}

func TestBuildPayload_SmallAttachment_Inlined(t *testing.T) {
	smallData := []byte("small attachment content")
	s3Key := "account/123/small.txt"

	storage := &mockStorage{
		downloadData: map[string][]byte{s3Key: smallData},
	}

	producer := NewProducer(nil, nil, storage)

	email := &domain.Email{
		ID:        uuid.New(),
		AccountID: uuid.New(),
		UID:       123,
		Subject:   "Test",
		Date:      time.Now(),
		Folder:    "INBOX",
		Attachments: []domain.Attachment{
			{
				ID:          uuid.New(),
				Filename:    "small.txt",
				ContentType: "text/plain",
				Size:        int64(len(smallData)),
				S3Key:       s3Key,
			},
		},
	}

	payload := producer.buildPayload(context.Background(), email)

	if len(payload.Email.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(payload.Email.Attachments))
	}

	att := payload.Email.Attachments[0]

	if att.URL != "" {
		t.Errorf("expected empty URL for inlined attachment, got %q", att.URL)
	}

	if att.Data == "" {
		t.Fatal("expected Data to be set for inlined attachment")
	}

	decoded, err := base64.StdEncoding.DecodeString(att.Data)

	if err != nil {
		t.Fatalf("failed to decode base64 data: %v", err)
	}

	if string(decoded) != string(smallData) {
		t.Errorf("decoded data mismatch: expected %q, got %q", smallData, decoded)
	}

	if storage.downloadCalls != 1 {
		t.Errorf("expected 1 download call, got %d", storage.downloadCalls)
	}

	if storage.presignCalls != 0 {
		t.Errorf("expected 0 presign calls, got %d", storage.presignCalls)
	}
}

func TestBuildPayload_LargeAttachment_UsesURL(t *testing.T) {
	storage := &mockStorage{
		presignedURL: "https://s3.example.com/presigned-url",
	}

	producer := NewProducer(nil, nil, storage)

	email := &domain.Email{
		ID:        uuid.New(),
		AccountID: uuid.New(),
		UID:       123,
		Subject:   "Test",
		Date:      time.Now(),
		Folder:    "INBOX",
		Attachments: []domain.Attachment{
			{
				ID:          uuid.New(),
				Filename:    "large.zip",
				ContentType: "application/zip",
				Size:        2 * 1024 * 1024, // 2MB
				S3Key:       "account/123/large.zip",
			},
		},
	}

	payload := producer.buildPayload(context.Background(), email)

	if len(payload.Email.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(payload.Email.Attachments))
	}

	att := payload.Email.Attachments[0]

	if att.Data != "" {
		t.Errorf("expected empty Data for large attachment, got %d bytes", len(att.Data))
	}

	if att.URL != "https://s3.example.com/presigned-url" {
		t.Errorf("expected presigned URL, got %q", att.URL)
	}

	if storage.downloadCalls != 0 {
		t.Errorf("expected 0 download calls, got %d", storage.downloadCalls)
	}

	if storage.presignCalls != 1 {
		t.Errorf("expected 1 presign call, got %d", storage.presignCalls)
	}
}

func TestBuildPayload_DownloadError_FallsBackToURL(t *testing.T) {
	storage := &mockStorage{
		downloadErr:  errors.New("download failed"),
		presignedURL: "https://s3.example.com/fallback-url",
	}

	producer := NewProducer(nil, nil, storage)

	email := &domain.Email{
		ID:        uuid.New(),
		AccountID: uuid.New(),
		UID:       123,
		Subject:   "Test",
		Date:      time.Now(),
		Folder:    "INBOX",
		Attachments: []domain.Attachment{
			{
				ID:          uuid.New(),
				Filename:    "small.txt",
				ContentType: "text/plain",
				Size:        100, // Small, should try download first
				S3Key:       "account/123/small.txt",
			},
		},
	}

	payload := producer.buildPayload(context.Background(), email)

	if len(payload.Email.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(payload.Email.Attachments))
	}

	att := payload.Email.Attachments[0]

	// Should fall back to URL when download fails
	if att.URL != "https://s3.example.com/fallback-url" {
		t.Errorf("expected fallback URL, got %q", att.URL)
	}

	if att.Data != "" {
		t.Errorf("expected empty Data when download failed, got %d bytes", len(att.Data))
	}
}

func TestBuildPayload_NoAttachments(t *testing.T) {
	storage := &mockStorage{}
	producer := NewProducer(nil, nil, storage)

	email := &domain.Email{
		ID:        uuid.New(),
		AccountID: uuid.New(),
		UID:       123,
		Subject:   "Test",
		Date:      time.Now(),
		Folder:    "INBOX",
	}

	payload := producer.buildPayload(context.Background(), email)

	if len(payload.Email.Attachments) != 0 {
		t.Errorf("expected 0 attachments, got %d", len(payload.Email.Attachments))
	}

	if storage.downloadCalls != 0 || storage.presignCalls != 0 {
		t.Error("expected no storage calls for email without attachments")
	}
}

func TestBuildPayload_EventType(t *testing.T) {
	producer := NewProducer(nil, nil, &mockStorage{})

	email := &domain.Email{
		ID:        uuid.New(),
		AccountID: uuid.New(),
		UID:       123,
		Subject:   "Test",
		Date:      time.Now(),
		Folder:    "INBOX",
	}

	payload := producer.buildPayload(context.Background(), email)

	if payload.Event != "email.received" {
		t.Errorf("expected event 'email.received', got %q", payload.Event)
	}
}

func TestQueueForEmail_NoWebhooks(t *testing.T) {
	webhookRepo := &mockWebhookRepo{webhooks: nil}
	queue := &mockQueueWriter{}
	producer := NewProducer(queue, webhookRepo, &mockStorage{})

	email := &domain.Email{
		ID:        uuid.New(),
		AccountID: uuid.New(),
		UID:       123,
		Subject:   "Test",
		Date:      time.Now(),
		Folder:    "INBOX",
	}

	err := producer.QueueForEmail(context.Background(), email)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if len(queue.items) != 0 {
		t.Errorf("expected 0 queue items, got %d", len(queue.items))
	}
}

func TestQueueForEmail_QueuesForEachWebhook(t *testing.T) {
	webhookRepo := &mockWebhookRepo{
		webhooks: []domain.Webhook{
			{ID: uuid.New(), URL: "https://example.com/hook1"},
			{ID: uuid.New(), URL: "https://example.com/hook2"},
		},
	}
	queue := &mockQueueWriter{}
	producer := NewProducer(queue, webhookRepo, &mockStorage{})

	email := &domain.Email{
		ID:        uuid.New(),
		AccountID: uuid.New(),
		UID:       123,
		Subject:   "Test",
		Date:      time.Now(),
		Folder:    "INBOX",
	}

	err := producer.QueueForEmail(context.Background(), email)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if len(queue.items) != 2 {
		t.Errorf("expected 2 queue items, got %d", len(queue.items))
	}
}
