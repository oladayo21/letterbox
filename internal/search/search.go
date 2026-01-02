package search

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/oladayo21/letterbox/internal/domain"
	"github.com/oladayo21/letterbox/internal/repository"
)

const (
	DefaultLimit = 50
	MaxLimit     = 100
)

var (
	ErrEmptyQuery = errors.New("search query cannot be empty")
)

type Service struct {
	emailRepo *repository.EmailRepository
}

func NewService(emailRepo *repository.EmailRepository) *Service {
	return &Service{emailRepo: emailRepo}
}

type SearchResult struct {
	Emails []domain.Email
	Total  int64
	Limit  int
	Offset int
}

// Search performs a full-text search across emails for the given account.
// The query uses PostgreSQL's websearch_to_tsquery which supports:
// - Simple terms: "invoice" finds emails containing "invoice"
// - Phrases: "\"john smith\"" finds emails with exact phrase
// - OR: "invoice OR receipt" finds emails with either term
// - NOT: "-spam" excludes emails containing "spam"
// - AND (implicit): "invoice payment" finds emails with both terms
func (s *Service) Search(ctx context.Context, accountID uuid.UUID, query string, folder string, limit, offset int) (*SearchResult, error) {
	query = strings.TrimSpace(query)

	if query == "" {
		return nil, ErrEmptyQuery
	}

	if limit <= 0 {
		limit = DefaultLimit
	}

	if limit > MaxLimit {
		limit = MaxLimit
	}

	if offset < 0 {
		offset = 0
	}

	filter := domain.SearchEmailsFilter{
		AccountID: accountID,
		Query:     query,
		Folder:    folder,
		Limit:     limit,
		Offset:    offset,
	}

	emails, err := s.emailRepo.Search(ctx, filter)

	if err != nil {
		return nil, err
	}

	total, err := s.emailRepo.CountSearch(ctx, accountID, query, folder)

	if err != nil {
		return nil, err
	}

	return &SearchResult{
		Emails: emails,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}
