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
)

// Default polling configuration.
const (
	DefaultPollInterval = 60 * time.Second
	minPollInterval     = 10 * time.Second
)

var (
	ErrPollerClosed = errors.New("poller is closed")
)

// pollerEntry holds state for a single polling account.
type pollerEntry struct {
	config IdleConfig

	// Last known state
	uidNext     uint32
	numMessages uint32
	initialized bool
	failedPolls int // Track consecutive failures before initialization

	// Control
	stopCh chan struct{}
	doneCh chan struct{}
}

// Poller checks accounts for new emails at regular intervals.
// Use this for accounts that don't support IMAP IDLE.
type Poller struct {
	interval time.Duration
	events   chan IdleEvent

	mu       gosync.RWMutex
	accounts map[string]*pollerEntry
	closed   bool

	ctx    context.Context
	cancel context.CancelFunc
}

// NewPoller creates a new poller with the specified interval.
// If interval is less than 10 seconds, it defaults to 60 seconds.
func NewPoller(interval time.Duration) *Poller {
	if interval < minPollInterval {
		slog.Warn("poll interval too low, using default",
			"requested", interval,
			"minimum", minPollInterval,
			"using", DefaultPollInterval,
		)
		interval = DefaultPollInterval
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Poller{
		interval: interval,
		events:   make(chan IdleEvent, 100),
		accounts: make(map[string]*pollerEntry),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Events returns the channel on which mailbox events are emitted.
// Events are the same format as IdleConnection events.
func (p *Poller) Events() <-chan IdleEvent {
	return p.events
}

// AddAccount adds an account to the poller.
func (p *Poller) AddAccount(config IdleConfig) error {
	if err := config.validate(); err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return ErrPollerClosed
	}

	if _, exists := p.accounts[config.AccountID]; exists {
		return ErrAccountExists
	}

	entry := &pollerEntry{
		config: config,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}

	p.accounts[config.AccountID] = entry

	go p.runPoller(entry)

	slog.Info("added account to poller",
		"account_id", config.AccountID,
		"host", config.Host,
		"folder", config.Folder,
		"interval", p.interval,
	)

	return nil
}

// RemoveAccount removes an account from the poller.
func (p *Poller) RemoveAccount(accountID string) error {
	p.mu.Lock()

	if p.closed {
		p.mu.Unlock()

		return ErrPollerClosed
	}

	entry, exists := p.accounts[accountID]

	if !exists {
		p.mu.Unlock()

		return ErrAccountNotFound
	}

	delete(p.accounts, accountID)
	p.mu.Unlock()

	close(entry.stopCh)
	<-entry.doneCh

	slog.Info("removed account from poller", "account_id", accountID)

	return nil
}

// TotalAccounts returns the number of accounts being polled.
func (p *Poller) TotalAccounts() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return len(p.accounts)
}

// Close stops all polling and closes the event channel.
func (p *Poller) Close() error {
	p.mu.Lock()

	if p.closed {
		p.mu.Unlock()

		return ErrAlreadyClosed
	}

	p.closed = true
	p.cancel()

	entries := make([]*pollerEntry, 0, len(p.accounts))

	for _, entry := range p.accounts {
		entries = append(entries, entry)
	}

	p.accounts = make(map[string]*pollerEntry)
	p.mu.Unlock()

	for _, entry := range entries {
		close(entry.stopCh)
	}

	for _, entry := range entries {
		<-entry.doneCh
	}

	close(p.events)

	slog.Info("poller closed")

	return nil
}

// runPoller runs the polling loop for a single account.
func (p *Poller) runPoller(entry *pollerEntry) {
	defer close(entry.doneCh)

	// Initial poll immediately
	p.poll(entry)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-entry.stopCh:
			slog.Debug("stopping poller - account removed",
				"account_id", entry.config.AccountID,
			)

			return
		case <-p.ctx.Done():
			slog.Debug("stopping poller - poller closing",
				"account_id", entry.config.AccountID,
			)

			return
		case <-ticker.C:
			p.poll(entry)
		}
	}
}

