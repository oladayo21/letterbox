package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/oladayo21/letterbox/internal/db"
	"github.com/oladayo21/letterbox/internal/domain"
)

var (
	ErrEmailNotFound      = errors.New("email not found")
	ErrEmailAlreadyExists = errors.New("email already exists")
)

type EmailRepository struct {
	queries *db.Queries
}

func NewEmailRepository(queries *db.Queries) *EmailRepository {

	return &EmailRepository{queries: queries}
}

func (r *EmailRepository) Create(ctx context.Context, input domain.CreateEmailInput) (*domain.Email, error) {
	// Ensure slices are not nil to avoid null in JSONB
	to := input.To

	if to == nil {
		to = []domain.EmailAddress{}
	}

	cc := input.CC

	if cc == nil {
		cc = []domain.EmailAddress{}
	}

	toJSON, err := json.Marshal(to)

	if err != nil {
		return nil, fmt.Errorf("marshaling to recipients: %w", err)
	}

	ccJSON, err := json.Marshal(cc)

	if err != nil {
		return nil, fmt.Errorf("marshaling cc recipients: %w", err)
	}

	params := db.InsertEmailParams{
		AccountID:    uuidToPgtype(input.AccountID),
		Uid:          input.UID,
		MessageID:    ptrString(input.MessageID),
		Folder:       input.Folder,
		Subject:      ptrString(input.Subject),
		FromEmail:    ptrString(input.FromEmail),
		FromName:     ptrString(input.FromName),
		ToRecipients: toJSON,
		CcRecipients: ccJSON,
		Date:         timeToPgtype(input.Date),
		ParsedText:   ptrString(input.ParsedText),
		ParsedHtml:   ptrString(input.ParsedHTML),
		Raw:          ptrString(input.Raw),
		Flags:        input.Flags,
	}

	dbEmail, err := r.queries.InsertEmail(ctx, params)

	if err != nil {
		var pgErr *pgconn.PgError

		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrEmailAlreadyExists
		}

		return nil, fmt.Errorf("inserting email: %w", err)
	}

	return r.toEmail(dbEmail)
}

func (r *EmailRepository) Get(ctx context.Context, id uuid.UUID) (*domain.Email, error) {
	dbEmail, err := r.queries.GetEmail(ctx, uuidToPgtype(id))

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEmailNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("getting email: %w", err)
	}

	return r.toEmail(dbEmail)
}

func (r *EmailRepository) GetByUID(ctx context.Context, accountID uuid.UUID, folder string, uid int64) (*domain.Email, error) {
	params := db.GetEmailByUIDParams{
		AccountID: uuidToPgtype(accountID),
		Folder:    folder,
		Uid:       uid,
	}

	dbEmail, err := r.queries.GetEmailByUID(ctx, params)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEmailNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("getting email by uid: %w", err)
	}

	return r.toEmail(dbEmail)
}

func (r *EmailRepository) List(ctx context.Context, filter domain.ListEmailsFilter) ([]domain.Email, error) {
	params := db.ListEmailsParams{
		AccountID: uuidToPgtype(filter.AccountID),
		Folder:    filter.Folder,
		Limit:     int32(filter.Limit),
		Offset:    int32(filter.Offset),
	}

	if filter.Before != nil {
		params.Before = timeToPgtype(*filter.Before)
	}

	if filter.After != nil {
		params.After = timeToPgtype(*filter.After)
	}

	dbEmails, err := r.queries.ListEmails(ctx, params)

	if err != nil {
		return nil, fmt.Errorf("listing emails: %w", err)
	}

	emails := make([]domain.Email, 0, len(dbEmails))

	for _, dbEmail := range dbEmails {
		email, err := r.toEmail(dbEmail)

		if err != nil {
			return nil, fmt.Errorf("converting email %v: %w", dbEmail.ID, err)
		}

		emails = append(emails, *email)
	}

	return emails, nil
}

func (r *EmailRepository) ExistsByUID(ctx context.Context, accountID uuid.UUID, folder string, uid int64) (bool, error) {
	params := db.EmailExistsByUIDParams{
		AccountID: uuidToPgtype(accountID),
		Folder:    folder,
		Uid:       uid,
	}

	exists, err := r.queries.EmailExistsByUID(ctx, params)

	if err != nil {
		return false, fmt.Errorf("checking email exists by uid: %w", err)
	}

	return exists, nil
}

func (r *EmailRepository) ExistsByMessageID(ctx context.Context, accountID uuid.UUID, messageID string) (bool, error) {
	params := db.EmailExistsByMessageIDParams{
		AccountID: uuidToPgtype(accountID),
		MessageID: ptrString(messageID),
	}

	exists, err := r.queries.EmailExistsByMessageID(ctx, params)

	if err != nil {
		return false, fmt.Errorf("checking email exists by message_id: %w", err)
	}

	return exists, nil
}

