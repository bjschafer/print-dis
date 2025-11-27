package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/bjschafer/print-dis/internal/config"
	"github.com/bjschafer/print-dis/internal/database"
	"github.com/gorilla/sessions"
)

// SessionContextKey is the key used to store session data in the request context
type SessionContextKey string

const (
	// UserIDKey is the context key for the user ID
	UserIDKey SessionContextKey = "user_id"
	// UserKey is the context key for the full user object
	UserKey SessionContextKey = "user"
	// SessionKey is the context key for the session store
	SessionKey SessionContextKey = "session"
)

// SessionStore holds the session store for the application
type SessionStore struct {
	store sessions.Store
	db    database.DBClient
}

// NewSessionStore creates a new session store
func NewSessionStore(cfg *config.Config, db database.DBClient) *SessionStore {
	// Create session store with secret key
	store := sessions.NewCookieStore([]byte(cfg.Auth.SessionSecret))
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   int(cfg.Auth.SessionTimeout.Seconds()),
		HttpOnly: true,
		Secure:   isHTTPS(cfg), // Environment-aware secure flag
		SameSite: http.SameSiteLaxMode,
	}

	return &SessionStore{
		store: store,
		db:    db,
	}
}

// SessionMiddleware creates a middleware that handles session management
func (s *SessionStore) SessionMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get the session - if there's an error (e.g., invalid cookie from old session secret),
			// gorilla/sessions still returns a new session, so we can safely ignore the error
			// and just log it for debugging
			session, err := s.store.Get(r, "print-dis-session")
			if err != nil {
				slog.Warn("session decode error (likely stale cookie), using new session", "error", err)
				// The session returned is still valid (it's a new empty session),
				// so we can continue. The old invalid cookie will be overwritten on save.
			}

			// Add session to context
			ctx := context.WithValue(r.Context(), SessionKey, session)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AuthMiddleware creates a middleware that requires authentication
func (s *SessionStore) AuthMiddleware(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// If auth is disabled, skip authentication
			if !cfg.Auth.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			// Get session from context
			session, ok := r.Context().Value(SessionKey).(*sessions.Session)
			if !ok {
				slog.Error("session not found in context")
				http.Error(w, "Session error", http.StatusInternalServerError)
				return
			}

			// Check if user is authenticated
			userID, ok := session.Values["user_id"].(string)
			if !ok || userID == "" {
				slog.Debug("user not authenticated", "path", r.URL.Path)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Fetch the full user object
			user, err := s.db.GetUser(r.Context(), userID)
			if err != nil {
				slog.Error("failed to get user", "error", err, "user_id", userID)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			if user == nil {
				slog.Warn("user not found", "user_id", userID)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			if !user.Enabled {
				slog.Info("disabled user attempted access", "user_id", userID)
				http.Error(w, "Account disabled", http.StatusForbidden)
				return
			}

			// Add user ID and full user object to context
			ctx := context.WithValue(r.Context(), UserIDKey, userID)
			ctx = context.WithValue(ctx, UserKey, user)
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

// GetSession retrieves the session from the request context
func GetSession(r *http.Request) *sessions.Session {
	if session, ok := r.Context().Value(SessionKey).(*sessions.Session); ok {
		return session
	}
	return nil
}

// LoginUser logs in a user by setting the session
func (s *SessionStore) LoginUser(w http.ResponseWriter, r *http.Request, userID string) error {
	session := GetSession(r)
	if session == nil {
		return fmt.Errorf("session not found")
	}

	session.Values["user_id"] = userID
	return session.Save(r, w)
}

// LogoutUser logs out a user by clearing the session
func (s *SessionStore) LogoutUser(w http.ResponseWriter, r *http.Request) error {
	session := GetSession(r)
	if session == nil {
		return fmt.Errorf("session not found")
	}

	delete(session.Values, "user_id")
	session.Options.MaxAge = -1 // Delete the cookie
	return session.Save(r, w)
}

// RegenerateSession regenerates the session ID to prevent session fixation attacks
func (s *SessionStore) RegenerateSession(w http.ResponseWriter, r *http.Request, userID string) error {
	// Try to invalidate old session if it exists, but don't fail if this doesn't work
	// (e.g., if there's a stale cookie with invalid signature)
	oldSession := GetSession(r)
	if oldSession != nil {
		oldSession.Options.MaxAge = -1
		// Ignore errors from saving the old session - it might have an invalid signature
		_ = oldSession.Save(r, w)
	}

	// Create new session with new ID - use Get instead of New to handle existing cookies
	newSession, err := s.store.Get(r, "print-dis-session")
	if err != nil {
		// If there's an error (e.g., stale cookie), create a truly new session
		slog.Warn("error getting session during regeneration, creating fresh session", "error", err)
		newSession, err = s.store.New(r, "print-dis-session")
		if err != nil {
			return fmt.Errorf("failed to create new session: %w", err)
		}
	}

	// Clear any existing values and set fresh data
	for key := range newSession.Values {
		delete(newSession.Values, key)
	}

	// Set user in new session
	newSession.Values["user_id"] = userID
	newSession.Values["created_at"] = time.Now()

	// Ensure the session is marked as new/modified so it gets a fresh ID
	newSession.IsNew = true

	// Save new session
	if err := newSession.Save(r, w); err != nil {
		return fmt.Errorf("failed to save new session: %w", err)
	}

	return nil
}

// isHTTPS determines if HTTPS should be used based on configuration
func isHTTPS(cfg *config.Config) bool {
	// Check if explicitly configured
	if cfg.Server.HTTPS != nil {
		return *cfg.Server.HTTPS
	}

	// Auto-detect based on host/port
	host := strings.ToLower(cfg.Server.Host)
	port := cfg.Server.Port

	// Production indicators
	if host != "localhost" && host != "127.0.0.1" && host != "0.0.0.0" {
		return true
	}

	// HTTPS port
	if port == "443" {
		return true
	}

	// Development default
	return false
}
