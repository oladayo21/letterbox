package repository_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/oladayo21/letterbox/internal/db"
	"github.com/oladayo21/letterbox/internal/domain"
	"github.com/oladayo21/letterbox/internal/repository"
)

var testEncryptionKey = []byte("12345678901234567890123456789012") // 32 bytes

func setupTestDB(t *testing.T) *db.Queries {
	t.Helper()

	dbURL := os.Getenv("DATABASE_URL")

	if dbURL == "" {
		dbURL = "postgres://letterbox:letterbox@localhost:5434/letterbox?sslmode=disable"
	}

	pool, err := pgxpool.New(context.Background(), dbURL)

	if err != nil {
		t.Fatalf("connecting to db: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
	})

	_, err = pool.Exec(context.Background(), "DELETE FROM accounts")

	if err != nil {
		t.Fatalf("cleaning accounts table: %v", err)
	}

	return db.New(pool)
}

func TestAccountRepository_Create(t *testing.T) {
	queries := setupTestDB(t)
	repo := repository.NewAccountRepository(queries, testEncryptionKey)
	ctx := context.Background()

	input := domain.CreateAccountInput{
		Name:         "Test Account",
		ImapHost:     "imap.example.com",
		ImapPort:     993,
		ImapUser:     "user@example.com",
		ImapPassword: "secret123",
		SmtpHost:     "smtp.example.com",
		SmtpPort:     587,
		SmtpUser:     "user@example.com",
		SmtpPassword: "secret456",
	}

	account, err := repo.Create(ctx, input)

	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if account.ID.String() == "" {
		t.Error("expected non-empty ID")
	}

	if account.Name != input.Name {
		t.Errorf("name = %q, want %q", account.Name, input.Name)
	}

	if account.ImapPassword != input.ImapPassword {
		t.Errorf("imap password not decrypted correctly: got %q, want %q", account.ImapPassword, input.ImapPassword)
	}

	if account.SmtpPassword != input.SmtpPassword {
		t.Errorf("smtp password not decrypted correctly: got %q, want %q", account.SmtpPassword, input.SmtpPassword)
	}
}

func TestAccountRepository_Get(t *testing.T) {
	queries := setupTestDB(t)
	repo := repository.NewAccountRepository(queries, testEncryptionKey)
	ctx := context.Background()

	input := domain.CreateAccountInput{
		Name:         "Get Test",
		ImapHost:     "imap.test.com",
		ImapPort:     993,
		ImapUser:     "test@test.com",
		ImapPassword: "password123",
	}

	created, err := repo.Create(ctx, input)

	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	fetched, err := repo.Get(ctx, created.ID)

	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if fetched.ID != created.ID {
		t.Errorf("ID = %v, want %v", fetched.ID, created.ID)
	}

	if fetched.ImapPassword != input.ImapPassword {
		t.Errorf("password not decrypted: got %q, want %q", fetched.ImapPassword, input.ImapPassword)
	}
}

func TestAccountRepository_Get_NotFound(t *testing.T) {
	queries := setupTestDB(t)
	repo := repository.NewAccountRepository(queries, testEncryptionKey)
	ctx := context.Background()

	_, err := repo.Get(ctx, [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})

	if err != repository.ErrAccountNotFound {
		t.Errorf("expected ErrAccountNotFound, got %v", err)
	}
}

func TestAccountRepository_List(t *testing.T) {
	queries := setupTestDB(t)
	repo := repository.NewAccountRepository(queries, testEncryptionKey)
	ctx := context.Background()

	inputs := []domain.CreateAccountInput{
		{Name: "Account 1", ImapHost: "imap1.com", ImapPort: 993, ImapUser: "u1", ImapPassword: "p1"},
		{Name: "Account 2", ImapHost: "imap2.com", ImapPort: 993, ImapUser: "u2", ImapPassword: "p2"},
	}

	for _, input := range inputs {
		_, err := repo.Create(ctx, input)

		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	accounts, err := repo.List(ctx)

	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(accounts) != 2 {
		t.Errorf("len = %d, want 2", len(accounts))
	}

	for _, acc := range accounts {
		if acc.ImapPassword == "" {
			t.Error("password should be decrypted, got empty")
		}
	}
}

func TestAccountRepository_Delete(t *testing.T) {
	queries := setupTestDB(t)
	repo := repository.NewAccountRepository(queries, testEncryptionKey)
	ctx := context.Background()

	input := domain.CreateAccountInput{
		Name:         "Delete Test",
		ImapHost:     "imap.delete.com",
		ImapPort:     993,
		ImapUser:     "delete@test.com",
		ImapPassword: "todelete",
	}

	created, err := repo.Create(ctx, input)

	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	err = repo.Delete(ctx, created.ID)

	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = repo.Get(ctx, created.ID)

	if err != repository.ErrAccountNotFound {
		t.Errorf("expected ErrAccountNotFound after delete, got %v", err)
	}
}

func TestAccountRepository_Delete_NotFound(t *testing.T) {
	queries := setupTestDB(t)
	repo := repository.NewAccountRepository(queries, testEncryptionKey)
	ctx := context.Background()

	err := repo.Delete(ctx, [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})

	if err != repository.ErrAccountNotFound {
		t.Errorf("expected ErrAccountNotFound, got %v", err)
	}
}

func TestAccountRepository_PasswordsStoredEncrypted(t *testing.T) {
	queries := setupTestDB(t)
	repo := repository.NewAccountRepository(queries, testEncryptionKey)
	ctx := context.Background()

	plaintextPassword := "my-secret-password"

	input := domain.CreateAccountInput{
		Name:         "Encryption Test",
		ImapHost:     "imap.encrypt.com",
		ImapPort:     993,
		ImapUser:     "encrypt@test.com",
		ImapPassword: plaintextPassword,
	}

	created, err := repo.Create(ctx, input)

	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Query directly to verify encrypted storage
	pgID := pgtype.UUID{Bytes: created.ID, Valid: true}
	dbAcc, err := queries.GetAccount(ctx, pgID)

	if err != nil {
		t.Fatalf("direct query failed: %v", err)
	}

	if dbAcc.ImapPassword == plaintextPassword {
		t.Error("password stored in plaintext, should be encrypted")
	}

	if dbAcc.ImapPassword == "" {
		t.Error("password is empty in db")
	}
}
