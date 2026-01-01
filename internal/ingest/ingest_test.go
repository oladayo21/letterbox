package ingest_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/oladayo21/letterbox/internal/db"
	"github.com/oladayo21/letterbox/internal/domain"
	"github.com/oladayo21/letterbox/internal/ingest"
	"github.com/oladayo21/letterbox/internal/repository"
	"github.com/oladayo21/letterbox/internal/storage"
)

const (
	testS3Endpoint  = "http://localhost:9000"
	testS3Bucket    = "letterbox-test"
	testS3AccessKey = "minioadmin"
	testS3SecretKey = "minioadmin"
	testS3Region    = "us-east-1"
)

func skipIfDepsUnavailable(t *testing.T) {
	t.Helper()

	// Check database
	dbURL := os.Getenv("DATABASE_URL")

	if dbURL == "" {
		t.Skip("DATABASE_URL not set")
	}

	// Check Minio
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(testS3Endpoint + "/minio/health/live")

	if err != nil {
		t.Skipf("Minio not available: %v", err)
	}

	resp.Body.Close()
}

func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dbURL := os.Getenv("DATABASE_URL")
	pool, err := pgxpool.New(context.Background(), dbURL)

	if err != nil {
		t.Fatalf("connecting to database: %v", err)
	}

	return pool
}

func setupTestS3(t *testing.T) *storage.S3Storage {
	t.Helper()

	// Create test bucket
	client := s3.New(s3.Options{
		Region:       testS3Region,
		BaseEndpoint: aws.String(testS3Endpoint),
		Credentials:  credentials.NewStaticCredentialsProvider(testS3AccessKey, testS3SecretKey, ""),
		UsePathStyle: true,
	})

	_, _ = client.CreateBucket(context.Background(), &s3.CreateBucketInput{
		Bucket: aws.String(testS3Bucket),
	})

	s, err := storage.NewS3Storage(storage.S3Config{
		Endpoint:  testS3Endpoint,
		Bucket:    testS3Bucket,
		AccessKey: testS3AccessKey,
		SecretKey: testS3SecretKey,
		Region:    testS3Region,
	})

	if err != nil {
		t.Fatalf("creating S3 storage: %v", err)
	}

	return s
}

func TestIngester_AccountNotFound(t *testing.T) {
	skipIfDepsUnavailable(t)

	pool := setupTestDB(t)
	defer pool.Close()

	s3Storage := setupTestS3(t)
	queries := db.New(pool)

	// Create repos with a valid encryption key
	encKey := make([]byte, 32)

	for i := range encKey {
		encKey[i] = byte(i)
	}

	accountRepo, _ := repository.NewAccountRepository(queries, encKey)
	emailRepo := repository.NewEmailRepository(queries)
	attachmentRepo := repository.NewAttachmentRepository(queries)

	ingester := ingest.NewIngester(accountRepo, emailRepo, attachmentRepo, s3Storage)

	// Try to ingest with non-existent account
	_, err := ingester.IngestEmail(
		context.Background(),
		uuid.New(), // Random non-existent account
		"INBOX",
		1,
	)

	if !errors.Is(err, ingest.ErrAccountNotFound) {
		t.Errorf("expected ErrAccountNotFound, got: %v", err)
	}
}

func TestIngester_DuplicateDetection(t *testing.T) {
	skipIfDepsUnavailable(t)

	pool := setupTestDB(t)
	defer pool.Close()

	s3Storage := setupTestS3(t)
	queries := db.New(pool)

	encKey := make([]byte, 32)

	for i := range encKey {
		encKey[i] = byte(i)
	}

	accountRepo, _ := repository.NewAccountRepository(queries, encKey)
	emailRepo := repository.NewEmailRepository(queries)
	attachmentRepo := repository.NewAttachmentRepository(queries)

	ingester := ingest.NewIngester(accountRepo, emailRepo, attachmentRepo, s3Storage)

	ctx := context.Background()

	// Create a test account
	account, err := accountRepo.Create(ctx, domain.CreateAccountInput{
		Name:         "Test Account",
		ImapHost:     "imap.example.com",
		ImapPort:     993,
		ImapUser:     "test@example.com",
		ImapPassword: "password",
	})

	if err != nil {
		t.Fatalf("creating test account: %v", err)
	}

	// Pre-insert an email with specific UID
	_, err = emailRepo.Create(ctx, domain.CreateEmailInput{
		AccountID: account.ID,
		UID:       12345,
		Folder:    "INBOX",
		Subject:   "Test Email",
		FromEmail: "sender@example.com",
	})

	if err != nil {
		t.Fatalf("creating test email: %v", err)
	}

	// Try to ingest same UID - should fail with duplicate error
	_, err = ingester.IngestEmail(ctx, account.ID, "INBOX", 12345)

	if !errors.Is(err, ingest.ErrEmailAlreadyExists) {
		t.Errorf("expected ErrEmailAlreadyExists, got: %v", err)
	}
}

// TestConvertAddresses tests the address conversion helper.
// This is tested indirectly through the ingest flow, but we can add
// a direct test if needed by exposing the function or testing via IngestEmail.

// Note: Full integration tests requiring real IMAP connection are not included here.
// Those would be manual tests or require a test IMAP server (like GreenMail).
