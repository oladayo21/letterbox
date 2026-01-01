package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/oladayo21/letterbox/internal/db"
	"github.com/oladayo21/letterbox/internal/domain"
)

var (
	ErrAttachmentNotFound = errors.New("attachment not found")
)

type AttachmentRepository struct {
	queries *db.Queries
}

func NewAttachmentRepository(queries *db.Queries) *AttachmentRepository {

	return &AttachmentRepository{queries: queries}
}

func (r *AttachmentRepository) Create(ctx context.Context, input domain.CreateAttachmentInput) (*domain.Attachment, error) {
	params := db.InsertAttachmentParams{
		EmailID:     uuidToPgtype(input.EmailID),
		Filename:    input.Filename,
		ContentType: input.ContentType,
		Size:        input.Size,
		S3Key:       input.S3Key,
	}

	dbAttachment, err := r.queries.InsertAttachment(ctx, params)

	if err != nil {
		return nil, fmt.Errorf("inserting attachment: %w", err)
	}

	return r.toAttachment(dbAttachment), nil
}

func (r *AttachmentRepository) Get(ctx context.Context, id uuid.UUID) (*domain.Attachment, error) {
	dbAttachment, err := r.queries.GetAttachment(ctx, uuidToPgtype(id))

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAttachmentNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("getting attachment: %w", err)
	}

	return r.toAttachment(dbAttachment), nil
}

func (r *AttachmentRepository) GetByEmailID(ctx context.Context, emailID uuid.UUID) ([]domain.Attachment, error) {
	dbAttachments, err := r.queries.GetAttachmentsByEmailID(ctx, uuidToPgtype(emailID))

	if err != nil {
		return nil, fmt.Errorf("getting attachments by email id: %w", err)
	}

	attachments := make([]domain.Attachment, len(dbAttachments))

	for i, dbAtt := range dbAttachments {
		attachments[i] = *r.toAttachment(dbAtt)
	}

	return attachments, nil
}

func (r *AttachmentRepository) DeleteByEmailID(ctx context.Context, emailID uuid.UUID) error {

	if err := r.queries.DeleteAttachmentsByEmailID(ctx, uuidToPgtype(emailID)); err != nil {
		return fmt.Errorf("deleting attachments by email id: %w", err)
	}

	return nil
}

func (r *AttachmentRepository) toAttachment(dbAtt db.Attachment) *domain.Attachment {

	return &domain.Attachment{
		ID:          pgtypeToUUID(dbAtt.ID),
		EmailID:     pgtypeToUUID(dbAtt.EmailID),
		Filename:    dbAtt.Filename,
		ContentType: dbAtt.ContentType,
		Size:        dbAtt.Size,
		S3Key:       dbAtt.S3Key,
	}
}
