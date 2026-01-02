package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/oladayo21/letterbox/internal/crypto"
	"github.com/oladayo21/letterbox/internal/db"
	"github.com/oladayo21/letterbox/internal/domain"
)

var (
	ErrWebhookNotFound = errors.New("webhook not found")
	ErrEmptyURL        = errors.New("webhook URL is required")
	ErrEmptySecret     = errors.New("webhook secret is required")
)

type WebhookRepository struct {
	queries       *db.Queries
	encryptionKey []byte
}

func NewWebhookRepository(queries *db.Queries, encryptionKey []byte) (*WebhookRepository, error) {

	if len(encryptionKey) != 32 {
		return nil, fmt.Errorf("%w: got %d bytes", ErrInvalidKey, len(encryptionKey))
	}

	return &WebhookRepository{
		queries:       queries,
		encryptionKey: encryptionKey,
	}, nil
}

func (r *WebhookRepository) Create(ctx context.Context, input domain.CreateWebhookInput) (*domain.Webhook, error) {

	if input.URL == "" {
		return nil, ErrEmptyURL
	}

	if input.Secret == "" {
		return nil, ErrEmptySecret
	}

	encryptedSecret, err := crypto.Encrypt(input.Secret, r.encryptionKey)

	if err != nil {
		return nil, fmt.Errorf("encrypting secret: %w", err)
	}

	params := db.CreateWebhookParams{
		AccountID: uuidToPgtype(input.AccountID),
		Url:       input.URL,
		Secret:    encryptedSecret,
	}

	dbWebhook, err := r.queries.CreateWebhook(ctx, params)

	if err != nil {
		return nil, fmt.Errorf("creating webhook: %w", err)
	}

	return r.toWebhook(dbWebhook)
}

func (r *WebhookRepository) Get(ctx context.Context, id uuid.UUID) (*domain.Webhook, error) {
	dbWebhook, err := r.queries.GetWebhook(ctx, uuidToPgtype(id))

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrWebhookNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("getting webhook: %w", err)
	}

	return r.toWebhook(dbWebhook)
}

func (r *WebhookRepository) List(ctx context.Context) ([]domain.Webhook, error) {
	dbWebhooks, err := r.queries.ListWebhooks(ctx)

	if err != nil {
		return nil, fmt.Errorf("listing webhooks: %w", err)
	}

	webhooks := make([]domain.Webhook, 0, len(dbWebhooks))

	for _, dbWh := range dbWebhooks {
		wh, err := r.toWebhook(dbWh)

		if err != nil {
			return nil, fmt.Errorf("webhook id=%v: %w", dbWh.ID.Bytes, err)
		}

		webhooks = append(webhooks, *wh)
	}

	return webhooks, nil
}

func (r *WebhookRepository) GetForAccount(ctx context.Context, accountID uuid.UUID) ([]domain.Webhook, error) {
	dbWebhooks, err := r.queries.GetWebhooksForAccount(ctx, uuidToPgtype(accountID))

	if err != nil {
		return nil, fmt.Errorf("getting webhooks for account: %w", err)
	}

	webhooks := make([]domain.Webhook, 0, len(dbWebhooks))

	for _, dbWh := range dbWebhooks {
		wh, err := r.toWebhook(dbWh)

		if err != nil {
			return nil, fmt.Errorf("webhook id=%v: %w", dbWh.ID.Bytes, err)
		}

		webhooks = append(webhooks, *wh)
	}

	return webhooks, nil
}

func (r *WebhookRepository) Delete(ctx context.Context, id uuid.UUID) error {
	rows, err := r.queries.DeleteWebhook(ctx, uuidToPgtype(id))

	if err != nil {
		return fmt.Errorf("deleting webhook: %w", err)
	}

	if rows == 0 {
		return ErrWebhookNotFound
	}

	return nil
}

func (r *WebhookRepository) toWebhook(dbWh db.Webhook) (*domain.Webhook, error) {
	secret, err := crypto.Decrypt(dbWh.Secret, r.encryptionKey)

	if err != nil {
		return nil, fmt.Errorf("decrypting secret: %w", err)
	}

	return &domain.Webhook{
		ID:        pgtypeToUUID(dbWh.ID),
		AccountID: pgtypeToUUID(dbWh.AccountID),
		URL:       dbWh.Url,
		Secret:    secret,
		CreatedAt: dbWh.CreatedAt.Time,
	}, nil
}
