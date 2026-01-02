package sync

import (
	"context"
	"errors"
	"log/slog"
	gosync "sync"
	"time"
)

var (
	ErrPoolClosed        = errors.New("pool is closed")
	ErrAccountExists     = errors.New("account already exists in pool")
	ErrAccountNotFound   = errors.New("account not found in pool")
	ErrAccountNotStarted = errors.New("account connection not started")
)

// Default backoff configuration for reconnection attempts.
const (
	initialBackoff = 1 * time.Second
	maxBackoff     = 5 * time.Minute
	backoffFactor  = 2.0
)

// IdlePool manages multiple IDLE connections, one per account.
// It handles automatic reconnection with exponential backoff and
// merges events from all connections into a single channel.
type IdlePool struct {
	events chan IdleEvent

	mu       gosync.RWMutex
	accounts map[string]*accountEntry
	closed   bool

	ctx    context.Context
	cancel context.CancelFunc
}

// accountEntry holds the state for a single account's IDLE connection.
type accountEntry struct {
	config IdleConfig
	conn   *IdleConnection

	// Reconnection state
	backoff    time.Duration
	retryCount int

	// Control channels
	stopCh chan struct{}
	doneCh chan struct{}
}

// NewIdlePool creates a new connection pool.
// Use AddAccount to add accounts and Events() to receive merged events.
// Call Close() to shut down all connections.
func NewIdlePool() *IdlePool {
	ctx, cancel := context.WithCancel(context.Background())

	return &IdlePool{
		events:   make(chan IdleEvent, 100),
		accounts: make(map[string]*accountEntry),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Events returns the channel on which all mailbox events are emitted.
// Events from all accounts are merged into this single channel.
// The channel is closed when the pool is closed.
func (p *IdlePool) Events() <-chan IdleEvent {
	return p.events
}

// AddAccount adds an account to the pool and starts its IDLE connection.
// Returns ErrAccountExists if the account is already in the pool.
func (p *IdlePool) AddAccount(config IdleConfig) error {
	if err := config.validate(); err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return ErrPoolClosed
	}

	if _, exists := p.accounts[config.AccountID]; exists {
		return ErrAccountExists
	}

	entry := &accountEntry{
		config:  config,
		backoff: initialBackoff,
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}

	p.accounts[config.AccountID] = entry

	go p.runAccount(entry)

	slog.Info("added account to IDLE pool",
		"account_id", config.AccountID,
		"host", config.Host,
		"folder", config.Folder,
	)

	return nil
}

// RemoveAccount removes an account from the pool and closes its connection.
// Returns ErrAccountNotFound if the account is not in the pool.
func (p *IdlePool) RemoveAccount(accountID string) error {
	p.mu.Lock()

	if p.closed {
		p.mu.Unlock()

		return ErrPoolClosed
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

	slog.Info("removed account from IDLE pool", "account_id", accountID)

	return nil
}

// AccountStatus returns connection status for an account.
func (p *IdlePool) AccountStatus(accountID string) (connected bool, retryCount int, err error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return false, 0, ErrPoolClosed
	}

	entry, exists := p.accounts[accountID]

	if !exists {
		return false, 0, ErrAccountNotFound
	}

	return entry.conn != nil, entry.retryCount, nil
}

// ConnectedAccounts returns the number of accounts with active connections.
func (p *IdlePool) ConnectedAccounts() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	count := 0

	for _, entry := range p.accounts {
		if entry.conn != nil {
			count++
		}
	}

	return count
}

// TotalAccounts returns the total number of accounts in the pool.
func (p *IdlePool) TotalAccounts() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return len(p.accounts)
}

// Close shuts down all connections and closes the event channel.
func (p *IdlePool) Close() error {
	p.mu.Lock()

	if p.closed {
		p.mu.Unlock()

		return ErrAlreadyClosed
	}

	p.closed = true
	p.cancel()

	entries := make([]*accountEntry, 0, len(p.accounts))

	for _, entry := range p.accounts {
		entries = append(entries, entry)
	}

	p.accounts = make(map[string]*accountEntry)
	p.mu.Unlock()

	for _, entry := range entries {
		close(entry.stopCh)
	}

	for _, entry := range entries {
		<-entry.doneCh
	}

	close(p.events)

	slog.Info("IDLE pool closed")

	return nil
}

