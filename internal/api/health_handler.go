package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// HealthChecker defines the interface for checking component health.
type HealthChecker interface {
	// Ping checks if the component is healthy.
	// Returns nil if healthy, error otherwise.
	Ping(ctx context.Context) error
}

// SyncStats contains statistics about the sync system.
type SyncStats struct {
	IdleAccounts   int
	PolledAccounts int
	ConnectedIdle  int
}

// SyncStatusProvider provides sync system status for health checks.
type SyncStatusProvider interface {
	// Stats returns current sync statistics.
	Stats() SyncStats
}

// HealthHandler handles health check endpoints.
type HealthHandler struct {
	db      HealthChecker
	s3      HealthChecker
	sync    SyncStatusProvider
	timeout time.Duration
}

// NewHealthHandler creates a new health handler.
func NewHealthHandler(db, s3 HealthChecker, sync SyncStatusProvider) *HealthHandler {
	return &HealthHandler{
		db:      db,
		s3:      s3,
		sync:    sync,
		timeout: 5 * time.Second,
	}
}

// Health is a basic liveness check - returns 200 if the process is alive.
// GET /health
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

// Ready is a readiness check - returns 200 if all dependencies are healthy.
// GET /ready
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	checks := make(map[string]interface{})
	allHealthy := true

	// Check database
	if err := h.db.Ping(ctx); err != nil {
		slog.Warn("health check failed", "component", "database", "error", err)
		checks["database"] = err.Error()
		allHealthy = false
	} else {
		checks["database"] = "ok"
	}

	// Check S3
	if err := h.s3.Ping(ctx); err != nil {
		slog.Warn("health check failed", "component", "s3", "error", err)
		checks["s3"] = err.Error()
		allHealthy = false
	} else {
		checks["s3"] = "ok"
	}

	// Check sync system (IMAP connections)
	if h.sync != nil {
		stats := h.sync.Stats()
		totalAccounts := stats.IdleAccounts + stats.PolledAccounts

		syncCheck := map[string]interface{}{
			"idle_accounts":   stats.IdleAccounts,
			"polled_accounts": stats.PolledAccounts,
			"connected_idle":  stats.ConnectedIdle,
		}

		// Consider unhealthy if we have IDLE accounts but none are connected
		if stats.IdleAccounts > 0 && stats.ConnectedIdle == 0 {
			slog.Warn("health check failed", "component", "sync",
				"error", "no IDLE connections active",
				"idle_accounts", stats.IdleAccounts,
			)
			syncCheck["status"] = fmt.Sprintf("unhealthy: %d IDLE accounts but 0 connected", stats.IdleAccounts)
			allHealthy = false
		} else if totalAccounts > 0 {
			syncCheck["status"] = "ok"
		} else {
			syncCheck["status"] = "ok (no accounts)"
		}

		checks["sync"] = syncCheck
	}

	response := map[string]interface{}{
		"checks": checks,
	}

	if allHealthy {
		response["status"] = "ok"
		writeJSON(w, http.StatusOK, response)
	} else {
		response["status"] = "unhealthy"
		writeJSON(w, http.StatusServiceUnavailable, response)
	}
}
