package parser

import (
	"bytes"
	"fmt"
	"time"

	"github.com/jhillyerd/enmime/v2"
)

// EmailAddress represents a parsed email address.
type EmailAddress struct {
	Name  string
	Email string
}

// Attachment represents a parsed email attachment.
type Attachment struct {
	Filename    string
	ContentType string
	Size        int
	Data        []byte
}

// ParsedEmail contains the result of parsing a raw email message.
type ParsedEmail struct {
	MessageID   string
	Subject     string
	From        EmailAddress
	To          []EmailAddress
	CC          []EmailAddress
	Date        time.Time
	Text        string
	HTML        string
	Attachments []Attachment
	Errors      []string
}

// Parse parses a raw RFC822 email message and returns structured data.
func Parse(raw []byte) (*ParsedEmail, error) {
	env, err := enmime.ReadEnvelope(bytes.NewReader(raw))

	if err != nil {
		return nil, fmt.Errorf("parsing email: %w", err)
	}

	return envToEmail(env), nil
}

func envToEmail(env *enmime.Envelope) *ParsedEmail {
	result := &ParsedEmail{
		MessageID: env.GetHeader("Message-ID"),
		Subject:   env.GetHeader("Subject"),
		Text:      env.Text,
		HTML:      env.HTML,
	}

	// Parse date
	if date, err := env.Date(); err == nil {
		result.Date = date
	}

	// Parse From address
	if fromList, err := env.AddressList("From"); err == nil && len(fromList) > 0 {
		result.From = EmailAddress{
			Name:  fromList[0].Name,
			Email: fromList[0].Address,
		}
	}

	// Parse To addresses
	if toList, err := env.AddressList("To"); err == nil {
		result.To = make([]EmailAddress, len(toList))

		for i, addr := range toList {
			result.To[i] = EmailAddress{
				Name:  addr.Name,
				Email: addr.Address,
			}
		}
	}

	// Parse CC addresses
	if ccList, err := env.AddressList("Cc"); err == nil {
		result.CC = make([]EmailAddress, len(ccList))

		for i, addr := range ccList {
			result.CC[i] = EmailAddress{
				Name:  addr.Name,
				Email: addr.Address,
			}
		}
	}

	// Extract attachments (for Story 2.2b, but structure is ready)
	for _, att := range env.Attachments {
		result.Attachments = append(result.Attachments, Attachment{
			Filename:    att.FileName,
			ContentType: att.ContentType,
			Size:        len(att.Content),
			Data:        att.Content,
		})
	}

	// Collect any parsing errors
	for _, e := range env.Errors {
		result.Errors = append(result.Errors, e.Error())
	}

	return result
}
