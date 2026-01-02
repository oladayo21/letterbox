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
