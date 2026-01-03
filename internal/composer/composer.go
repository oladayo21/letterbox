package composer

import (
	"bytes"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/jhillyerd/enmime/v2"

	"github.com/oladayo21/letterbox/internal/domain"
)

var (
	ErrNoRecipients = errors.New("at least one recipient is required")
	ErrNoFrom       = errors.New("from address is required")
	ErrInvalidEmail = errors.New("invalid email address")
	ErrNoSubject    = errors.New("subject is required")
	ErrNoBody       = errors.New("at least text or html body is required")
)

// Attachment represents a file to attach to an email.
type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
	IsInline    bool
	ContentID   string // For inline images, used in HTML as cid:ContentID
}

// ComposeEmail represents an email to be sent.
type ComposeEmail struct {
	From        domain.EmailAddress
	To          []domain.EmailAddress
	CC          []domain.EmailAddress
	BCC         []domain.EmailAddress
	Subject     string
	Text        string
	HTML        string
	Attachments []Attachment

	// For replies/forwards
	InReplyTo  string   // Message-ID of the email being replied to
	References []string // Chain of Message-IDs for threading
}

// Validate checks that the email has all required fields and valid addresses.
func (e *ComposeEmail) Validate() error {
	if e.From.Email == "" {
		return ErrNoFrom
	}

	if err := validateEmail(e.From.Email); err != nil {
		return fmt.Errorf("%w: from: %s", ErrInvalidEmail, e.From.Email)
	}

	if len(e.To) == 0 && len(e.CC) == 0 && len(e.BCC) == 0 {
		return ErrNoRecipients
	}

	for _, addr := range e.To {
		if err := validateEmail(addr.Email); err != nil {
			return fmt.Errorf("%w: to: %s", ErrInvalidEmail, addr.Email)
		}
	}

	for _, addr := range e.CC {
		if err := validateEmail(addr.Email); err != nil {
			return fmt.Errorf("%w: cc: %s", ErrInvalidEmail, addr.Email)
		}
	}

	for _, addr := range e.BCC {
		if err := validateEmail(addr.Email); err != nil {
			return fmt.Errorf("%w: bcc: %s", ErrInvalidEmail, addr.Email)
		}
	}

	if e.Subject == "" {
		return ErrNoSubject
	}

	if e.Text == "" && e.HTML == "" {
		return ErrNoBody
	}

	return nil
}

// Build creates an RFC 2822 compliant email message.
// Returns the raw message bytes ready for sending via SMTP.
func (e *ComposeEmail) Build() ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}

	builder := enmime.Builder().
		From(e.From.Name, e.From.Email).
		Subject(e.Subject).
		Date(time.Now())

	// Add recipients
	for _, addr := range e.To {
		builder = builder.To(addr.Name, addr.Email)
	}

	for _, addr := range e.CC {
		builder = builder.CC(addr.Name, addr.Email)
	}

	for _, addr := range e.BCC {
		builder = builder.BCC(addr.Name, addr.Email)
	}

	// Add body - enmime handles multipart/alternative automatically
	if e.Text != "" {
		builder = builder.Text([]byte(e.Text))
	}

	if e.HTML != "" {
		builder = builder.HTML([]byte(e.HTML))
	}

	// Add threading headers for replies
	if e.InReplyTo != "" {
		builder = builder.Header("In-Reply-To", e.InReplyTo)
	}

	if len(e.References) > 0 {
		builder = builder.Header("References", strings.Join(e.References, " "))
	}

	// Add attachments
	for _, att := range e.Attachments {
		if att.IsInline {
			builder = builder.AddInline(
				att.Data,
				att.ContentType,
				att.Filename,
				att.ContentID,
			)
		} else {
			builder = builder.AddAttachment(
				att.Data,
				att.ContentType,
				att.Filename,
			)
		}
	}

	// Build the message
	part, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("building email: %w", err)
	}

	var buf bytes.Buffer
	if err := part.Encode(&buf); err != nil {
		return nil, fmt.Errorf("encoding email: %w", err)
	}

	return buf.Bytes(), nil
}

// AllRecipients returns all recipient email addresses (To + CC + BCC).
// This is useful for SMTP RCPT TO commands.
func (e *ComposeEmail) AllRecipients() []string {
	var recipients []string

	for _, addr := range e.To {
		recipients = append(recipients, addr.Email)
	}

	for _, addr := range e.CC {
		recipients = append(recipients, addr.Email)
	}

	for _, addr := range e.BCC {
		recipients = append(recipients, addr.Email)
	}

	return recipients
}

// validateEmail checks if an email address is valid.
func validateEmail(email string) error {
	_, err := mail.ParseAddress(email)
	return err
}