func (r *EmailRepository) UpdateFlags(ctx context.Context, id uuid.UUID, flags []string) error {
	params := db.UpdateEmailFlagsParams{
		ID:    uuidToPgtype(id),
		Flags: flags,
	}

	rowsAffected, err := r.queries.UpdateEmailFlags(ctx, params)

	if err != nil {
		return fmt.Errorf("updating email flags: %w", err)
	}

	if rowsAffected == 0 {
		return ErrEmailNotFound
	}

	return nil
}

func (r *EmailRepository) MarkDeletedUpstream(ctx context.Context, id uuid.UUID) error {
	rowsAffected, err := r.queries.MarkEmailDeletedUpstream(ctx, uuidToPgtype(id))

	if err != nil {
		return fmt.Errorf("marking email deleted upstream: %w", err)
	}

	if rowsAffected == 0 {
		return ErrEmailNotFound
	}

	return nil
}

func (r *EmailRepository) CountInFolder(ctx context.Context, accountID uuid.UUID, folder string) (int64, error) {
	params := db.CountEmailsInFolderParams{
		AccountID: uuidToPgtype(accountID),
		Folder:    folder,
	}

	count, err := r.queries.CountEmailsInFolder(ctx, params)

	if err != nil {
		return 0, fmt.Errorf("counting emails in folder: %w", err)
	}

	return count, nil
}

func (r *EmailRepository) Search(ctx context.Context, filter domain.SearchEmailsFilter) ([]domain.Email, error) {
	params := db.SearchEmailsParams{
		AccountID:          uuidToPgtype(filter.AccountID),
		WebsearchToTsquery: filter.Query,
		Limit:              int32(filter.Limit),
		Offset:             int32(filter.Offset),
	}

	if filter.Folder != "" {
		params.Folder = &filter.Folder
	}

	dbEmails, err := r.queries.SearchEmails(ctx, params)

	if err != nil {
		return nil, fmt.Errorf("searching emails: %w", err)
	}

	emails := make([]domain.Email, 0, len(dbEmails))

	for _, dbEmail := range dbEmails {
		email, err := r.toEmail(dbEmail)

		if err != nil {
			return nil, fmt.Errorf("converting email %v: %w", dbEmail.ID, err)
		}

		emails = append(emails, *email)
	}

	return emails, nil
}

func (r *EmailRepository) CountSearch(ctx context.Context, accountID uuid.UUID, query string, folder string) (int64, error) {
	params := db.CountSearchEmailsParams{
		AccountID:          uuidToPgtype(accountID),
		WebsearchToTsquery: query,
	}

	if folder != "" {
		params.Folder = &folder
	}

	count, err := r.queries.CountSearchEmails(ctx, params)

	if err != nil {
		return 0, fmt.Errorf("counting search results: %w", err)
	}

	return count, nil
}

func (r *EmailRepository) toEmail(dbEmail db.Email) (*domain.Email, error) {
	to := []domain.EmailAddress{}

	if len(dbEmail.ToRecipients) > 0 {

		if err := json.Unmarshal(dbEmail.ToRecipients, &to); err != nil {
			return nil, fmt.Errorf("unmarshaling to recipients: %w", err)
		}
	}

	cc := []domain.EmailAddress{}

	if len(dbEmail.CcRecipients) > 0 {

		if err := json.Unmarshal(dbEmail.CcRecipients, &cc); err != nil {
			return nil, fmt.Errorf("unmarshaling cc recipients: %w", err)
		}
	}

	deletedUpstream := false

	if dbEmail.DeletedUpstream != nil {
		deletedUpstream = *dbEmail.DeletedUpstream
	}

	flags := dbEmail.Flags

	if flags == nil {
		flags = []string{}
	}

	return &domain.Email{
		ID:              pgtypeToUUID(dbEmail.ID),
		AccountID:       pgtypeToUUID(dbEmail.AccountID),
		UID:             dbEmail.Uid,
		MessageID:       derefString(dbEmail.MessageID),
		Folder:          dbEmail.Folder,
		Subject:         derefString(dbEmail.Subject),
		FromEmail:       derefString(dbEmail.FromEmail),
		FromName:        derefString(dbEmail.FromName),
		To:              to,
		CC:              cc,
		Date:            dbEmail.Date.Time,
		ParsedText:      derefString(dbEmail.ParsedText),
		ParsedHTML:      derefString(dbEmail.ParsedHtml),
		Raw:             derefString(dbEmail.Raw),
		Flags:           flags,
		DeletedUpstream: deletedUpstream,
		CreatedAt:       dbEmail.CreatedAt.Time,
	}, nil
}

func timeToPgtype(t time.Time) pgtype.Timestamptz {

	return pgtype.Timestamptz{Time: t, Valid: !t.IsZero()}
}
