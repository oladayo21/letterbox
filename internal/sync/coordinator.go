package sync

import (
	"context"
	"errors"
	"log/slog"
	gosync "sync"
	"time"

	"github.com/google/uuid"

	"github.com/oladayo21/letterbox/internal/domain"
	"github.com/oladayo21/letterbox/internal/imap"
	"github.com/oladayo21/letterbox/internal/ingest"
	"github.com/oladayo21/letterbox/internal/repository"
)

var (
	ErrCoordinatorClosed = errors.New("coordinator is closed")
)

// EventHandler is called when new emails are ingested.
// This can be used to trigger webhooks or other actions.
type EventHandler func(ctx context.Context, email *domain.Email)

// Coordinator orchestrates IDLE connections and polling, processing
// new email events through the ingest pipeline.
type Coordinator struct {
	pool     *IdlePool
	poller   *Poller
	ingester *ingest.Ingester

	accountRepo *repository.AccountRepository

	// Track last seen UID per account/folder
	mu       gosync.RWMutex
	lastUIDs map[string]uint32 // key: accountID:folder

	// Event handler for ingested emails (e.g., webhook trigger)
	eventHandler EventHandler

	ctx    context.Context
	cancel context.CancelFunc

	wg     gosync.WaitGroup
	closed bool
}

// CoordinatorConfig contains configuration for the coordinator.
type CoordinatorConfig struct {
	PollInterval time.Duration
	EventHandler EventHandler
}

// NewCoordinator creates a new sync coordinator.
func NewCoordinator(
	ingester *ingest.Ingester,
	accountRepo *repository.AccountRepository,
	config CoordinatorConfig,
) *Coordinator {
	ctx, cancel := context.WithCancel(context.Background())

	pollInterval := config.PollInterval
	if pollInterval == 0 {
		pollInterval = DefaultPollInterval
	}

	return &Coordinator{
		pool:         NewIdlePool(),
		poller:       NewPoller(pollInterval),
		ingester:     ingester,
		accountRepo:  accountRepo,
		lastUIDs:     make(map[string]uint32),
		eventHandler: config.EventHandler,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Start begins processing events from IDLE pool and poller.
func (c *Coordinator) Start() {
	c.wg.Add(2)

	go c.processPoolEvents()
	go c.processPollerEvents()

	slog.Info("sync coordinator started")
}

// AddAccount adds an account to the sync system.
// It will automatically use IDLE if supported, otherwise polling.
func (c *Coordinator) AddAccount(ctx context.Context, accountID uuid.UUID) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()

		return ErrCoordinatorClosed
	}
	c.mu.Unlock()

	account, err := c.accountRepo.Get(ctx, accountID)

	if err != nil {
		return err
	}

	config := IdleConfig{
		AccountID: accountID.String(),
		Host:      account.ImapHost,
		Port:      account.ImapPort,
		Username:  account.ImapUser,
		Password:  account.ImapPassword,
		Folder:    "INBOX",
	}

	// Try IDLE first
	hasIdle, err := HasIdleCapability(ctx, config.Host, config.Port, config.Username, config.Password)

	if err != nil {
		slog.Warn("failed to check IDLE capability, falling back to polling",
			"account_id", accountID,
			"error", err,
		)

		return c.poller.AddAccount(config)
	}

	if hasIdle {
		slog.Info("using IDLE for account", "account_id", accountID)

		return c.pool.AddAccount(config)
	}

	slog.Info("using polling for account (no IDLE support)", "account_id", accountID)

	return c.poller.AddAccount(config)
}

// RemoveAccount removes an account from the sync system.
func (c *Coordinator) RemoveAccount(accountID uuid.UUID) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()

		return ErrCoordinatorClosed
	}
	c.mu.Unlock()

	accountIDStr := accountID.String()

	// Try to remove from both - one will succeed
	poolErr := c.pool.RemoveAccount(accountIDStr)
	pollerErr := c.poller.RemoveAccount(accountIDStr)

	// Clean up last UID tracking
	c.mu.Lock()
	delete(c.lastUIDs, accountIDStr+":INBOX")
	c.mu.Unlock()

	if poolErr != nil && pollerErr != nil {
		// Both failed - account wasn't in either
		if errors.Is(poolErr, ErrAccountNotFound) && errors.Is(pollerErr, ErrAccountNotFound) {
			return ErrAccountNotFound
		}

		return poolErr // Return pool error as primary
	}

	return nil
}

