package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockHealthChecker is a mock implementation of HealthChecker.
type mockHealthChecker struct {
	err error
}

func (m *mockHealthChecker) Ping(ctx context.Context) error {
	return m.err
}

// mockSyncProvider is a mock implementation of SyncStatusProvider.
type mockSyncProvider struct {
	stats SyncStats
}

func (m *mockSyncProvider) Stats() SyncStats {
	return m.stats
}

func TestHealthHandler_Health(t *testing.T) {
	handler := NewHealthHandler(&mockHealthChecker{}, &mockHealthChecker{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handler.Health(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	if w.Body.String() == "" {
		t.Error("expected non-empty response body")
	}
}

func TestHealthHandler_Ready_AllHealthy(t *testing.T) {
	handler := NewHealthHandler(
		&mockHealthChecker{},
		&mockHealthChecker{},
		&mockSyncProvider{stats: SyncStats{IdleAccounts: 1, ConnectedIdle: 1}},
	)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	handler.Ready(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestHealthHandler_Ready_DBUnhealthy(t *testing.T) {
	handler := NewHealthHandler(
		&mockHealthChecker{err: errors.New("connection refused")},
		&mockHealthChecker{},
		&mockSyncProvider{},
	)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	handler.Ready(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestHealthHandler_Ready_S3Unhealthy(t *testing.T) {
	handler := NewHealthHandler(
		&mockHealthChecker{},
		&mockHealthChecker{err: errors.New("bucket not accessible")},
		&mockSyncProvider{},
	)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	handler.Ready(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestHealthHandler_Ready_SyncUnhealthy(t *testing.T) {
	// IDLE accounts exist but none connected
	handler := NewHealthHandler(
		&mockHealthChecker{},
		&mockHealthChecker{},
		&mockSyncProvider{stats: SyncStats{IdleAccounts: 2, ConnectedIdle: 0}},
	)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	handler.Ready(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestHealthHandler_Ready_NoAccounts(t *testing.T) {
	// No accounts configured - should be healthy
	handler := NewHealthHandler(
		&mockHealthChecker{},
		&mockHealthChecker{},
		&mockSyncProvider{stats: SyncStats{IdleAccounts: 0, PolledAccounts: 0, ConnectedIdle: 0}},
	)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	handler.Ready(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestHealthHandler_Ready_NilSyncProvider(t *testing.T) {
	// Sync provider is nil - should still work (just skip sync check)
	handler := NewHealthHandler(
		&mockHealthChecker{},
		&mockHealthChecker{},
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	handler.Ready(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}
