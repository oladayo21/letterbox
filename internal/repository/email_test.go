package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/oladayo21/letterbox/internal/domain"
	"github.com/oladayo21/letterbox/internal/repository"
)

func setupEmailTest(t *testing.T) (*repository.EmailRepository, *repository.AccountRepository, uuid.UUID) {
	t.Helper()

	queries := setupTestDB(t)
	emailRepo := repository.NewEmailRepository(queries)
	accountRepo := mustNewRepo(t, queries, testEncryptionKey)

	// Create test account
	account, err := accountRepo.Create(context.Background(), domain.CreateAccountInput{
		Name:         "Email Test Account",
		ImapHost:     "imap.test.com",
		ImapPort:     993,
		ImapUser:     "test@test.com",
		ImapPassword: "secret",
	})

	if err != nil {
		t.Fatalf("creating test account: %v", err)
	}

	return emailRepo, accountRepo, account.ID
}

func TestEmailRepository_Create(t *testing.T) {
	repo, _, accountID := setupEmailTest(t)
	ctx := context.Background()

	input := domain.CreateEmailInput{
		AccountID:  accountID,
		UID:        12345,
		MessageID:  "<test@example.com>",
		Folder:     "INBOX",
		Subject:    "Test Email",
		FromEmail:  "sender@example.com",
		FromName:   "Test Sender",
		To:         []domain.EmailAddress{{Name: "Recipient", Email: "recipient@example.com"}},
		CC:         []domain.EmailAddress{{Name: "CC User", Email: "cc@example.com"}},
		Date:       time.Now().Truncate(time.Microsecond),
		ParsedText: "This is the plain text body",
		ParsedHTML: "<p>This is the HTML body</p>",
		Raw:        "From: sender@example.com\r\nTo: recipient@example.com\r\n\r\nBody",
		Flags:      []string{"\\Seen"},
	}

	email, err := repo.Create(ctx, input)

	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if email.ID == uuid.Nil {
		t.Error("expected non-nil ID")
	}

	if email.UID != input.UID {
		t.Errorf("UID = %d, want %d", email.UID, input.UID)
	}

	if email.Subject != input.Subject {
		t.Errorf("Subject = %q, want %q", email.Subject, input.Subject)
	}

	if len(email.To) != 1 || email.To[0].Email != "recipient@example.com" {
		t.Errorf("To = %v, want single recipient", email.To)
	}

	if len(email.CC) != 1 || email.CC[0].Email != "cc@example.com" {
		t.Errorf("CC = %v, want single cc", email.CC)
	}
}

