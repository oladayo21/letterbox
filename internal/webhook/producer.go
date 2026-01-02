package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/oladayo21/letterbox/internal/db"
	"github.com/oladayo21/letterbox/internal/domain"
	"github.com/oladayo21/letterbox/internal/repository"
	"github.com/oladayo21/letterbox/internal/storage"
)

const (
	// PresignedURLExpiry is how long presigned URLs are valid for.
	// Using 1 hour to match API expiry and limit exposure if URL leaks.
	PresignedURLExpiry = 1 * time.Hour
)

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
	From        EmailAddressPayload   `json:"from"`
	To          []EmailAddressPayload `json:"to"`
	CC          []EmailAddressPayload `json:"cc,omitempty"`
	Subject     string                `json:"subject"`
	Parsed      ParsedContent         `json:"parsed"`
	Raw         string                `json:"raw,omitempty"`
	Attachments []AttachmentPayload   `json:"attachments"`
	Flags       []string              `json:"flags"`
	Folder      string                `json:"folder"`
}

// EmailAddressPayload represents an email address in the payload.
type EmailAddressPayload struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email"`
}

// ParsedContent contains the parsed email body.
type ParsedContent struct {
	Text string `json:"text,omitempty"`
	HTML string `json:"html,omitempty"`
}

// AttachmentPayload represents an attachment in the webhook payload.
type AttachmentPayload struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	URL         string `json:"url"`
}

// Producer queues webhook deliveries when new emails are ingested.
type Producer struct {
	queries     *db.Queries
	webhookRepo *repository.WebhookRepository
	s3          *storage.S3Storage
}

// NewProducer creates a new webhook queue producer.
func NewProducer(
	queries *db.Queries,
	webhookRepo *repository.WebhookRepository,
	s3 *storage.S3Storage,
) *Producer {

	return &Producer{
		queries:     queries,
		webhookRepo: webhookRepo,
		s3:          s3,
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

		_, err := p.queries.CreateWebhookQueueItem(ctx, params)

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
	// Build attachment payloads with presigned URLs
	attachments := make([]AttachmentPayload, 0, len(email.Attachments))

	for _, att := range email.Attachments {
		url, err := p.s3.GeneratePresignedURL(ctx, att.S3Key, PresignedURLExpiry)

		if err != nil {
			slog.Warn("failed to generate presigned URL for attachment, skipping",
				"attachment_id", att.ID,
				"filename", att.Filename,
				"s3_key", att.S3Key,
				"error", err,
			)

			continue
		}

		attachments = append(attachments, AttachmentPayload{
			Filename:    att.Filename,
			ContentType: att.ContentType,
			Size:        att.Size,
			URL:         url,
		})
	}

	// Convert To recipients
	to := make([]EmailAddressPayload, 0, len(email.To))

	for _, addr := range email.To {
		to = append(to, EmailAddressPayload{
			Name:  addr.Name,
			Email: addr.Email,
		})
	}

	// Convert CC recipients
	cc := make([]EmailAddressPayload, 0, len(email.CC))

	for _, addr := range email.CC {
		cc = append(cc, EmailAddressPayload{
			Name:  addr.Name,
			Email: addr.Email,
		})
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
			From: EmailAddressPayload{
				Name:  email.FromName,
				Email: email.FromEmail,
			},
			To:      to,
			CC:      cc,
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
