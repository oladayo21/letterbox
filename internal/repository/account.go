package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/oladayo21/letterbox/internal/crypto"
	"github.com/oladayo21/letterbox/internal/db"
	"github.com/oladayo21/letterbox/internal/domain"
)

var (
	ErrAccountNotFound    = errors.New("account not found")
	ErrInvalidKey         = errors.New("encryption key must be 32 bytes")
	ErrEmptyImapPassword  = errors.New("imap password is required")
)

type AccountRepository struct {
	queries       *db.Queries
	encryptionKey []byte
}

func NewAccountRepository(queries *db.Queries, encryptionKey []byte) (*AccountRepository, error) {

	if len(encryptionKey) != 32 {
		return nil, fmt.Errorf("%w: got %d bytes", ErrInvalidKey, len(encryptionKey))
	}

	return &AccountRepository{
		queries:       queries,
		encryptionKey: encryptionKey,
	}, nil
}

func (r *AccountRepository) Create(ctx context.Context, input domain.CreateAccountInput) (*domain.Account, error) {

	if input.ImapPassword == "" {
		return nil, ErrEmptyImapPassword
	}

	encryptedImapPass, err := crypto.Encrypt(input.ImapPassword, r.encryptionKey)

	if err != nil {
		return nil, fmt.Errorf("encrypting imap password: %w", err)
	}

	var encryptedSmtpPass *string

	if input.SmtpPassword != "" {
		encrypted, err := crypto.Encrypt(input.SmtpPassword, r.encryptionKey)

		if err != nil {
			return nil, fmt.Errorf("encrypting smtp password: %w", err)
		}

		encryptedSmtpPass = &encrypted
	}

	params := db.CreateAccountParams{
		Name:         input.Name,
		ImapHost:     input.ImapHost,
		ImapPort:     int32(input.ImapPort),
		ImapUser:     input.ImapUser,
		ImapPassword: encryptedImapPass,
		SmtpHost:     ptrString(input.SmtpHost),
		SmtpPort:     ptrInt32(input.SmtpPort),
		SmtpUser:     ptrString(input.SmtpUser),
		SmtpPassword: encryptedSmtpPass,
	}

	dbAccount, err := r.queries.CreateAccount(ctx, params)

	if err != nil {
		return nil, fmt.Errorf("creating account: %w", err)
	}

	return r.toAccount(dbAccount)
}

func (r *AccountRepository) Get(ctx context.Context, id uuid.UUID) (*domain.Account, error) {
	dbAccount, err := r.queries.GetAccount(ctx, uuidToPgtype(id))

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAccountNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("getting account: %w", err)
	}

	return r.toAccount(dbAccount)
}

func (r *AccountRepository) List(ctx context.Context) ([]domain.Account, error) {
	dbAccounts, err := r.queries.ListAccounts(ctx)

	if err != nil {
		return nil, fmt.Errorf("listing accounts: %w", err)
	}

	accounts := make([]domain.Account, 0, len(dbAccounts))

	for _, dbAcc := range dbAccounts {
		acc, err := r.toAccount(dbAcc)

		if err != nil {
			return nil, fmt.Errorf("account %q (id=%v): %w", dbAcc.Name, dbAcc.ID.Bytes, err)
		}

		accounts = append(accounts, *acc)
	}

	return accounts, nil
}

func (r *AccountRepository) Delete(ctx context.Context, id uuid.UUID) error {
	rows, err := r.queries.DeleteAccount(ctx, uuidToPgtype(id))

	if err != nil {
		return fmt.Errorf("deleting account: %w", err)
	}

	if rows == 0 {
		return ErrAccountNotFound
	}

	return nil
}

func (r *AccountRepository) toAccount(dbAcc db.Account) (*domain.Account, error) {
	imapPassword, err := crypto.Decrypt(dbAcc.ImapPassword, r.encryptionKey)

	if err != nil {
		return nil, fmt.Errorf("decrypting imap password: %w", err)
	}

	var smtpPassword string

	if dbAcc.SmtpPassword != nil && *dbAcc.SmtpPassword != "" {
		smtpPassword, err = crypto.Decrypt(*dbAcc.SmtpPassword, r.encryptionKey)

		if err != nil {
			return nil, fmt.Errorf("decrypting smtp password: %w", err)
		}
	}

	return &domain.Account{
		ID:           pgtypeToUUID(dbAcc.ID),
		Name:         dbAcc.Name,
		ImapHost:     dbAcc.ImapHost,
		ImapPort:     int(dbAcc.ImapPort),
		ImapUser:     dbAcc.ImapUser,
		ImapPassword: imapPassword,
		SmtpHost:     derefString(dbAcc.SmtpHost),
		SmtpPort:     derefInt32(dbAcc.SmtpPort),
		SmtpUser:     derefString(dbAcc.SmtpUser),
		SmtpPassword: smtpPassword,
		CreatedAt:    dbAcc.CreatedAt.Time,
	}, nil
}

func uuidToPgtype(id uuid.UUID) pgtype.UUID {

	return pgtype.UUID{Bytes: id, Valid: true}
}

func pgtypeToUUID(pg pgtype.UUID) uuid.UUID {

	return uuid.UUID(pg.Bytes)
}

func ptrString(s string) *string {

	if s == "" {
		return nil
	}

	return &s
}

func ptrInt32(i int) *int32 {

	if i == 0 {
		return nil
	}

	v := int32(i)

	return &v
}

func derefString(s *string) string {

	if s == nil {
		return ""
	}

	return *s
}

func derefInt32(i *int32) int {

	if i == nil {
		return 0
	}

	return int(*i)
}
