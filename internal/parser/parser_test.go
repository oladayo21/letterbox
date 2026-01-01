package parser_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oladayo21/letterbox/internal/parser"
)

func readTestEmail(t *testing.T, filename string) []byte {
	t.Helper()

	path := filepath.Join("testdata", filename)
	data, err := os.ReadFile(path)

	if err != nil {
		t.Fatalf("reading test email %s: %v", filename, err)
	}

	return data
}

func TestParse_PlainText(t *testing.T) {
	raw := readTestEmail(t, "plain_text.eml")

	email, err := parser.Parse(raw)

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Check Message-ID
	if email.MessageID != "<test001@example.com>" {
		t.Errorf("MessageID = %q, want %q", email.MessageID, "<test001@example.com>")
	}

	// Check Subject
	if email.Subject != "Hello World" {
		t.Errorf("Subject = %q, want %q", email.Subject, "Hello World")
	}

	// Check From
	if email.From.Name != "John Doe" {
		t.Errorf("From.Name = %q, want %q", email.From.Name, "John Doe")
	}

	if email.From.Email != "john@example.com" {
		t.Errorf("From.Email = %q, want %q", email.From.Email, "john@example.com")
	}

	// Check To
	if len(email.To) != 1 {
		t.Fatalf("len(To) = %d, want 1", len(email.To))
	}

	if email.To[0].Name != "Jane Smith" {
		t.Errorf("To[0].Name = %q, want %q", email.To[0].Name, "Jane Smith")
	}

	if email.To[0].Email != "jane@example.com" {
		t.Errorf("To[0].Email = %q, want %q", email.To[0].Email, "jane@example.com")
	}

	// Check CC
	if len(email.CC) != 1 {
		t.Fatalf("len(CC) = %d, want 1", len(email.CC))
	}

	if email.CC[0].Name != "Bob Wilson" {
		t.Errorf("CC[0].Name = %q, want %q", email.CC[0].Name, "Bob Wilson")
	}

	// Check Date
	expectedDate := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

	if !email.Date.Equal(expectedDate) {
		t.Errorf("Date = %v, want %v", email.Date, expectedDate)
	}

	// Check body
	if !strings.Contains(email.Text, "plain text email body") {
		t.Errorf("Text body missing expected content: %q", email.Text)
	}

	// No HTML
	if email.HTML != "" {
		t.Errorf("HTML should be empty for plain text email, got %q", email.HTML)
	}
}

func TestParse_QuotedPrintable(t *testing.T) {
	raw := readTestEmail(t, "quoted_printable.eml")

	email, err := parser.Parse(raw)

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Check that quoted-printable was decoded
	// éàü should be decoded from =C3=A9=C3=A0=C3=BC
	if !strings.Contains(email.Text, "éàü") {
		t.Errorf("Text should contain decoded éàü, got: %q", email.Text)
	}

	// Check soft line break handling
	if strings.Contains(email.Text, "=\n") {
		t.Error("Soft line breaks should be removed")
	}
}

func TestParse_Base64Body(t *testing.T) {
	raw := readTestEmail(t, "base64_body.eml")

	email, err := parser.Parse(raw)

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Check that base64 was decoded
	if !strings.Contains(email.Text, "base64 encoded") {
		t.Errorf("Text should contain decoded content, got: %q", email.Text)
	}

	if !strings.Contains(email.Text, "decoded correctly") {
		t.Errorf("Text should contain 'decoded correctly', got: %q", email.Text)
	}
}

func TestParse_EncodedHeaders(t *testing.T) {
	raw := readTestEmail(t, "encoded_headers.eml")

	email, err := parser.Parse(raw)

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Check RFC 2047 decoded From name
	// =?UTF-8?B?SsO2aG4gRMO2ZQ==?= decodes to "Jöhn Döe"
	if email.From.Name != "Jöhn Döe" {
		t.Errorf("From.Name = %q, want %q", email.From.Name, "Jöhn Döe")
	}

	// Check RFC 2047 decoded To name
	// =?UTF-8?Q?M=C3=BCller?= decodes to "Müller"
	if len(email.To) != 1 || email.To[0].Name != "Müller" {
		name := ""

		if len(email.To) > 0 {
			name = email.To[0].Name
		}

		t.Errorf("To[0].Name = %q, want %q", name, "Müller")
	}

	// Check RFC 2047 decoded Subject
	// =?UTF-8?B?VGVzdCB3aXRoIMOcbWxhdXRz?= decodes to "Test with Ümlauts"
	if email.Subject != "Test with Ümlauts" {
		t.Errorf("Subject = %q, want %q", email.Subject, "Test with Ümlauts")
	}
}

