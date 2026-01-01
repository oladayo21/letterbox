package domain

import (
	"time"

	"github.com/google/uuid"
)

type Account struct {
	ID           uuid.UUID
	Name         string
	ImapHost     string
	ImapPort     int
	ImapUser     string
	ImapPassword string
	SmtpHost     string
	SmtpPort     int
	SmtpUser     string
	SmtpPassword string
	CreatedAt    time.Time
}

type CreateAccountInput struct {
	Name         string
	ImapHost     string
	ImapPort     int
	ImapUser     string
	ImapPassword string
	SmtpHost     string
	SmtpPort     int
	SmtpUser     string
	SmtpPassword string
}
