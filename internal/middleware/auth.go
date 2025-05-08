package middleware

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/bjschafer/print-dis/internal/config"
)

// UserContextKey is the key used to store the user ID in the request context
type UserContextKey string

const (
	// UserIDKey is the context key for the user ID
	UserIDKey UserContextKey = "user_id"
)

// AuthMiddleware creates a middleware that validates the auth header and adds the user ID to the context
func AuthMiddleware(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// If auth is not configured, proceed without authentication
			if cfg.Auth.HeaderName == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Get the auth header
			userID := r.Header.Get(cfg.Auth.HeaderName)
			if userID == "" {
				slog.Warn("missing auth header",
					"header", cfg.Auth.HeaderName,
					"path", r.URL.Path,
				)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Add user ID to context
			ctx := context.WithValue(r.Context(), UserIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserID retrieves the user ID from the request context
func GetUserID(r *http.Request) string {
	if userID, ok := r.Context().Value(UserIDKey).(string); ok {
		return userID
	}
	return ""
}
