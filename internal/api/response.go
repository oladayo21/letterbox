package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
)

const (
	defaultLimit = 50
	maxLimit     = 100
)

// parsePaginationParams extracts and validates limit/offset from query params.
func parsePaginationParams(r *http.Request) (limit, offset int, err error) {
	limit = defaultLimit
	offset = 0

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		limit, err = strconv.Atoi(limitStr)

		if err != nil || limit < 1 {
			return 0, 0, errors.New("invalid limit")
		}

		if limit > maxLimit {
			limit = maxLimit
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		offset, err = strconv.Atoi(offsetStr)

		if err != nil || offset < 0 {
			return 0, 0, errors.New("invalid offset")
		}
	}

	return limit, offset, nil
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

// formatValidationError formats validator.ValidationErrors into a human-readable string.
// fieldNames maps struct field names to JSON field names.
func formatValidationError(err error, fieldNames map[string]string) string {
	var ve validator.ValidationErrors

	if !errors.As(err, &ve) {
		return "validation failed"
	}

	var msgs []string

	for _, fe := range ve {
		field := fieldNames[fe.Field()]

		if field == "" {
			field = fe.Field()
		}

		msgs = append(msgs, formatFieldError(field, fe.Tag(), fe.Param()))
	}

	return strings.Join(msgs, "; ")
}

func formatFieldError(field, tag, param string) string {
	switch tag {
	case "required":
		return field + " is required"
	case "uuid":
		return field + " must be a valid UUID"
	case "url":
		return field + " must be a valid URL"
	case "max":
		return field + " must be " + param + " characters or less"
	case "min":
		return field + " must be at least " + param + " characters"
	default:
		return field + " is invalid"
	}
}
