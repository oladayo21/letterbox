package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/oladayo21/letterbox/internal/db"
	"github.com/oladayo21/letterbox/internal/domain"
	"github.com/oladayo21/letterbox/internal/repository"
)

func mustNewWebhookRepo(t *testing.T, queries *db.Queries, key []byte) *repository.WebhookRepository {
	t.Helper()

	repo, err := repository.NewWebhookRepository(queries, key)

	if err != nil {
		t.Fatalf("NewWebhookRepository failed: %v", err)
	}

	return repo
}

func createTestAccount(t *testing.T, queries *db.Queries) uuid.UUID {
	t.Helper()

	params := db.CreateAccountParams{
		Name:         "Test Account",
		ImapHost:     "imap.test.com",
		ImapPort:     993,
		ImapUser:     "test@test.com",
		ImapPassword: "encrypted-password",
	}

	account, err := queries.CreateAccount(context.Background(), params)

	if err != nil {
		t.Fatalf("creating test account: %v", err)
	}

	return uuid.UUID(account.ID.Bytes)
}

func cleanupWebhooks(t *testing.T, queries *db.Queries) {
	t.Helper()

	_, err := queries.ListWebhooks(context.Background())

	if err != nil {
		return
	}

	// Clean up in order: webhooks depend on accounts
	ctx := context.Background()

	_, _ = ctx, queries
}

func setupWebhookTestDB(t *testing.T) *db.Queries {
	t.Helper()

	queries := setupTestDB(t)

	// Clean webhooks table
	ctx := context.Background()
	webhooks, _ := queries.ListWebhooks(ctx)

	for _, wh := range webhooks {
		_, _ = queries.DeleteWebhook(ctx, wh.ID)
	}

	return queries
}

func TestNewWebhookRepository_InvalidKey(t *testing.T) {
	queries := setupWebhookTestDB(t)

	tests := []struct {
		name string
		key  []byte
	}{
		{"empty", []byte{}},
		{"too short", []byte("short")},
		{"too long", []byte("12345678901234567890123456789012extra")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := repository.NewWebhookRepository(queries, tt.key)

			if err == nil {
				t.Error("expected error for invalid key")
			}

			if !errors.Is(err, repository.ErrInvalidKey) {
				t.Errorf("expected ErrInvalidKey, got %v", err)
			}
		})
	}
}

func TestWebhookRepository_Create(t *testing.T) {
	queries := setupWebhookTestDB(t)
	repo := mustNewWebhookRepo(t, queries, testEncryptionKey)
	ctx := context.Background()

	accountID := createTestAccount(t, queries)

	input := domain.CreateWebhookInput{
		AccountID: accountID,
		URL:       "https://example.com/webhook",
		Secret:    "my-webhook-secret",
	}

	webhook, err := repo.Create(ctx, input)

	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if webhook.ID == uuid.Nil {
		t.Error("expected non-empty ID")
	}

	if webhook.AccountID != accountID {
		t.Errorf("account_id = %v, want %v", webhook.AccountID, accountID)
	}

	if webhook.URL != input.URL {
		t.Errorf("url = %q, want %q", webhook.URL, input.URL)
	}

	if webhook.Secret != input.Secret {
		t.Errorf("secret not decrypted correctly: got %q, want %q", webhook.Secret, input.Secret)
	}
}

func TestWebhookRepository_Create_EmptyURL(t *testing.T) {
	queries := setupWebhookTestDB(t)
	repo := mustNewWebhookRepo(t, queries, testEncryptionKey)
	ctx := context.Background()

	accountID := createTestAccount(t, queries)

	input := domain.CreateWebhookInput{
		AccountID: accountID,
		URL:       "",
		Secret:    "secret",
	}

	_, err := repo.Create(ctx, input)

	if err == nil {
		t.Error("expected error for empty URL")
	}

	if !errors.Is(err, repository.ErrEmptyURL) {
		t.Errorf("expected ErrEmptyURL, got %v", err)
	}
}

func TestWebhookRepository_Create_EmptySecret(t *testing.T) {
	queries := setupWebhookTestDB(t)
	repo := mustNewWebhookRepo(t, queries, testEncryptionKey)
	ctx := context.Background()

	accountID := createTestAccount(t, queries)

	input := domain.CreateWebhookInput{
		AccountID: accountID,
		URL:       "https://example.com/webhook",
		Secret:    "",
	}

	_, err := repo.Create(ctx, input)

	if err == nil {
		t.Error("expected error for empty secret")
	}

	if !errors.Is(err, repository.ErrEmptySecret) {
		t.Errorf("expected ErrEmptySecret, got %v", err)
	}
}

