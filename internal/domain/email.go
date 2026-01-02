package domain

import (
	"time"

	"github.com/google/uuid"
)

type EmailAddress struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type Email struct {
	ID              uuid.UUID
	AccountID       uuid.UUID
	UID             int64
	MessageID       string
	Folder          string
	Subject         string
	FromEmail       string
	FromName        string
	To              []EmailAddress
	CC              []EmailAddress
	Date            time.Time
	ParsedText      string
	ParsedHTML      string
	Raw             string
	Flags           []string
	DeletedUpstream bool
	CreatedAt       time.Time
	Attachments     []Attachment
}

type Attachment struct {
	ID          uuid.UUID
	EmailID     uuid.UUID
	Filename    string
	ContentType string
	Size        int64
	S3Key       string
}

type CreateEmailInput struct {
	AccountID  uuid.UUID
	UID        int64
	MessageID  string
	Folder     string
	Subject    string
	FromEmail  string
	FromName   string
	To         []EmailAddress
	CC         []EmailAddress
	Date       time.Time
	ParsedText string
	ParsedHTML string
	Raw        string
	Flags      []string
}

type CreateAttachmentInput struct {
	EmailID     uuid.UUID
	Filename    string
	ContentType string
	Size        int64
	S3Key       string
}

type ListEmailsFilter struct {
	AccountID uuid.UUID
	Folder    string
	Limit     int
	Offset    int
	Before    *time.Time
	After     *time.Time
}

type SearchEmailsFilter struct {
	AccountID uuid.UUID
	Query     string
	Folder    string // optional, empty means all folders
	Limit     int
	Offset    int
}
