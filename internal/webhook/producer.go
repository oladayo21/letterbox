package webhook

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/oladayo21/letterbox/internal/db"
	"github.com/oladayo21/letterbox/internal/domain"
)

const (
	// PresignedURLExpiry is how long presigned URLs are valid for.
	// Using 1 hour to match API expiry and limit exposure if URL leaks.
	PresignedURLExpiry = 1 * time.Hour

	// InlineAttachmentThreshold is the max size for inline attachments.
	// Attachments smaller than this are base64-encoded in the payload.
	// Attachments larger use presigned S3 URLs instead.
	InlineAttachmentThreshold = 1024 * 1024 // 1MB
)

// AttachmentStorage defines the storage operations needed by Producer.
type AttachmentStorage interface {
	Download(ctx context.Context, key string) ([]byte, error)
	GeneratePresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error)
}

// WebhookRepository defines the repository operations needed by Producer.
type WebhookRepository interface {
	GetForAccount(ctx context.Context, accountID uuid.UUID) ([]domain.Webhook, error)
}

// QueueWriter defines the queue operations needed by Producer.
type QueueWriter interface {
	CreateWebhookQueueItem(ctx context.Context, arg db.CreateWebhookQueueItemParams) (db.WebhookQueue, error)
}

// WebhookPayload is the JSON structure sent to webhook endpoints.
type WebhookPayload struct {
	Event     string       `json:"event"`
	Timestamp time.Time    `json:"timestamp"`
	AccountID uuid.UUID    `json:"account_id"`
	Email     EmailPayload `json:"email"`
}

// EmailPayload is the email data within a webhook payload.
type EmailPayload struct {
	ID          uuid.UUID             `json:"id"`
	UID         int64                 `json:"uid"`
	MessageID   string                `json:"message_id,omitempty"`
	Date        time.Time             `json:"date"`
	From        domain.EmailAddress   `json:"from"`
	To          []domain.EmailAddress `json:"to"`
	CC          []domain.EmailAddress `json:"cc,omitempty"`
	Subject     string                `json:"subject"`
	Parsed      ParsedContent         `json:"parsed"`
	Raw         string                `json:"raw,omitempty"`
	Attachments []AttachmentPayload   `json:"attachments"`
	Flags       []string              `json:"flags"`
	Folder      string                `json:"folder"`
}

// ParsedContent contains the parsed email body.
type ParsedContent struct {
	Text string `json:"text,omitempty"`
	HTML string `json:"html,omitempty"`
}

// AttachmentPayload represents an attachment in the webhook payload.
// Either URL or Data is set, not both:
// - For small attachments (<1MB): Data contains base64-encoded content
// - For large attachments (>=1MB): URL contains a presigned S3 URL
type AttachmentPayload struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	URL         string `json:"url,omitempty"`
	Data        string `json:"data,omitempty"`
}

// Producer queues webhook deliveries when new emails are ingested.
type Producer struct {
	queue       QueueWriter
	webhookRepo WebhookRepository
	storage     AttachmentStorage
}

// NewProducer creates a new webhook queue producer.
func NewProducer(
	queue QueueWriter,
	webhookRepo WebhookRepository,
	storage AttachmentStorage,
) *Producer {

	return &Producer{
		queue:       queue,
		webhookRepo: webhookRepo,
		storage:     storage,
	}
}