func TestParse_InvalidEmail(t *testing.T) {
	_, err := parser.Parse([]byte("not a valid email"))

	// enmime returns error for completely invalid input
	if err == nil {
		t.Error("expected error for invalid email input")
	}
}

func TestParse_EmptyEmail(t *testing.T) {
	email, err := parser.Parse([]byte{})

	if err != nil {
		t.Fatalf("Parse failed on empty input: %v", err)
	}

	// Should return empty but valid struct
	if email.Subject != "" {
		t.Errorf("Subject should be empty, got %q", email.Subject)
	}
}

func TestParse_MultipleRecipients(t *testing.T) {
	raw := []byte(`Message-ID: <multi@example.com>
Date: Mon, 20 Jan 2025 12:00:00 +0000
From: sender@example.com
To: alice@example.com, Bob <bob@example.com>, charlie@example.com
Cc: dan@example.com, Eve <eve@example.com>
Subject: Multiple Recipients

Body text.
`)

	email, err := parser.Parse(raw)

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(email.To) != 3 {
		t.Errorf("len(To) = %d, want 3", len(email.To))
	}

	if len(email.CC) != 2 {
		t.Errorf("len(CC) = %d, want 2", len(email.CC))
	}

	// Check that Bob has a name
	found := false

	for _, addr := range email.To {

		if addr.Email == "bob@example.com" && addr.Name == "Bob" {
			found = true

			break
		}
	}

	if !found {
		t.Error("Expected Bob <bob@example.com> in To list")
	}
}

func TestParse_EmptySlicesNotNil(t *testing.T) {
	// Email with no To, CC, or attachments should have empty slices, not nil
	raw := []byte(`Message-ID: <empty@example.com>
Date: Mon, 20 Jan 2025 12:00:00 +0000
From: sender@example.com
Subject: No Recipients

Body.
`)

	email, err := parser.Parse(raw)

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Verify slices are empty, not nil (important for JSON serialization)
	if email.To == nil {
		t.Error("To should be empty slice, not nil")
	}

	if email.CC == nil {
		t.Error("CC should be empty slice, not nil")
	}

	if email.Attachments == nil {
		t.Error("Attachments should be empty slice, not nil")
	}

	if email.Errors == nil {
		t.Error("Errors should be empty slice, not nil")
	}

	// Verify they're actually empty
	if len(email.To) != 0 {
		t.Errorf("len(To) = %d, want 0", len(email.To))
	}
}

func TestParse_MalformedDateCollectsError(t *testing.T) {
	raw := []byte(`Message-ID: <malformed@example.com>
Date: not-a-valid-date
From: sender@example.com
Subject: Malformed Date

Body.
`)

	email, err := parser.Parse(raw)

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Date should be zero value
	if !email.Date.IsZero() {
		t.Errorf("Date should be zero for malformed input, got %v", email.Date)
	}

	// Error should be collected
	hasDateError := false

	for _, e := range email.Errors {

		if strings.Contains(e, "date") {
			hasDateError = true

			break
		}
	}

	if !hasDateError {
		t.Errorf("Expected date parsing error in Errors, got: %v", email.Errors)
	}
}

// Story 2.2b: Multipart & Attachment Tests

func TestParse_MultipartAlternative(t *testing.T) {
	raw := readTestEmail(t, "multipart_alternative.eml")

	email, err := parser.Parse(raw)

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Should have both text and HTML
	if !strings.Contains(email.Text, "plain text version") {
		t.Errorf("Text body missing expected content: %q", email.Text)
	}

	if !strings.Contains(email.HTML, "<strong>HTML</strong>") {
		t.Errorf("HTML body missing expected content: %q", email.HTML)
	}

	// No attachments
	if len(email.Attachments) != 0 {
		t.Errorf("Expected 0 attachments, got %d", len(email.Attachments))
	}
}

func TestParse_MultipartMixed(t *testing.T) {
	raw := readTestEmail(t, "multipart_mixed.eml")

	email, err := parser.Parse(raw)

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Should have text body
	if !strings.Contains(email.Text, "has an attachment") {
		t.Errorf("Text body missing expected content: %q", email.Text)
	}

	// Should have one attachment
	if len(email.Attachments) != 1 {
		t.Fatalf("Expected 1 attachment, got %d", len(email.Attachments))
	}

	att := email.Attachments[0]

	if att.Filename != "document.pdf" {
		t.Errorf("Attachment filename = %q, want %q", att.Filename, "document.pdf")
	}

	if att.ContentType != "application/pdf" {
		t.Errorf("Attachment ContentType = %q, want %q", att.ContentType, "application/pdf")
	}

	if att.IsInline {
		t.Error("Attachment should not be marked as inline")
	}

	if att.Size == 0 {
		t.Error("Attachment size should not be 0")
	}

	if len(att.Data) == 0 {
		t.Error("Attachment data should not be empty")
	}
}