// Close shuts down the coordinator and all connections.
func (c *Coordinator) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()

		return ErrAlreadyClosed
	}

	c.closed = true
	c.mu.Unlock()

	c.cancel()

	// Close pool and poller
	c.pool.Close()
	c.poller.Close()

	// Wait for event processors to finish
	c.wg.Wait()

	slog.Info("sync coordinator closed")

	return nil
}

// processPoolEvents handles events from the IDLE pool.
func (c *Coordinator) processPoolEvents() {
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			return
		case event, ok := <-c.pool.Events():
			if !ok {
				return
			}

			c.handleEvent(event)
		}
	}
}

// processPollerEvents handles events from the poller.
func (c *Coordinator) processPollerEvents() {
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			return
		case event, ok := <-c.poller.Events():
			if !ok {
				return
			}

			c.handleEvent(event)
		}
	}
}

// handleEvent processes a single mailbox event.
func (c *Coordinator) handleEvent(event IdleEvent) {
	if event.Type != EventNewMessage {
		// For now, we only handle new messages
		// Expunge events could be handled to mark emails as deleted
		return
	}

	slog.Debug("handling new message event",
		"account_id", event.AccountID,
		"folder", event.Folder,
	)

	accountID, err := uuid.Parse(event.AccountID)

	if err != nil {
		slog.Error("invalid account ID in event",
			"account_id", event.AccountID,
			"error", err,
		)

		return
	}

	// Get account credentials
	account, err := c.accountRepo.Get(c.ctx, accountID)

	if err != nil {
		slog.Error("failed to get account for sync",
			"account_id", event.AccountID,
			"error", err,
		)

		return
	}

	// Get last known UID for this account/folder
	uidKey := event.AccountID + ":" + event.Folder

	c.mu.RLock()
	lastUID := c.lastUIDs[uidKey]
	c.mu.RUnlock()

	// Fetch new UIDs
	newUIDs, err := imap.FetchUIDsAfter(
		c.ctx,
		account.ImapHost,
		account.ImapPort,
		account.ImapUser,
		account.ImapPassword,
		event.Folder,
		lastUID,
	)

	if err != nil {
		slog.Error("failed to fetch new UIDs",
			"account_id", event.AccountID,
			"folder", event.Folder,
			"error", err,
		)

		return
	}

	if len(newUIDs) == 0 {
		slog.Debug("no new UIDs found",
			"account_id", event.AccountID,
			"folder", event.Folder,
		)

		return
	}

	slog.Info("found new emails to ingest",
		"account_id", event.AccountID,
		"folder", event.Folder,
		"count", len(newUIDs),
	)

	// Ingest each new email
	var maxUID uint32

	for _, uid := range newUIDs {
		email, err := c.ingester.IngestEmail(c.ctx, accountID, event.Folder, uid)

		if err != nil {
			if errors.Is(err, ingest.ErrEmailAlreadyExists) {
				// Already ingested, just update last UID
				if uid > maxUID {
					maxUID = uid
				}

				continue
			}

			slog.Error("failed to ingest email",
				"account_id", event.AccountID,
				"folder", event.Folder,
				"uid", uid,
				"error", err,
			)

			continue
		}

		slog.Info("ingested email",
			"account_id", event.AccountID,
			"folder", event.Folder,
			"uid", uid,
			"subject", email.Subject,
		)

		if uid > maxUID {
			maxUID = uid
		}

		// Call event handler if set (for webhooks)
		if c.eventHandler != nil {
			c.eventHandler(c.ctx, email)
		}
	}

	// Update last known UID
	if maxUID > 0 {
		c.mu.Lock()
		if maxUID > c.lastUIDs[uidKey] {
			c.lastUIDs[uidKey] = maxUID
		}
		c.mu.Unlock()
	}
}

// Stats returns current coordinator statistics.
func (c *Coordinator) Stats() CoordinatorStats {
	return CoordinatorStats{
		IdleAccounts:   c.pool.TotalAccounts(),
		PolledAccounts: c.poller.TotalAccounts(),
		ConnectedIdle:  c.pool.ConnectedAccounts(),
	}
}

// CoordinatorStats contains statistics about the coordinator.
type CoordinatorStats struct {
	IdleAccounts   int
	PolledAccounts int
	ConnectedIdle  int
}