// QueueForEmail finds all webhooks subscribed to the email's account
// and creates queue entries for each.
func (p *Producer) QueueForEmail(ctx context.Context, email *domain.Email) error {
	// Find webhooks for this account
	webhooks, err := p.webhookRepo.GetForAccount(ctx, email.AccountID)

	if err != nil {
		return fmt.Errorf("getting webhooks for account: %w", err)
	}

	if len(webhooks) == 0 {
		slog.Debug("no webhooks registered for account",
			"account_id", email.AccountID,
			"email_id", email.ID,
		)

		return nil
	}

	// Build the payload once for all webhooks
	payload := p.buildPayload(ctx, email)

	payloadBytes, err := json.Marshal(payload)

	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}

	// Queue for each webhook
	var queuedCount int

	for _, wh := range webhooks {
		params := db.CreateWebhookQueueItemParams{
			WebhookID: pgtype.UUID{Bytes: wh.ID, Valid: true},
			EmailID:   pgtype.UUID{Bytes: email.ID, Valid: true},
			Payload:   payloadBytes,
		}

		_, err := p.queue.CreateWebhookQueueItem(ctx, params)

		if err != nil {
			slog.Error("failed to queue webhook",
				"webhook_id", wh.ID,
				"webhook_url", wh.URL,
				"email_id", email.ID,
				"account_id", email.AccountID,
				"error", err,
			)

			continue
		}

		queuedCount++

		slog.Info("queued webhook delivery",
			"webhook_id", wh.ID,
			"email_id", email.ID,
			"account_id", email.AccountID,
		)
	}

	if queuedCount == 0 {
		return fmt.Errorf("failed to queue all %d webhooks for email %s", len(webhooks), email.ID)
	}

	return nil
}

// buildPayload creates the webhook payload for an email.
func (p *Producer) buildPayload(ctx context.Context, email *domain.Email) *WebhookPayload {
	// Build attachment payloads - inline small ones, use URLs for large ones
	attachments := make([]AttachmentPayload, 0, len(email.Attachments))

	for _, att := range email.Attachments {
		payload := AttachmentPayload{
			Filename:    att.Filename,
			ContentType: att.ContentType,
			Size:        att.Size,
		}

		if att.Size < InlineAttachmentThreshold {
			// Small attachment - fetch and inline as base64
			data, err := p.storage.Download(ctx, att.S3Key)

			if err != nil {
				slog.Warn("failed to download attachment for inlining, falling back to URL",
					"attachment_id", att.ID,
					"filename", att.Filename,
					"s3_key", att.S3Key,
					"error", err,
				)

				// Fall back to presigned URL
				url, urlErr := p.storage.GeneratePresignedURL(ctx, att.S3Key, PresignedURLExpiry)

				if urlErr != nil {
					slog.Warn("failed to generate presigned URL for attachment, skipping",
						"attachment_id", att.ID,
						"filename", att.Filename,
						"error", urlErr,
					)

					continue
				}

				payload.URL = url
			} else {
				payload.Data = base64.StdEncoding.EncodeToString(data)
			}
		} else {
			// Large attachment - use presigned URL
			url, err := p.storage.GeneratePresignedURL(ctx, att.S3Key, PresignedURLExpiry)

			if err != nil {
				slog.Warn("failed to generate presigned URL for attachment, skipping",
					"attachment_id", att.ID,
					"filename", att.Filename,
					"s3_key", att.S3Key,
					"error", err,
				)

				continue
			}

			payload.URL = url
		}

		attachments = append(attachments, payload)
	}

	return &WebhookPayload{
		Event:     "email.received",
		Timestamp: time.Now().UTC(),
		AccountID: email.AccountID,
		Email: EmailPayload{
			ID:        email.ID,
			UID:       email.UID,
			MessageID: email.MessageID,
			Date:      email.Date,
			From: domain.EmailAddress{
				Name:  email.FromName,
				Email: email.FromEmail,
			},
			To:      email.To,
			CC:      email.CC,
			Subject: email.Subject,
			Parsed: ParsedContent{
				Text: email.ParsedText,
				HTML: email.ParsedHTML,
			},
			Raw:         email.Raw,
			Attachments: attachments,
			Flags:       email.Flags,
			Folder:      email.Folder,
		},
	}
}

// EventHandler returns a function compatible with sync.Coordinator's EventHandler.
func (p *Producer) EventHandler() func(ctx context.Context, email *domain.Email) {

	return func(ctx context.Context, email *domain.Email) {
		if err := p.QueueForEmail(ctx, email); err != nil {
			slog.Error("failed to queue webhooks for email",
				"email_id", email.ID,
				"account_id", email.AccountID,
				"error", err,
			)
		}
	}
}