func TestParse_NestedMultipart(t *testing.T) {
	raw := readTestEmail(t, "nested_multipart.eml")

	email, err := parser.Parse(raw)

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Should have both text and HTML from inner multipart/alternative
	if !strings.Contains(email.Text, "Plain text body of nested") {
		t.Errorf("Text body missing expected content: %q", email.Text)
	}

	if !strings.Contains(email.HTML, "HTML body of nested") {
		t.Errorf("HTML body missing expected content: %q", email.HTML)
	}

	// Should have one attachment from outer multipart/mixed
	if len(email.Attachments) != 1 {
		t.Fatalf("Expected 1 attachment, got %d", len(email.Attachments))
	}

	if email.Attachments[0].Filename != "notes.txt" {
		t.Errorf("Attachment filename = %q, want %q", email.Attachments[0].Filename, "notes.txt")
	}
}

func TestParse_NonASCIIFilename(t *testing.T) {
	raw := readTestEmail(t, "non_ascii_filename.eml")

	email, err := parser.Parse(raw)

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(email.Attachments) != 1 {
		t.Fatalf("Expected 1 attachment, got %d", len(email.Attachments))
	}

	// RFC 2231 encoded filename should be decoded
	// %E6%96%87%E6%A1%A3.txt = 文档.txt (Chinese for "document")
	att := email.Attachments[0]

	if att.Filename != "文档.txt" {
		t.Errorf("Attachment filename = %q, want %q", att.Filename, "文档.txt")
	}
}

func TestParse_InlineImage(t *testing.T) {
	raw := readTestEmail(t, "inline_image.eml")

	email, err := parser.Parse(raw)

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Should have HTML referencing the inline image
	if !strings.Contains(email.HTML, "cid:logo123@example.com") {
		t.Errorf("HTML should contain cid reference: %q", email.HTML)
	}

	// Should have one inline attachment
	if len(email.Attachments) != 1 {
		t.Fatalf("Expected 1 attachment, got %d", len(email.Attachments))
	}

	att := email.Attachments[0]

	if att.Filename != "logo.png" {
		t.Errorf("Inline filename = %q, want %q", att.Filename, "logo.png")
	}

	if !att.IsInline {
		t.Error("Attachment should be marked as inline")
	}

	if att.ContentID != "logo123@example.com" {
		t.Errorf("ContentID = %q, want %q", att.ContentID, "logo123@example.com")
	}

	if !strings.HasPrefix(att.ContentType, "image/png") {
		t.Errorf("ContentType = %q, want image/png", att.ContentType)
	}
}

func TestParse_MixedWithInlineAndAttachment(t *testing.T) {
	// Email with both an inline image and a regular attachment
	raw := []byte(`Message-ID: <both001@example.com>
Date: Wed, 15 Jan 2025 15:00:00 +0000
From: sender@example.com
To: recipient@example.com
Subject: Both Inline and Attachment
MIME-Version: 1.0
Content-Type: multipart/mixed; boundary="outer"

--outer
Content-Type: multipart/related; boundary="related"

--related
Content-Type: text/html; charset="utf-8"

<html><body><img src="cid:img1"></body></html>

--related
Content-Type: image/gif; name="icon.gif"
Content-Disposition: inline; filename="icon.gif"
Content-ID: <img1>
Content-Transfer-Encoding: base64

R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7

--related--

--outer
Content-Type: text/plain; name="readme.txt"
Content-Disposition: attachment; filename="readme.txt"

This is a readme file.

--outer--
`)

	email, err := parser.Parse(raw)

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Should have 2 attachments total
	if len(email.Attachments) != 2 {
		t.Fatalf("Expected 2 attachments, got %d", len(email.Attachments))
	}

	var inlineCount, attachmentCount int

	for _, att := range email.Attachments {

		if att.IsInline {
			inlineCount++

			if att.Filename != "icon.gif" {
				t.Errorf("Inline filename = %q, want icon.gif", att.Filename)
			}
		} else {
			attachmentCount++

			if att.Filename != "readme.txt" {
				t.Errorf("Attachment filename = %q, want readme.txt", att.Filename)
			}
		}
	}

	if inlineCount != 1 {
		t.Errorf("Expected 1 inline, got %d", inlineCount)
	}

	if attachmentCount != 1 {
		t.Errorf("Expected 1 attachment, got %d", attachmentCount)
	}
}