func TestEmailRepository_Get(t *testing.T) {
	repo, _, accountID := setupEmailTest(t)
	ctx := context.Background()

	input := domain.CreateEmailInput{
		AccountID:  accountID,
		UID:        100,
		MessageID:  "<get-test@example.com>",
		Folder:     "INBOX",
		Subject:    "Get Test",
		FromEmail:  "from@example.com",
		FromName:   "From Name",
		Date:       time.Now().Truncate(time.Microsecond),
		ParsedText: "Body text",
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

	if fetched.Subject != input.Subject {
		t.Errorf("Subject = %q, want %q", fetched.Subject, input.Subject)
	}
}

func TestEmailRepository_Get_NotFound(t *testing.T) {
	repo, _, _ := setupEmailTest(t)
	ctx := context.Background()

	_, err := repo.Get(ctx, uuid.New())

	if err != repository.ErrEmailNotFound {
		t.Errorf("expected ErrEmailNotFound, got %v", err)
	}
}

func TestEmailRepository_GetByUID(t *testing.T) {
	repo, _, accountID := setupEmailTest(t)
	ctx := context.Background()

	input := domain.CreateEmailInput{
		AccountID:  accountID,
		UID:        555,
		MessageID:  "<uid-test@example.com>",
		Folder:     "Sent",
		Subject:    "UID Test",
		FromEmail:  "me@example.com",
		Date:       time.Now().Truncate(time.Microsecond),
		ParsedText: "Sent message",
	}

	_, err := repo.Create(ctx, input)

	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	fetched, err := repo.GetByUID(ctx, accountID, "Sent", 555)

	if err != nil {
		t.Fatalf("GetByUID failed: %v", err)
	}

	if fetched.UID != 555 {
		t.Errorf("UID = %d, want 555", fetched.UID)
	}

	if fetched.Folder != "Sent" {
		t.Errorf("Folder = %q, want Sent", fetched.Folder)
	}
}

func TestEmailRepository_GetByUID_NotFound(t *testing.T) {
	repo, _, accountID := setupEmailTest(t)
	ctx := context.Background()

	_, err := repo.GetByUID(ctx, accountID, "INBOX", 99999)

	if err != repository.ErrEmailNotFound {
		t.Errorf("expected ErrEmailNotFound, got %v", err)
	}
}

func TestEmailRepository_ExistsByUID(t *testing.T) {
	repo, _, accountID := setupEmailTest(t)
	ctx := context.Background()

	input := domain.CreateEmailInput{
		AccountID:  accountID,
		UID:        777,
		Folder:     "INBOX",
		Subject:    "Exists Test",
		Date:       time.Now(),
		ParsedText: "test",
	}

	_, err := repo.Create(ctx, input)

	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	exists, err := repo.ExistsByUID(ctx, accountID, "INBOX", 777)

	if err != nil {
		t.Fatalf("ExistsByUID failed: %v", err)
	}

	if !exists {
		t.Error("expected email to exist")
	}

	exists, err = repo.ExistsByUID(ctx, accountID, "INBOX", 888)

	if err != nil {
		t.Fatalf("ExistsByUID failed: %v", err)
	}

	if exists {
		t.Error("expected email to not exist")
	}
}

func TestEmailRepository_ExistsByMessageID(t *testing.T) {
	repo, _, accountID := setupEmailTest(t)
	ctx := context.Background()

	messageID := "<unique-message-id@example.com>"

	input := domain.CreateEmailInput{
		AccountID:  accountID,
		UID:        1,
		MessageID:  messageID,
		Folder:     "INBOX",
		Subject:    "MessageID Test",
		Date:       time.Now(),
		ParsedText: "test",
	}

	_, err := repo.Create(ctx, input)

	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	exists, err := repo.ExistsByMessageID(ctx, accountID, messageID)

	if err != nil {
		t.Fatalf("ExistsByMessageID failed: %v", err)
	}

	if !exists {
		t.Error("expected email to exist by message_id")
	}

	exists, err = repo.ExistsByMessageID(ctx, accountID, "<nonexistent@example.com>")

	if err != nil {
		t.Fatalf("ExistsByMessageID failed: %v", err)
	}

	if exists {
		t.Error("expected email to not exist")
	}
}

func TestEmailRepository_List(t *testing.T) {
	repo, _, accountID := setupEmailTest(t)
	ctx := context.Background()

	now := time.Now()

	for i := 0; i < 5; i++ {
		input := domain.CreateEmailInput{
			AccountID:  accountID,
			UID:        int64(i + 1),
			Folder:     "INBOX",
			Subject:    "List Test " + string(rune('A'+i)),
			Date:       now.Add(time.Duration(i) * time.Hour),
			ParsedText: "test",
		}

		_, err := repo.Create(ctx, input)

		if err != nil {
			t.Fatalf("Create %d failed: %v", i, err)
		}
	}

	filter := domain.ListEmailsFilter{
		AccountID: accountID,
		Folder:    "INBOX",
		Limit:     3,
		Offset:    0,
	}

	emails, err := repo.List(ctx, filter)

	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(emails) != 3 {
		t.Errorf("len = %d, want 3", len(emails))
	}

	// Should be ordered by date DESC
	if emails[0].Date.Before(emails[1].Date) {
		t.Error("expected descending order by date")
	}
}

func TestEmailRepository_List_Pagination(t *testing.T) {
	repo, _, accountID := setupEmailTest(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		input := domain.CreateEmailInput{
			AccountID:  accountID,
			UID:        int64(i + 1),
			Folder:     "INBOX",
			Subject:    "Pagination Test",
			Date:       time.Now().Add(time.Duration(i) * time.Hour),
			ParsedText: "test",
		}

		_, err := repo.Create(ctx, input)

		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	filter := domain.ListEmailsFilter{
		AccountID: accountID,
		Folder:    "INBOX",
		Limit:     2,
		Offset:    2,
	}

	emails, err := repo.List(ctx, filter)

	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(emails) != 2 {
		t.Errorf("len = %d, want 2", len(emails))
	}
}

func TestEmailRepository_UpdateFlags(t *testing.T) {
	repo, _, accountID := setupEmailTest(t)
	ctx := context.Background()

	input := domain.CreateEmailInput{
		AccountID:  accountID,
		UID:        1,
		Folder:     "INBOX",
		Subject:    "Flags Test",
		Date:       time.Now(),
		ParsedText: "test",
		Flags:      []string{},
	}

	created, err := repo.Create(ctx, input)

	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	newFlags := []string{"\\Seen", "\\Flagged"}
	err = repo.UpdateFlags(ctx, created.ID, newFlags)

	if err != nil {
		t.Fatalf("UpdateFlags failed: %v", err)
	}

	fetched, err := repo.Get(ctx, created.ID)

	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if len(fetched.Flags) != 2 {
		t.Errorf("len(Flags) = %d, want 2", len(fetched.Flags))
	}
}

func TestEmailRepository_MarkDeletedUpstream(t *testing.T) {
	repo, _, accountID := setupEmailTest(t)
	ctx := context.Background()

	input := domain.CreateEmailInput{
		AccountID:  accountID,
		UID:        1,
		Folder:     "INBOX",
		Subject:    "Delete Test",
		Date:       time.Now(),
		ParsedText: "test",
	}

	created, err := repo.Create(ctx, input)

	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if created.DeletedUpstream {
		t.Error("expected DeletedUpstream to be false initially")
	}

	err = repo.MarkDeletedUpstream(ctx, created.ID)

	if err != nil {
		t.Fatalf("MarkDeletedUpstream failed: %v", err)
	}

	fetched, err := repo.Get(ctx, created.ID)

	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if !fetched.DeletedUpstream {
		t.Error("expected DeletedUpstream to be true")
	}
}

func TestEmailRepository_CountInFolder(t *testing.T) {
	repo, _, accountID := setupEmailTest(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		input := domain.CreateEmailInput{
			AccountID:  accountID,
			UID:        int64(i + 1),
			Folder:     "INBOX",
			Subject:    "Count Test",
			Date:       time.Now(),
			ParsedText: "test",
		}

		_, err := repo.Create(ctx, input)

		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	count, err := repo.CountInFolder(ctx, accountID, "INBOX")

	if err != nil {
		t.Fatalf("CountInFolder failed: %v", err)
	}

	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}

	count, err = repo.CountInFolder(ctx, accountID, "Sent")

	if err != nil {
		t.Fatalf("CountInFolder failed: %v", err)
	}

	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestEmailRepository_UniqueConstraint(t *testing.T) {
	repo, _, accountID := setupEmailTest(t)
	ctx := context.Background()

	input := domain.CreateEmailInput{
		AccountID:  accountID,
		UID:        123,
		Folder:     "INBOX",
		Subject:    "Duplicate Test",
		Date:       time.Now(),
		ParsedText: "test",
	}

	_, err := repo.Create(ctx, input)

	if err != nil {
		t.Fatalf("First Create failed: %v", err)
	}

	// Try to create duplicate
	_, err = repo.Create(ctx, input)

	if err != repository.ErrEmailAlreadyExists {
		t.Errorf("expected ErrEmailAlreadyExists, got %v", err)
	}
}

func TestEmailRepository_UpdateFlags_NotFound(t *testing.T) {
	repo, _, _ := setupEmailTest(t)
	ctx := context.Background()

	err := repo.UpdateFlags(ctx, uuid.New(), []string{"\\Seen"})

	if err != repository.ErrEmailNotFound {
		t.Errorf("expected ErrEmailNotFound, got %v", err)
	}
}

func TestEmailRepository_MarkDeletedUpstream_NotFound(t *testing.T) {
	repo, _, _ := setupEmailTest(t)
	ctx := context.Background()

	err := repo.MarkDeletedUpstream(ctx, uuid.New())

	if err != repository.ErrEmailNotFound {
		t.Errorf("expected ErrEmailNotFound, got %v", err)
	}
}

func TestEmailRepository_Create_NilSlices(t *testing.T) {
	repo, _, accountID := setupEmailTest(t)
	ctx := context.Background()

	// Create with nil To and CC slices
	input := domain.CreateEmailInput{
		AccountID:  accountID,
		UID:        999,
		Folder:     "INBOX",
		Subject:    "Nil Slice Test",
		Date:       time.Now(),
		ParsedText: "test",
		To:         nil,
		CC:         nil,
	}

	created, err := repo.Create(ctx, input)

	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify slices are empty, not nil
	if created.To == nil {
		t.Error("To should be empty slice, not nil")
	}

	if len(created.To) != 0 {
		t.Errorf("To length = %d, want 0", len(created.To))
	}

	if created.CC == nil {
		t.Error("CC should be empty slice, not nil")
	}

	if len(created.CC) != 0 {
		t.Errorf("CC length = %d, want 0", len(created.CC))
	}

	// Fetch and verify again
	fetched, err := repo.Get(ctx, created.ID)

	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if fetched.To == nil {
		t.Error("fetched To should be empty slice, not nil")
	}

	if fetched.CC == nil {
		t.Error("fetched CC should be empty slice, not nil")
	}
}
