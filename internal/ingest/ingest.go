package ingest

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/oladayo21/letterbox/internal/domain"
	"github.com/oladayo21/letterbox/internal/imap"
	"github.com/oladayo21/letterbox/internal/parser"
	"github.com/oladayo21/letterbox/internal/repository"
	"github.com/oladayo21/letterbox/internal/storage"
)

var (
	ErrAccountNotFound    = errors.New("account not found")
	ErrEmailAlreadyExists = errors.New("email already exists")
)

// Ingester orchestrates the email ingestion pipeline.
type Ingester struct {
	accountRepo    *repository.AccountRepository
	emailRepo      *repository.EmailRepository
	attachmentRepo *repository.AttachmentRepository
	storage        *storage.S3Storage
}

// NewIngester creates a new email ingester.
func NewIngester(
	accountRepo *repository.AccountRepository,
	emailRepo *repository.EmailRepository,
	attachmentRepo *repository.AttachmentRepository,
	s3 *storage.S3Storage,
) *Ingester {

	return &Ingester{
		accountRepo:    accountRepo,
		emailRepo:      emailRepo,
		attachmentRepo: attachmentRepo,
		storage:        s3,
	}
}

// IngestEmail fetches, parses, and stores an email from IMAP.
// Returns the stored email with attachments, or error if failed.
func (i *Ingester) IngestEmail(ctx context.Context, accountID uuid.UUID, folder string, uid uint32) (*domain.Email, error) {
	// 1. Get account credentials
	account, err := i.accountRepo.Get(ctx, accountID)

	if errors.Is(err, repository.ErrAccountNotFound) {
		return nil, ErrAccountNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("getting account: %w", err)
	}

	// 2. Check if email already exists (idempotency)
	exists, err := i.emailRepo.ExistsByUID(ctx, accountID, folder, int64(uid))

	if err != nil {
		return nil, fmt.Errorf("checking email exists: %w", err)
	}

	if exists {
		return nil, ErrEmailAlreadyExists
	}

	// 3. Fetch raw email from IMAP
	rawBytes, err := imap.FetchRaw(
		ctx,
		account.ImapHost,
		account.ImapPort,
		account.ImapUser,
		account.ImapPassword,
		folder,
		uid,
	)

	if err != nil {
		return nil, fmt.Errorf("fetching email from IMAP: %w", err)
	}

	// 4. Parse the raw email
	parsed, err := parser.Parse(rawBytes)

	if err != nil {
		return nil, fmt.Errorf("parsing email: %w", err)
	}

	// 5. Upload attachments to S3
	s3Keys := make([]string, 0, len(parsed.Attachments))

	for _, att := range parsed.Attachments {
		key := storage.AttachmentKey(accountID.String(), int64(uid), att.Filename)

		_, err := i.storage.Upload(ctx, key, att.Data, att.ContentType)

		if err != nil {
			// Cleanup already uploaded attachments on failure
			for _, uploadedKey := range s3Keys {
				_ = i.storage.Delete(ctx, uploadedKey)
			}

			return nil, fmt.Errorf("uploading attachment %s: %w", att.Filename, err)
		}

		s3Keys = append(s3Keys, key)
	}

	// 6. Store email in database
	emailInput := domain.CreateEmailInput{
		AccountID:  accountID,
		UID:        int64(uid),
		MessageID:  parsed.MessageID,
		Folder:     folder,
		Subject:    parsed.Subject,
		FromEmail:  parsed.From.Email,
		FromName:   parsed.From.Name,
		To:         convertAddresses(parsed.To),
		CC:         convertAddresses(parsed.CC),
		Date:       parsed.Date,
		ParsedText: parsed.Text,
		ParsedHTML: parsed.HTML,
		Raw:        string(rawBytes),
		Flags:      []string{}, // Flags will be synced separately
	}

	email, err := i.emailRepo.Create(ctx, emailInput)

	if err != nil {
		// Cleanup S3 attachments on DB failure
		for _, key := range s3Keys {
			_ = i.storage.Delete(ctx, key)
		}

		if errors.Is(err, repository.ErrEmailAlreadyExists) {
			return nil, ErrEmailAlreadyExists
		}

		return nil, fmt.Errorf("storing email: %w", err)
	}

	// 7. Store attachment metadata
	attachments := make([]domain.Attachment, 0, len(parsed.Attachments))

	for idx, att := range parsed.Attachments {
		attInput := domain.CreateAttachmentInput{
			EmailID:     email.ID,
			Filename:    att.Filename,
			ContentType: att.ContentType,
			Size:        int64(att.Size),
			S3Key:       s3Keys[idx],
		}

		stored, err := i.attachmentRepo.Create(ctx, attInput)

		if err != nil {
			// Note: Email is already stored, attachments are partially stored
			// This is acceptable for MVP - caller can retry or check attachments
			return nil, fmt.Errorf("storing attachment metadata %s: %w", att.Filename, err)
		}

		attachments = append(attachments, *stored)
	}

	email.Attachments = attachments

	return email, nil
}

// convertAddresses converts parser addresses to domain addresses.
func convertAddresses(addrs []parser.EmailAddress) []domain.EmailAddress {
	result := make([]domain.EmailAddress, len(addrs))

	for i, addr := range addrs {
		result[i] = domain.EmailAddress{
			Name:  addr.Name,
			Email: addr.Email,
		}
	}

	return result
}
