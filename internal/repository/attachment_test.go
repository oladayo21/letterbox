package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/oladayo21/letterbox/internal/domain"
	"github.com/oladayo21/letterbox/internal/repository"
)

func setupAttachmentTest(t *testing.T) (*repository.AttachmentRepository, uuid.UUID) {
	t.Helper()

	queries := setupTestDB(t)
	emailRepo := repository.NewEmailRepository(queries)
	attachmentRepo := repository.NewAttachmentRepository(queries)
	accountRepo := mustNewRepo(t, queries, testEncryptionKey)
	ctx := context.Background()

	// Create test account
	account, err := accountRepo.Create(ctx, domain.CreateAccountInput{
		Name:         "Attachment Test Account",
		ImapHost:     "imap.test.com",
		ImapPort:     993,
		ImapUser:     "test@test.com",
		ImapPassword: "secret",
	})

	if err != nil {
		t.Fatalf("creating test account: %v", err)
	}

	// Create test email
	email, err := emailRepo.Create(ctx, domain.CreateEmailInput{
		AccountID:  account.ID,
		UID:        1,
		Folder:     "INBOX",
		Subject:    "Attachment Test Email",
		Date:       time.Now(),
		ParsedText: "test",
	})

	if err != nil {
		t.Fatalf("creating test email: %v", err)
	}

	return attachmentRepo, email.ID
}

func TestAttachmentRepository_Create(t *testing.T) {
	repo, emailID := setupAttachmentTest(t)
	ctx := context.Background()

	input := domain.CreateAttachmentInput{
		EmailID:     emailID,
		Filename:    "document.pdf",
		ContentType: "application/pdf",
		Size:        1024,
		S3Key:       "attachments/123/document.pdf",
	}

	attachment, err := repo.Create(ctx, input)

	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if attachment.ID == uuid.Nil {
		t.Error("expected non-nil ID")
	}

	if attachment.Filename != input.Filename {
		t.Errorf("Filename = %q, want %q", attachment.Filename, input.Filename)
	}

	if attachment.Size != input.Size {
		t.Errorf("Size = %d, want %d", attachment.Size, input.Size)
	}
}

func TestAttachmentRepository_Get(t *testing.T) {
	repo, emailID := setupAttachmentTest(t)
	ctx := context.Background()

	input := domain.CreateAttachmentInput{
		EmailID:     emailID,
		Filename:    "image.png",
		ContentType: "image/png",
		Size:        2048,
		S3Key:       "attachments/123/image.png",
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

	if fetched.Filename != input.Filename {
		t.Errorf("Filename = %q, want %q", fetched.Filename, input.Filename)
	}
}

func TestAttachmentRepository_Get_NotFound(t *testing.T) {
	repo, _ := setupAttachmentTest(t)
	ctx := context.Background()

	_, err := repo.Get(ctx, uuid.New())

	if err != repository.ErrAttachmentNotFound {
		t.Errorf("expected ErrAttachmentNotFound, got %v", err)
	}
}

func TestAttachmentRepository_GetByEmailID(t *testing.T) {
	repo, emailID := setupAttachmentTest(t)
	ctx := context.Background()

	inputs := []domain.CreateAttachmentInput{
		{EmailID: emailID, Filename: "file1.txt", ContentType: "text/plain", Size: 100, S3Key: "a/1"},
		{EmailID: emailID, Filename: "file2.txt", ContentType: "text/plain", Size: 200, S3Key: "a/2"},
		{EmailID: emailID, Filename: "file3.txt", ContentType: "text/plain", Size: 300, S3Key: "a/3"},
	}

	for _, input := range inputs {
		_, err := repo.Create(ctx, input)

		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	attachments, err := repo.GetByEmailID(ctx, emailID)

	if err != nil {
		t.Fatalf("GetByEmailID failed: %v", err)
	}

	if len(attachments) != 3 {
		t.Errorf("len = %d, want 3", len(attachments))
	}
}

func TestAttachmentRepository_GetByEmailID_Empty(t *testing.T) {
	repo, _ := setupAttachmentTest(t)
	ctx := context.Background()

	// Query for non-existent email
	attachments, err := repo.GetByEmailID(ctx, uuid.New())

	if err != nil {
		t.Fatalf("GetByEmailID failed: %v", err)
	}

	if len(attachments) != 0 {
		t.Errorf("len = %d, want 0", len(attachments))
	}
}

func TestAttachmentRepository_DeleteByEmailID(t *testing.T) {
	repo, emailID := setupAttachmentTest(t)
	ctx := context.Background()

	// Create attachments
	for i := 0; i < 2; i++ {
		_, err := repo.Create(ctx, domain.CreateAttachmentInput{
			EmailID:     emailID,
			Filename:    "delete.txt",
			ContentType: "text/plain",
			Size:        100,
			S3Key:       "del/" + string(rune('a'+i)),
		})

		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	// Verify they exist
	attachments, _ := repo.GetByEmailID(ctx, emailID)

	if len(attachments) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(attachments))
	}

	// Delete
	err := repo.DeleteByEmailID(ctx, emailID)

	if err != nil {
		t.Fatalf("DeleteByEmailID failed: %v", err)
	}

	// Verify deleted
	attachments, _ = repo.GetByEmailID(ctx, emailID)

	if len(attachments) != 0 {
		t.Errorf("expected 0 attachments after delete, got %d", len(attachments))
	}
}
