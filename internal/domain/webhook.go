package domain

import (
	"time"

	"github.com/google/uuid"
)

type Webhook struct {
	ID        uuid.UUID
	AccountID uuid.UUID
	URL       string
	Secret    string
	CreatedAt time.Time
}

type CreateWebhookInput struct {
	AccountID uuid.UUID
	URL       string
	Secret    string
}
