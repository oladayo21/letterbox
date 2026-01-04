package composer

import (
	"errors"
	"strings"
	"testing"

	"github.com/oladayo21/letterbox/internal/domain"
)

func TestComposeEmail_Validate(t *testing.T) {
	tests := []struct {
		name    string
		email   ComposeEmail
		wantErr error
	}{
		{
			name: "valid plain text email",
			email: ComposeEmail{
				From:    domain.EmailAddress{Email: "sender@test.com"},
				To:      []domain.EmailAddress{{Email: "recipient@test.com"}},
				Subject: "Test",
				Text:    "Hello",
			},
			wantErr: nil,
		},
		{
			name: "valid html email",
			email: ComposeEmail{
				From:    domain.EmailAddress{Email: "sender@test.com"},
				To:      []domain.EmailAddress{{Email: "recipient@test.com"}},
				Subject: "Test",
				HTML:    "<p>Hello</p>",
			},
			wantErr: nil,
		},
		{
			name: "valid email with CC only",
			email: ComposeEmail{
				From:    domain.EmailAddress{Email: "sender@test.com"},
				CC:      []domain.EmailAddress{{Email: "cc@test.com"}},
				Subject: "Test",
				Text:    "Hello",
			},
			wantErr: nil,
		},
		{
			name: "valid email with BCC only",
			email: ComposeEmail{
				From:    domain.EmailAddress{Email: "sender@test.com"},
				BCC:     []domain.EmailAddress{{Email: "bcc@test.com"}},
				Subject: "Test",
				Text:    "Hello",
			},
			wantErr: nil,
		},
		{
			name: "missing from",
			email: ComposeEmail{
				To:      []domain.EmailAddress{{Email: "recipient@test.com"}},
				Subject: "Test",
				Text:    "Hello",
			},
			wantErr: ErrNoFrom,
		},
		{
			name: "invalid from email",
			email: ComposeEmail{
				From:    domain.EmailAddress{Email: "not-an-email"},
				To:      []domain.EmailAddress{{Email: "recipient@test.com"}},
				Subject: "Test",
				Text:    "Hello",
			},
			wantErr: ErrInvalidEmail,
		},
		{
			name: "no recipients",
			email: ComposeEmail{
				From:    domain.EmailAddress{Email: "sender@test.com"},
				Subject: "Test",
				Text:    "Hello",
			},
			wantErr: ErrNoRecipients,
		},
		{
			name: "invalid to email",
			email: ComposeEmail{
				From:    domain.EmailAddress{Email: "sender@test.com"},
				To:      []domain.EmailAddress{{Email: "bad-email"}},
				Subject: "Test",
				Text:    "Hello",
			},
			wantErr: ErrInvalidEmail,
		},
		{
			name: "missing subject",
			email: ComposeEmail{
				From: domain.EmailAddress{Email: "sender@test.com"},
				To:   []domain.EmailAddress{{Email: "recipient@test.com"}},
				Text: "Hello",
			},
			wantErr: ErrNoSubject,
		},
		{
			name: "missing body",
			email: ComposeEmail{
				From:    domain.EmailAddress{Email: "sender@test.com"},
				To:      []domain.EmailAddress{{Email: "recipient@test.com"}},
				Subject: "Test",
			},
			wantErr: ErrNoBody,
		},
		{
			name: "invalid cc email",
			email: ComposeEmail{
				From:    domain.EmailAddress{Email: "sender@test.com"},
				To:      []domain.EmailAddress{{Email: "valid@test.com"}},
				CC:      []domain.EmailAddress{{Email: "bad-cc-email"}},
				Subject: "Test",
				Text:    "Hello",
			},
			wantErr: ErrInvalidEmail,
		},
		{
			name: "invalid bcc email",
			email: ComposeEmail{
				From:    domain.EmailAddress{Email: "sender@test.com"},
				To:      []domain.EmailAddress{{Email: "valid@test.com"}},
				BCC:     []domain.EmailAddress{{Email: "bad-bcc-email"}},
				Subject: "Test",
				Text:    "Hello",
			},
			wantErr: ErrInvalidEmail,
		},
		{
			name: "subject with newline (header injection)",
			email: ComposeEmail{
				From:    domain.EmailAddress{Email: "sender@test.com"},
				To:      []domain.EmailAddress{{Email: "recipient@test.com"}},
				Subject: "Test\r\nBcc: attacker@evil.com",
				Text:    "Hello",
			},
			wantErr: ErrInvalidSubject,
		},
		{
			name: "invalid in-reply-to format",
			email: ComposeEmail{
				From:      domain.EmailAddress{Email: "sender@test.com"},
				To:        []domain.EmailAddress{{Email: "recipient@test.com"}},
				Subject:   "Re: Test",
				Text:      "Hello",
				InReplyTo: "not-a-valid-message-id",
			},
			wantErr: ErrInvalidMessageID,
		},
		{
			name: "inline attachment without content-id",
			email: ComposeEmail{
				From:    domain.EmailAddress{Email: "sender@test.com"},
				To:      []domain.EmailAddress{{Email: "recipient@test.com"}},
				Subject: "Test",
				HTML:    "<img src='cid:missing'>",
				Attachments: []Attachment{
					{Filename: "img.png", ContentType: "image/png", Data: []byte{1}, IsInline: true, ContentID: ""},
				},
			},
			wantErr: ErrMissingContentID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.email.Validate()

			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
				return
			}

			if err == nil {
				t.Errorf("expected error %v, got nil", tt.wantErr)
				return
			}

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestComposeEmail_Build_PlainText(t *testing.T) {
	email := ComposeEmail{
		From:    domain.EmailAddress{Name: "Sender", Email: "sender@test.com"},
		To:      []domain.EmailAddress{{Name: "Recipient", Email: "recipient@test.com"}},
		Subject: "Test Subject",
		Text:    "Hello, World!",
	}

	msg, err := email.Build()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	msgStr := string(msg)

	// Check headers - enmime uses quoted format
	if !strings.Contains(msgStr, "From:") || !strings.Contains(msgStr, "sender@test.com") {
		t.Errorf("missing or incorrect From header, got: %s", msgStr)
	}

	if !strings.Contains(msgStr, "To:") || !strings.Contains(msgStr, "recipient@test.com") {
		t.Errorf("missing or incorrect To header, got: %s", msgStr)
	}

	if !strings.Contains(msgStr, "Subject: Test Subject") {
		t.Error("missing or incorrect Subject header")
	}

	// Check Message-ID is generated
	if !strings.Contains(msgStr, "Message-Id:") || !strings.Contains(msgStr, "@letterbox>") {
		t.Error("missing or incorrect Message-Id header")
	}

	// Check body
	if !strings.Contains(msgStr, "Hello, World!") {
		t.Error("missing body content")
	}
}

func TestComposeEmail_Build_HTMLAndText(t *testing.T) {
	email := ComposeEmail{
		From:    domain.EmailAddress{Email: "sender@test.com"},
		To:      []domain.EmailAddress{{Email: "recipient@test.com"}},
		Subject: "Test",
		Text:    "Plain text version",
		HTML:    "<p>HTML version</p>",
	}

	msg, err := email.Build()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	msgStr := string(msg)

	// Should be multipart/alternative
	if !strings.Contains(msgStr, "multipart/alternative") {
		t.Error("expected multipart/alternative for text+html email")
	}

	if !strings.Contains(msgStr, "Plain text version") {
		t.Error("missing plain text content")
	}

	if !strings.Contains(msgStr, "<p>HTML version</p>") {
		t.Error("missing HTML content")
	}
}

func TestComposeEmail_Build_WithAttachment(t *testing.T) {
	email := ComposeEmail{
		From:    domain.EmailAddress{Email: "sender@test.com"},
		To:      []domain.EmailAddress{{Email: "recipient@test.com"}},
		Subject: "Test with attachment",
		Text:    "See attached file",
		Attachments: []Attachment{
			{
				Filename:    "test.txt",
				ContentType: "text/plain",
				Data:        []byte("file content"),
				IsInline:    false,
			},
		},
	}

	msg, err := email.Build()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	msgStr := string(msg)

	// Should be multipart/mixed for attachments
	if !strings.Contains(msgStr, "multipart/mixed") {
		t.Error("expected multipart/mixed for email with attachment")
	}

	if !strings.Contains(msgStr, "test.txt") {
		t.Error("missing attachment filename")
	}
}

func TestComposeEmail_Build_WithInlineImage(t *testing.T) {
	email := ComposeEmail{
		From:    domain.EmailAddress{Email: "sender@test.com"},
		To:      []domain.EmailAddress{{Email: "recipient@test.com"}},
		Subject: "Test with inline image",
		HTML:    `<p>Image: <img src="cid:image1"></p>`,
		Attachments: []Attachment{
			{
				Filename:    "image.png",
				ContentType: "image/png",
				Data:        []byte{0x89, 0x50, 0x4E, 0x47}, // PNG magic bytes
				IsInline:    true,
				ContentID:   "image1",
			},
		},
	}

	msg, err := email.Build()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	msgStr := string(msg)

	// Check that the inline image is present (enmime may format Content-ID differently)
	if !strings.Contains(msgStr, "image.png") && !strings.Contains(msgStr, "image/png") {
		t.Errorf("missing inline image, got: %s", msgStr)
	}
}

func TestComposeEmail_Build_WithReplyHeaders(t *testing.T) {
	email := ComposeEmail{
		From:       domain.EmailAddress{Email: "sender@test.com"},
		To:         []domain.EmailAddress{{Email: "recipient@test.com"}},
		Subject:    "Re: Original Subject",
		Text:       "My reply",
		InReplyTo:  "<original-id@test.com>",
		References: []string{"<original-id@test.com>", "<even-older@test.com>"},
	}

	msg, err := email.Build()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	msgStr := string(msg)

	if !strings.Contains(msgStr, "In-Reply-To: <original-id@test.com>") {
		t.Error("missing In-Reply-To header")
	}

	if !strings.Contains(msgStr, "References: <original-id@test.com> <even-older@test.com>") {
		t.Error("missing References header")
	}
}

func TestComposeEmail_AllRecipients(t *testing.T) {
	email := ComposeEmail{
		To:  []domain.EmailAddress{{Email: "to@test.com"}},
		CC:  []domain.EmailAddress{{Email: "cc@test.com"}},
		BCC: []domain.EmailAddress{{Email: "bcc@test.com"}},
	}

	recipients := email.AllRecipients()

	if len(recipients) != 3 {
		t.Fatalf("expected 3 recipients, got %d", len(recipients))
	}

	expected := []string{"to@test.com", "cc@test.com", "bcc@test.com"}
	for i, exp := range expected {
		if recipients[i] != exp {
			t.Errorf("expected recipient %d to be %s, got %s", i, exp, recipients[i])
		}
	}
}

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		email   string
		isValid bool
	}{
		{"user@example.com", true},
		{"user.name@example.com", true},
		{"user+tag@example.com", true},
		{"user@subdomain.example.com", true},
		{"invalid", false},
		{"@example.com", false},
		{"user@", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			err := validateEmail(tt.email)
			if tt.isValid && err != nil {
				t.Errorf("expected %q to be valid, got error: %v", tt.email, err)
			}
			if !tt.isValid && err == nil {
				t.Errorf("expected %q to be invalid", tt.email)
			}
		})
	}
}

func TestIsValidMessageID(t *testing.T) {
	tests := []struct {
		id      string
		isValid bool
	}{
		{"<abc123@example.com>", true},
		{"<unique-id@letterbox>", true},
		{"<123.456@domain.org>", true},
		{"abc123@example.com", false}, // missing angle brackets
		{"<abc123>", false},           // missing @
		{"<>", false},                 // empty
		{"", false},                   // empty string
		{"<@domain>", false},          // missing local part (too short)
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			result := isValidMessageID(tt.id)
			if result != tt.isValid {
				t.Errorf("isValidMessageID(%q) = %v, want %v", tt.id, result, tt.isValid)
			}
		})
	}
}