// poll checks the account for new messages.
func (p *Poller) poll(entry *pollerEntry) {
	ctx, cancel := context.WithTimeout(p.ctx, defaultConnectTimeout)
	defer cancel()

	status, err := p.fetchStatus(ctx, entry.config)

	if err != nil {
		if !entry.initialized {
			entry.failedPolls++
		}

		slog.Warn("poll failed",
			"account_id", entry.config.AccountID,
			"host", entry.config.Host,
			"error", err,
		)

		return
	}

	// First successful poll
	if !entry.initialized {
		entry.uidNext = status.uidNext
		entry.numMessages = status.numMessages
		entry.initialized = true

		// If we had failures before initializing, emit an event to trigger
		// a sync in case messages arrived during the failure period
		if entry.failedPolls > 0 && status.numMessages > 0 {
			slog.Info("poller initialized after failures, triggering sync",
				"account_id", entry.config.AccountID,
				"failed_polls", entry.failedPolls,
				"num_messages", status.numMessages,
			)

			p.emitEvent(entry, status.numMessages)
		} else {
			slog.Debug("poller initialized",
				"account_id", entry.config.AccountID,
				"uid_next", status.uidNext,
				"num_messages", status.numMessages,
			)
		}

		return
	}

	// Check for new messages
	if status.uidNext > entry.uidNext || status.numMessages > entry.numMessages {
		slog.Debug("poller detected new messages",
			"account_id", entry.config.AccountID,
			"old_uid_next", entry.uidNext,
			"new_uid_next", status.uidNext,
			"old_num_messages", entry.numMessages,
			"new_num_messages", status.numMessages,
		)

		p.emitEvent(entry, status.numMessages)
	}

	// Update state
	entry.uidNext = status.uidNext
	entry.numMessages = status.numMessages
}

// folderStatus holds the result of a STATUS command.
type folderStatus struct {
	uidNext     uint32
	numMessages uint32
}

// fetchStatus connects to IMAP and fetches folder status.
func (p *Poller) fetchStatus(ctx context.Context, config IdleConfig) (*folderStatus, error) {
	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)

	client, err := p.dial(ctx, addr, config.Host, config.Port)

	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrConnectionFailed, err)
	}

	defer client.Close()

	if err := client.Login(config.Username, config.Password).Wait(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrAuthFailed, err)
	}

	defer client.Logout().Wait()

	statusOpts := &imap.StatusOptions{
		NumMessages: true,
		UIDNext:     true,
	}

	statusData, err := client.Status(config.Folder, statusOpts).Wait()

	if err != nil {
		return nil, fmt.Errorf("STATUS failed: %w", err)
	}

	result := &folderStatus{}

	if statusData.NumMessages != nil {
		result.numMessages = *statusData.NumMessages
	}

	if statusData.UIDNext != 0 {
		result.uidNext = uint32(statusData.UIDNext)
	}

	return result, nil
}

// emitEvent sends a new message event to the events channel.
func (p *Poller) emitEvent(entry *pollerEntry, numMessages uint32) {
	event := IdleEvent{
		Type:        EventNewMessage,
		AccountID:   entry.config.AccountID,
		Folder:      entry.config.Folder,
		NumMessages: &numMessages,
	}

	p.mu.RLock()
	closed := p.closed
	p.mu.RUnlock()

	if closed {
		return
	}

	select {
	case p.events <- event:
		slog.Debug("poller emitted new message event",
			"account_id", entry.config.AccountID,
			"folder", entry.config.Folder,
			"num_messages", numMessages,
		)
	default:
		slog.Warn("poller event channel full, dropping event",
			"account_id", entry.config.AccountID,
			"folder", entry.config.Folder,
		)
	}
}

// dial connects to the IMAP server.
func (p *Poller) dial(ctx context.Context, addr, host string, port int) (*imapclient.Client, error) {
	dialer := &net.Dialer{}

	conn, err := dialer.DialContext(ctx, "tcp", addr)

	if err != nil {
		return nil, err
	}

	if port == 993 {
		tlsConfig := &tls.Config{ServerName: host}
		tlsConn := tls.Client(conn, tlsConfig)

		if err := tlsConn.HandshakeContext(ctx); err != nil {
			conn.Close()

			return nil, err
		}

		return imapclient.New(tlsConn, nil), nil
	}

	opts := &imapclient.Options{
		TLSConfig: &tls.Config{ServerName: host},
	}

	client, err := imapclient.NewStartTLS(conn, opts)

	if err != nil {
		conn.Close()

		return nil, err
	}

	return client, nil
}
