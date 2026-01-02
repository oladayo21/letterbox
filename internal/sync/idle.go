package sync

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	gosync "sync"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	letterboximap "github.com/oladayo21/letterbox/internal/imap"
)

// Reuse error definitions from imap package for errors.Is() compatibility.
var (
	ErrConnectionFailed = letterboximap.ErrConnectionFailed
	ErrAuthFailed       = letterboximap.ErrAuthFailed
	ErrSelectFailed     = letterboximap.ErrSelectFailed
)

// Errors specific to IDLE connections.
var (
	ErrIdleFailed       = errors.New("failed to enter IDLE mode")
	ErrIdleNotSupported = errors.New("server does not support IDLE")
	ErrAlreadyClosed    = errors.New("connection already closed")
	ErrCapsFailed       = errors.New("failed to retrieve server capabilities")
)

const (
	defaultConnectTimeout = 30 * time.Second
)

// EventType represents the type of mailbox event.
type EventType int

const (
	// EventNewMessage indicates new message(s) arrived in the mailbox.
	EventNewMessage EventType = iota
	// EventExpunge indicates a message was deleted from the mailbox.
	EventExpunge
)

func (e EventType) String() string {
	switch e {
	case EventNewMessage:
		return "new_message"
	case EventExpunge:
		return "expunge"
	default:
		return "unknown"
	}
}

// IdleEvent represents a mailbox update received during IDLE.
type IdleEvent struct {
	Type      EventType
	AccountID string
	Folder    string

	// NumMessages is the new total message count (for EventNewMessage).
	// Only set when the server provides EXISTS response.
	NumMessages *uint32

	// SeqNum is the sequence number of the expunged message (for EventExpunge).
	SeqNum uint32
}

// IdleConfig contains configuration for an IDLE connection.
type IdleConfig struct {
	AccountID string
	Host      string
	Port      int
	Username  string
	Password  string
	Folder    string
}

// IdleConnection maintains an IMAP IDLE connection and emits events
// when the mailbox state changes.
type IdleConnection struct {
	config IdleConfig
	events chan IdleEvent

	client  *imapclient.Client
	idleCmd *imapclient.IdleCommand

	mu     gosync.Mutex
	closed bool

	// For clean shutdown
	done chan struct{}
}

// NewIdleConnection establishes an IMAP connection, logs in, selects
// the folder, and enters IDLE mode. Use Events() to receive mailbox
// updates when new messages arrive or messages are expunged.
//
// The caller must call Close() to release resources.
func NewIdleConnection(ctx context.Context, config IdleConfig) (*IdleConnection, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}

	ic := &IdleConnection{
		config: config,
		events: make(chan IdleEvent, 100),
		done:   make(chan struct{}),
	}

	if err := ic.connect(ctx); err != nil {
		return nil, err
	}

	return ic, nil
}

func (c IdleConfig) validate() error {
	if c.AccountID == "" {
		return errors.New("account_id is required")
	}

	if c.Host == "" {
		return errors.New("host is required")
	}

	if c.Port <= 0 {
		return errors.New("port must be positive")
	}

	if c.Username == "" {
		return errors.New("username is required")
	}

	if c.Password == "" {
		return errors.New("password is required")
	}

	if c.Folder == "" {
		return errors.New("folder is required")
	}

	return nil
}

// Events returns the channel on which mailbox events are emitted.
// The channel is closed when the connection is closed.
func (ic *IdleConnection) Events() <-chan IdleEvent {
	return ic.events
}

// Folder returns the folder this connection is monitoring.
func (ic *IdleConnection) Folder() string {
	return ic.config.Folder
}