// runAccount manages the lifecycle of a single account's IDLE connection.
// It handles connection, event forwarding, and reconnection with backoff.
func (p *IdlePool) runAccount(entry *accountEntry) {
	defer close(entry.doneCh)

	for {
		select {
		case <-entry.stopCh:
			p.closeConnection(entry)

			return
		case <-p.ctx.Done():
			p.closeConnection(entry)

			return
		default:
		}

		conn, err := NewIdleConnection(p.ctx, entry.config)

		if err != nil {
			// Non-retryable error - exit goroutine
			if errors.Is(err, ErrIdleNotSupported) {
				slog.Warn("account does not support IDLE, giving up",
					"account_id", entry.config.AccountID,
					"error", err,
				)

				return
			}

			p.handleConnectionError(entry, err)

			select {
			case <-entry.stopCh:
				return
			case <-p.ctx.Done():
				return
			case <-time.After(entry.backoff):
				continue
			}
		}

		p.mu.Lock()
		entry.conn = conn
		entry.backoff = initialBackoff
		entry.retryCount = 0
		p.mu.Unlock()

		slog.Info("IDLE connection established",
			"account_id", entry.config.AccountID,
			"host", entry.config.Host,
		)

		p.forwardEvents(entry, conn)

		p.mu.Lock()
		entry.conn = nil
		p.mu.Unlock()
	}
}

// forwardEvents reads events from the connection and forwards them to the pool's channel.
// Returns when the connection is closed or the account is stopped.
func (p *IdlePool) forwardEvents(entry *accountEntry, conn *IdleConnection) {
	for {
		select {
		case <-entry.stopCh:
			slog.Debug("stopping event forwarding - account removed",
				"account_id", entry.config.AccountID,
			)
			conn.Close()

			return
		case <-p.ctx.Done():
			slog.Debug("stopping event forwarding - pool closing",
				"account_id", entry.config.AccountID,
			)
			conn.Close()

			return
		case event, ok := <-conn.Events():
			if !ok {
				slog.Warn("IDLE connection closed unexpectedly",
					"account_id", entry.config.AccountID,
				)
				conn.Close()

				return
			}

			p.mu.RLock()
			closed := p.closed
			p.mu.RUnlock()

			if closed {
				return
			}

			select {
			case p.events <- event:
				// Event forwarded
			default:
				slog.Warn("pool event channel full, dropping event",
					"account_id", entry.config.AccountID,
					"type", event.Type.String(),
					"folder", event.Folder,
				)
			}
		}
	}
}

// handleConnectionError logs the error and updates backoff state.
// Note: Non-retryable errors (ErrIdleNotSupported) should be handled by caller before this.
func (p *IdlePool) handleConnectionError(entry *accountEntry, err error) {
	p.mu.Lock()
	entry.retryCount++
	currentBackoff := entry.backoff
	entry.backoff = time.Duration(float64(entry.backoff) * backoffFactor)

	if entry.backoff > maxBackoff {
		entry.backoff = maxBackoff
	}

	p.mu.Unlock()

	slog.Warn("IDLE connection failed, will retry",
		"account_id", entry.config.AccountID,
		"error", err,
		"retry_count", entry.retryCount,
		"next_retry_in", currentBackoff,
	)
}

// closeConnection safely closes the connection if it exists.
func (p *IdlePool) closeConnection(entry *accountEntry) {
	p.mu.Lock()
	conn := entry.conn
	entry.conn = nil
	p.mu.Unlock()

	if conn != nil {
		if err := conn.Close(); err != nil && !errors.Is(err, ErrAlreadyClosed) {
			slog.Warn("error closing IDLE connection",
				"account_id", entry.config.AccountID,
				"error", err,
			)
		}
	}
}