func TestWebhookRepository_Get(t *testing.T) {
	queries := setupWebhookTestDB(t)
	repo := mustNewWebhookRepo(t, queries, testEncryptionKey)
	ctx := context.Background()

	accountID := createTestAccount(t, queries)

	input := domain.CreateWebhookInput{
		AccountID: accountID,
		URL:       "https://example.com/get-test",
		Secret:    "get-test-secret",
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

	if fetched.Secret != input.Secret {
		t.Errorf("secret not decrypted: got %q, want %q", fetched.Secret, input.Secret)
	}
}

func TestWebhookRepository_Get_NotFound(t *testing.T) {
	queries := setupWebhookTestDB(t)
	repo := mustNewWebhookRepo(t, queries, testEncryptionKey)
	ctx := context.Background()

	_, err := repo.Get(ctx, uuid.New())

	if !errors.Is(err, repository.ErrWebhookNotFound) {
		t.Errorf("expected ErrWebhookNotFound, got %v", err)
	}
}

func TestWebhookRepository_Get_WrongKey(t *testing.T) {
	queries := setupWebhookTestDB(t)
	ctx := context.Background()

	accountID := createTestAccount(t, queries)

	// Create with one key
	repo1 := mustNewWebhookRepo(t, queries, testEncryptionKey)

	input := domain.CreateWebhookInput{
		AccountID: accountID,
		URL:       "https://example.com/wrong-key",
		Secret:    "secret123",
	}

	created, err := repo1.Create(ctx, input)

	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Try to read with different key
	differentKey := []byte("abcdefghijklmnopqrstuvwxyz123456")
	repo2 := mustNewWebhookRepo(t, queries, differentKey)

	_, err = repo2.Get(ctx, created.ID)

	if err == nil {
		t.Error("expected error when decrypting with wrong key")
	}
}

func TestWebhookRepository_List(t *testing.T) {
	queries := setupWebhookTestDB(t)
	repo := mustNewWebhookRepo(t, queries, testEncryptionKey)
	ctx := context.Background()

	accountID := createTestAccount(t, queries)

	inputs := []domain.CreateWebhookInput{
		{AccountID: accountID, URL: "https://example.com/hook1", Secret: "secret1"},
		{AccountID: accountID, URL: "https://example.com/hook2", Secret: "secret2"},
	}

	for _, input := range inputs {
		_, err := repo.Create(ctx, input)

		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	webhooks, err := repo.List(ctx)

	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(webhooks) != 2 {
		t.Errorf("len = %d, want 2", len(webhooks))
	}

	for _, wh := range webhooks {
		if wh.Secret == "" {
			t.Error("secret should be decrypted, got empty")
		}
	}
}

func TestWebhookRepository_List_Empty(t *testing.T) {
	queries := setupWebhookTestDB(t)
	repo := mustNewWebhookRepo(t, queries, testEncryptionKey)
	ctx := context.Background()

	webhooks, err := repo.List(ctx)

	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(webhooks) != 0 {
		t.Errorf("len = %d, want 0", len(webhooks))
	}
}

func TestWebhookRepository_GetForAccount(t *testing.T) {
	queries := setupWebhookTestDB(t)
	repo := mustNewWebhookRepo(t, queries, testEncryptionKey)
	ctx := context.Background()

	account1 := createTestAccount(t, queries)
	account2 := createTestAccount(t, queries)

	// Create webhooks for account1
	for i := 0; i < 2; i++ {
		input := domain.CreateWebhookInput{
			AccountID: account1,
			URL:       "https://example.com/acc1",
			Secret:    "secret",
		}

		_, err := repo.Create(ctx, input)

		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	// Create webhook for account2
	input := domain.CreateWebhookInput{
		AccountID: account2,
		URL:       "https://example.com/acc2",
		Secret:    "secret",
	}

	_, err := repo.Create(ctx, input)

	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Get webhooks for account1 only
	webhooks, err := repo.GetForAccount(ctx, account1)

	if err != nil {
		t.Fatalf("GetForAccount failed: %v", err)
	}

	if len(webhooks) != 2 {
		t.Errorf("len = %d, want 2", len(webhooks))
	}

	for _, wh := range webhooks {
		if wh.AccountID != account1 {
			t.Errorf("webhook account_id = %v, want %v", wh.AccountID, account1)
		}
	}
}

func TestWebhookRepository_GetForAccount_Empty(t *testing.T) {
	queries := setupWebhookTestDB(t)
	repo := mustNewWebhookRepo(t, queries, testEncryptionKey)
	ctx := context.Background()

	webhooks, err := repo.GetForAccount(ctx, uuid.New())

	if err != nil {
		t.Fatalf("GetForAccount failed: %v", err)
	}

	if len(webhooks) != 0 {
		t.Errorf("len = %d, want 0", len(webhooks))
	}
}

func TestWebhookRepository_Delete(t *testing.T) {
	queries := setupWebhookTestDB(t)
	repo := mustNewWebhookRepo(t, queries, testEncryptionKey)
	ctx := context.Background()

	accountID := createTestAccount(t, queries)

	input := domain.CreateWebhookInput{
		AccountID: accountID,
		URL:       "https://example.com/delete",
		Secret:    "to-delete",
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

	if !errors.Is(err, repository.ErrWebhookNotFound) {
		t.Errorf("expected ErrWebhookNotFound after delete, got %v", err)
	}
}

func TestWebhookRepository_Delete_NotFound(t *testing.T) {
	queries := setupWebhookTestDB(t)
	repo := mustNewWebhookRepo(t, queries, testEncryptionKey)
	ctx := context.Background()

	err := repo.Delete(ctx, uuid.New())

	if !errors.Is(err, repository.ErrWebhookNotFound) {
		t.Errorf("expected ErrWebhookNotFound, got %v", err)
	}
}

func TestWebhookRepository_SecretStoredEncrypted(t *testing.T) {
	queries := setupWebhookTestDB(t)
	repo := mustNewWebhookRepo(t, queries, testEncryptionKey)
	ctx := context.Background()

	accountID := createTestAccount(t, queries)
	plaintextSecret := "my-secret-webhook-key"

	input := domain.CreateWebhookInput{
		AccountID: accountID,
		URL:       "https://example.com/encrypted",
		Secret:    plaintextSecret,
	}

	created, err := repo.Create(ctx, input)

	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Query directly to verify encrypted storage
	pgID := pgtype.UUID{Bytes: created.ID, Valid: true}
	dbWh, err := queries.GetWebhook(ctx, pgID)

	if err != nil {
		t.Fatalf("direct query failed: %v", err)
	}

	if dbWh.Secret == plaintextSecret {
		t.Error("secret stored in plaintext, should be encrypted")
	}

	if dbWh.Secret == "" {
		t.Error("secret is empty in db")
	}
}