// Close stops the IDLE command and closes the connection.
func (ic *IdleConnection) Close() error {
	ic.mu.Lock()
	if ic.closed {
		ic.mu.Unlock()

		return ErrAlreadyClosed
	}

	ic.closed = true
	close(ic.done)
	close(ic.events)
	ic.mu.Unlock()

	var errs []error

	if ic.idleCmd != nil {
		if err := ic.idleCmd.Close(); err != nil {
			slog.Warn("error closing IDLE command", "error", err)
			errs = append(errs, fmt.Errorf("closing IDLE: %w", err))
		}
	}

	if ic.client != nil {
		if err := ic.client.Logout().Wait(); err != nil {
			slog.Warn("error during IMAP logout", "error", err)
			errs = append(errs, fmt.Errorf("logout: %w", err))
		}

		if err := ic.client.Close(); err != nil {
			slog.Warn("error closing IMAP connection", "error", err)
			errs = append(errs, fmt.Errorf("close: %w", err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

func (ic *IdleConnection) connect(ctx context.Context) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultConnectTimeout)
		defer cancel()
	}

	addr := fmt.Sprintf("%s:%d", ic.config.Host, ic.config.Port)

	handler := &imapclient.UnilateralDataHandler{
		Mailbox: ic.handleMailboxUpdate,
		Expunge: ic.handleExpunge,
	}

	client, err := ic.dial(ctx, addr, handler)

	if err != nil {
		return fmt.Errorf("%w: %w", ErrConnectionFailed, err)
	}

	ic.client = client

	if err := client.Login(ic.config.Username, ic.config.Password).Wait(); err != nil {
		client.Close()

		return fmt.Errorf("%w: %w", ErrAuthFailed, err)
	}

	// Check for IDLE capability
	caps := client.Caps()

	if caps == nil {
		client.Logout().Wait()
		client.Close()

		return ErrCapsFailed
	}

	if !caps.Has(imap.CapIdle) {
		client.Logout().Wait()
		client.Close()

		return ErrIdleNotSupported
	}

	// Select the folder
	if _, err := client.Select(ic.config.Folder, nil).Wait(); err != nil {
		client.Logout().Wait()
		client.Close()

		return fmt.Errorf("%w: %w", ErrSelectFailed, err)
	}

	// Enter IDLE mode
	idleCmd, err := client.Idle()

	if err != nil {
		client.Logout().Wait()
		client.Close()

		return fmt.Errorf("%w: %w", ErrIdleFailed, err)
	}

	ic.idleCmd = idleCmd

	slog.Info("IDLE connection established",
		"host", ic.config.Host,
		"folder", ic.config.Folder,
	)

	return nil
}

func (ic *IdleConnection) dial(ctx context.Context, addr string, handler *imapclient.UnilateralDataHandler) (*imapclient.Client, error) {
	dialer := &net.Dialer{}

	conn, err := dialer.DialContext(ctx, "tcp", addr)

	if err != nil {
		return nil, err
	}

	opts := &imapclient.Options{
		UnilateralDataHandler: handler,
	}

	if ic.config.Port == 993 {
		return ic.dialImplicitTLS(ctx, conn, opts)
	}

	return ic.dialStartTLS(conn, opts)
}

func (ic *IdleConnection) dialImplicitTLS(ctx context.Context, conn net.Conn, opts *imapclient.Options) (*imapclient.Client, error) {
	tlsConfig := &tls.Config{ServerName: ic.config.Host}
	tlsConn := tls.Client(conn, tlsConfig)

	if err := tlsConn.HandshakeContext(ctx); err != nil {
		conn.Close()

		return nil, err
	}

	opts.TLSConfig = tlsConfig

	return imapclient.New(tlsConn, opts), nil
}

func (ic *IdleConnection) dialStartTLS(conn net.Conn, opts *imapclient.Options) (*imapclient.Client, error) {
	opts.TLSConfig = &tls.Config{ServerName: ic.config.Host}

	client, err := imapclient.NewStartTLS(conn, opts)

	if err != nil {
		conn.Close()

		return nil, err
	}

	return client, nil
}

// handleMailboxUpdate is called by go-imap when the mailbox state changes.
func (ic *IdleConnection) handleMailboxUpdate(data *imapclient.UnilateralDataMailbox) {
	if data.NumMessages == nil {
		return
	}

	event := IdleEvent{
		Type:        EventNewMessage,
		AccountID:   ic.config.AccountID,
		Folder:      ic.config.Folder,
		NumMessages: data.NumMessages,
	}

	ic.mu.Lock()
	defer ic.mu.Unlock()

	if ic.closed {
		return
	}

	select {
	case ic.events <- event:
		slog.Debug("emitted new message event",
			"folder", ic.config.Folder,
			"num_messages", *data.NumMessages,
		)
	default:
		slog.Warn("event channel full, dropping event",
			"folder", ic.config.Folder,
			"type", event.Type.String(),
		)
	}
}

// handleExpunge is called by go-imap when a message is deleted.
func (ic *IdleConnection) handleExpunge(seqNum uint32) {
	event := IdleEvent{
		Type:      EventExpunge,
		AccountID: ic.config.AccountID,
		Folder:    ic.config.Folder,
		SeqNum:    seqNum,
	}

	ic.mu.Lock()
	defer ic.mu.Unlock()

	if ic.closed {
		return
	}

	select {
	case ic.events <- event:
		slog.Debug("emitted expunge event",
			"folder", ic.config.Folder,
			"seq_num", seqNum,
		)
	default:
		slog.Warn("event channel full, dropping event",
			"folder", ic.config.Folder,
			"type", event.Type.String(),
		)
	}
}

// HasIdleCapability checks if an IMAP server supports the IDLE extension.
// This performs a connection, capability check, and disconnects.
func HasIdleCapability(ctx context.Context, host string, port int, username, password string) (bool, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultConnectTimeout)
		defer cancel()
	}

	addr := fmt.Sprintf("%s:%d", host, port)

	dialer := &net.Dialer{}

	conn, err := dialer.DialContext(ctx, "tcp", addr)

	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrConnectionFailed, err)
	}

	var client *imapclient.Client

	if port == 993 {
		tlsConfig := &tls.Config{ServerName: host}
		tlsConn := tls.Client(conn, tlsConfig)

		if err := tlsConn.HandshakeContext(ctx); err != nil {
			conn.Close()

			return false, fmt.Errorf("TLS handshake failed: %w", err)
		}

		client = imapclient.New(tlsConn, nil)
	} else {
		opts := &imapclient.Options{
			TLSConfig: &tls.Config{ServerName: host},
		}

		client, err = imapclient.NewStartTLS(conn, opts)

		if err != nil {
			conn.Close()

			return false, fmt.Errorf("STARTTLS failed: %w", err)
		}
	}

	defer client.Close()

	if err := client.Login(username, password).Wait(); err != nil {
		return false, fmt.Errorf("%w: %v", ErrAuthFailed, err)
	}

	defer client.Logout().Wait()

	caps := client.Caps()

	if caps == nil {
		return false, ErrCapsFailed
	}

	return caps.Has(imap.CapIdle), nil
}
