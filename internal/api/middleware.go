package api

import (
	"crypto/subtle"
	"net/http"
)

const apiKeyHeader = "X-API-Key"

func APIKeyAuth(apiKey string) func(http.Handler) http.Handler {

	return func(next http.Handler) http.Handler {

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			provided := r.Header.Get(apiKeyHeader)

			if provided == "" {
				writeError(w, http.StatusUnauthorized, "missing API key")

				return
			}

			if subtle.ConstantTimeCompare([]byte(provided), []byte(apiKey)) != 1 {
				writeError(w, http.StatusUnauthorized, "invalid API key")

				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
