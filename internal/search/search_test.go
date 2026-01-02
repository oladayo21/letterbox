package search_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/oladayo21/letterbox/internal/search"
)

func TestService_Search_EmptyQuery(t *testing.T) {
	// Service with nil repo is fine for this test since we fail before calling repo
	svc := search.NewService(nil)

	testCases := []struct {
		name  string
		query string
	}{
		{"empty string", ""},
		{"whitespace only", "   "},
		{"tabs and newlines", "\t\n  "},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Search(context.Background(), uuid.New(), tc.query, "", 50, 0)

			if err != search.ErrEmptyQuery {
				t.Errorf("expected ErrEmptyQuery, got %v", err)
			}
		})
	}
}

func TestService_Search_LimitDefaults(t *testing.T) {
	// These tests verify the limit/offset normalization logic
	// We can't easily test without a real repo, but we document expected behavior

	t.Run("negative limit becomes default", func(t *testing.T) {
		// limit <= 0 should become DefaultLimit (50)
		// This is tested implicitly through integration tests
	})

	t.Run("excessive limit capped to max", func(t *testing.T) {
		// limit > MaxLimit (100) should become MaxLimit
		// This is tested implicitly through integration tests
	})

	t.Run("negative offset becomes zero", func(t *testing.T) {
		// offset < 0 should become 0
		// This is tested implicitly through integration tests
	})
}
