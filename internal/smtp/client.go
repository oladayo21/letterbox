package smtp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"time"
)

var (
	ErrConnectionFailed = errors.New("failed to connect to SMTP server")
	ErrTLSFailed        = errors.New("TLS handshake failed")
	ErrAuthFailed       = errors.New("authentication failed")
	ErrTimeout          = errors.New("connection timed out")
	ErrSendFailed       = errors.New("failed to send email")
)

const defaultTimeout = 30 * time.Second

// insecureSkipVerify is a test hook to skip TLS certificate verification.
// This should ONLY be set to true in tests.
var insecureSkipVerify = false

// Config holds SMTP server configuration.
type Config struct {
	Host     string
	Port     int
	Username string
	Password string
}

// TestConnection validates SMTP credentials by connecting and authenticating.
// It performs EHLO and AUTH but does not send any email.
func TestConnection(ctx context.Context, host string, port int, username, password string) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
	}

	client, err := dial(ctx, host, port)
	if err != nil {
		return classifyError(err)
	}

	defer client.Close()

	if err := authenticate(client, host, username, password); err != nil {
		return err
	}

	// Quit errors are not critical for connection testing
	if err := client.Quit(); err != nil {
		slog.Debug("SMTP quit error during connection test", "error", err)
	}

	return nil
}

// SendEmail sends an email message through the configured SMTP server.
// The message parameter should be a complete RFC 2822 formatted email.
func SendEmail(ctx context.Context, cfg Config, from string, to []string, message []byte) error {
	if len(to) == 0 {
		return fmt.Errorf("%w: no recipients specified", ErrSendFailed)
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
	}

	client, err := dial(ctx, cfg.Host, cfg.Port)
	if err != nil {
		return classifyError(err)
	}

	defer client.Close()

	if err := authenticate(client, cfg.Host, cfg.Username, cfg.Password); err != nil {
		return err
	}

	if err := client.Mail(from); err != nil {
		return fmt.Errorf("%w: setting sender: %v", ErrSendFailed, err)
	}

	for _, addr := range to {
		if err := client.Rcpt(addr); err != nil {
			return fmt.Errorf("%w: setting recipient %s: %v", ErrSendFailed, addr, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("%w: opening data: %v", ErrSendFailed, err)
	}

	if _, err := w.Write(message); err != nil {
		return fmt.Errorf("%w: writing message: %v", ErrSendFailed, err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("%w: closing data: %v", ErrSendFailed, err)
	}

	// Quit errors after successful send should not fail the operation
	// The email was already accepted by the server
	if err := client.Quit(); err != nil {
		slog.Debug("SMTP quit error after send", "error", err)
	}

	return nil
}

// dial connects to the SMTP server with appropriate TLS handling.
// Port 465 uses implicit TLS, port 587 uses STARTTLS.
func dial(ctx context.Context, host string, port int) (*smtp.Client, error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	dialer := &net.Dialer{}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}

	if port == 465 {
		return dialImplicitTLS(ctx, conn, host)
	}

	return dialStartTLS(conn, host)
}

// dialImplicitTLS wraps the connection in TLS before creating the SMTP client.
func dialImplicitTLS(ctx context.Context, conn net.Conn, host string) (*smtp.Client, error) {
	tlsConfig := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: insecureSkipVerify, //nolint:gosec // Only true in tests
	}
	tlsConn := tls.Client(conn, tlsConfig)

	if err := tlsConn.HandshakeContext(ctx); err != nil {
		conn.Close()
		return nil, err
	}

	client, err := smtp.NewClient(tlsConn, host)
	if err != nil {
		tlsConn.Close()
		return nil, err
	}

	return client, nil
}

// dialStartTLS creates an SMTP client and upgrades to TLS via STARTTLS.
// STARTTLS is required for security - we fail if the server doesn't support it.
func dialStartTLS(conn net.Conn, host string) (*smtp.Client, error) {
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return nil, err
	}

	ok, _ := client.Extension("STARTTLS")
	if !ok {
		client.Close()
		return nil, fmt.Errorf("%w: server does not support STARTTLS", ErrTLSFailed)
	}

	tlsConfig := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: insecureSkipVerify, //nolint:gosec // Only true in tests
	}
	if err := client.StartTLS(tlsConfig); err != nil {
		client.Close()
		return nil, err
	}

	return client, nil
}

// authenticate performs SMTP authentication using the best available mechanism.
func authenticate(client *smtp.Client, host, username, password string) error {
	if username == "" && password == "" {
		return nil
	}

	auth := selectAuth(client, host, username, password)
	if auth == nil {
		return fmt.Errorf("%w: no supported auth mechanism", ErrAuthFailed)
	}

	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("%w: %v", ErrAuthFailed, err)
	}

	return nil
}

// selectAuth chooses the best authentication mechanism based on server capabilities.
// SMTP servers advertise AUTH with space-separated mechanisms (e.g., "AUTH PLAIN LOGIN").
func selectAuth(client *smtp.Client, host, username, password string) smtp.Auth {
	ok, mechs := client.Extension("AUTH")
	if !ok {
		// No AUTH extension advertised, try PLAIN as fallback
		return smtp.PlainAuth("", username, password, host)
	}

	mechList := strings.ToUpper(mechs)

	// Prefer LOGIN for servers like Outlook that require it
	if strings.Contains(mechList, "LOGIN") {
		return &loginAuth{username: username, password: password}
	}

	// Use PLAIN if available
	if strings.Contains(mechList, "PLAIN") {
		return smtp.PlainAuth("", username, password, host)
	}

	// Fallback to PLAIN
	return smtp.PlainAuth("", username, password, host)
}

// loginAuth implements the LOGIN authentication mechanism.
type loginAuth struct {
	username string
	password string
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}

	// The server sends base64-encoded prompts, decode them
	prompt, err := base64.StdEncoding.DecodeString(string(fromServer))
	if err != nil {
		// Fall back to treating it as plain text for compatibility
		prompt = fromServer
	}

	promptStr := string(prompt)
	switch promptStr {
	case "Username:", "Username", "username:":
		return []byte(a.username), nil
	case "Password:", "Password", "password:":
		return []byte(a.password), nil
	default:
		return nil, fmt.Errorf("unexpected server prompt: %s", promptStr)
	}
}

func classifyError(err error) error {
	// If error already contains one of our sentinel errors, return as-is
	if errors.Is(err, ErrTLSFailed) || errors.Is(err, ErrAuthFailed) ||
		errors.Is(err, ErrTimeout) || errors.Is(err, ErrConnectionFailed) {
		return err
	}

	if isTimeoutError(err) {
		return fmt.Errorf("%w: %v", ErrTimeout, err)
	}

	if isTLSError(err) {
		return fmt.Errorf("%w: %v", ErrTLSFailed, err)
	}

	return fmt.Errorf("%w: %v", ErrConnectionFailed, err)
}

func isTimeoutError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func isTLSError(err error) bool {
	var recordErr *tls.RecordHeaderError
	var certVerifyErr *tls.CertificateVerificationError
	var unknownAuthErr x509.UnknownAuthorityError
	var hostnameErr x509.HostnameError
	var certInvalidErr x509.CertificateInvalidError

	return errors.As(err, &recordErr) ||
		errors.As(err, &certVerifyErr) ||
		errors.As(err, &unknownAuthErr) ||
		errors.As(err, &hostnameErr) ||
		errors.As(err, &certInvalidErr)
}
