package services

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/bjschafer/print-dis/internal/database"
	"github.com/bjschafer/print-dis/internal/models"
	"github.com/google/uuid"
)

// UserService provides business logic for user management operations
type UserService struct {
	db database.DBClient
}

// NewUserService creates a new UserService
func NewUserService(db database.DBClient) *UserService {
	return &UserService{
		db: db,
	}
}

// RegisterUser creates a new user account with local authentication using a transaction to prevent race conditions
func (s *UserService) RegisterUser(ctx context.Context, username, email, password string) (*models.User, error) {
	// Validate input
	if strings.TrimSpace(username) == "" {
		return nil, fmt.Errorf("username is required")
	}
	if strings.TrimSpace(password) == "" {
		return nil, fmt.Errorf("password is required")
	}

	// Start a transaction
	tx, err := s.db.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Check if username already exists within the transaction
	existingUser, err := tx.GetUserByUsername(ctx, username)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to check existing username: %w", err)
	}
	if existingUser != nil {
		return nil, fmt.Errorf("username already exists")
	}

	// Check if email already exists (if provided) within the transaction
	var emailPtr *string
	if email != "" {
		emailPtr = &email
		existingUser, err = tx.GetUserByEmail(ctx, email)
		if err != nil && err != sql.ErrNoRows {
			return nil, fmt.Errorf("failed to check existing email: %w", err)
		}
		if existingUser != nil {
			return nil, fmt.Errorf("email already exists")
		}
	}

	// Create new user
	user := models.NewUser(username, emailPtr)
	user.ID = uuid.New().String()

	// Set password
	if err := user.SetPassword(password); err != nil {
		return nil, fmt.Errorf("failed to set password: %w", err)
	}

	// Save to database within the transaction
	if err := tx.CreateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	slog.Info("user registered", "user_id", user.ID, "username", user.Username)
	return user, nil
}

// AuthenticateUser validates user credentials and returns the user if valid
func (s *UserService) AuthenticateUser(ctx context.Context, username, password string) (*models.User, error) {
	user, err := s.db.GetUserByUsername(ctx, username)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("invalid credentials")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if !user.Enabled {
		return nil, fmt.Errorf("account is disabled")
	}

	if !user.CheckPassword(password) {
		return nil, fmt.Errorf("invalid credentials")
	}

	slog.Info("user authenticated", "user_id", user.ID, "username", user.Username)
	return user, nil
}

// GetUser retrieves a user by ID
func (s *UserService) GetUser(ctx context.Context, userID string) (*models.User, error) {
	user, err := s.db.GetUser(ctx, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return user, nil
}

// UpdateUser updates user information
func (s *UserService) UpdateUser(ctx context.Context, user *models.User) error {
	user.UpdatedAt = time.Now()
	if err := s.db.UpdateUser(ctx, user); err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}
	return nil
}

// ChangePassword changes a user's password
func (s *UserService) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	user, err := s.GetUser(ctx, userID)
	if err != nil {
		return err
	}

	if !user.CheckPassword(currentPassword) {
		return fmt.Errorf("current password is incorrect")
	}

	if err := user.SetPassword(newPassword); err != nil {
		return fmt.Errorf("failed to set new password: %w", err)
	}

	return s.UpdateUser(ctx, user)
}

// DisableUser disables a user account
func (s *UserService) DisableUser(ctx context.Context, userID string) error {
	user, err := s.GetUser(ctx, userID)
	if err != nil {
		return err
	}

	user.Enabled = false
	return s.UpdateUser(ctx, user)
}

// EnableUser enables a user account
func (s *UserService) EnableUser(ctx context.Context, userID string) error {
	user, err := s.GetUser(ctx, userID)
	if err != nil {
		return err
	}

	user.Enabled = true
	return s.UpdateUser(ctx, user)
}

// ListUsers returns all users
func (s *UserService) ListUsers(ctx context.Context) ([]*models.User, error) {
	users, err := s.db.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	return users, nil
}

// CreateSession creates a new session for a user
func (s *UserService) CreateSession(ctx context.Context, userID string, duration time.Duration) (*models.UserSession, error) {
	// Generate session token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("failed to generate session token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)

	// Create session
	session := &models.UserSession{
		ID:           uuid.New().String(),
		UserID:       userID,
		SessionToken: token,
		ExpiresAt:    time.Now().Add(duration),
		CreatedAt:    time.Now(),
	}

	// Note: We'll need to add session management to the database interface
	// For now, we'll return the session object
	return session, nil
}

// FindOrCreateUserFromOIDC finds an existing user or creates a new one from OIDC identity
func (s *UserService) FindOrCreateUserFromOIDC(ctx context.Context, providerName, subject, email, displayName string) (*models.User, error) {
	// First, try to find existing OIDC identity
	// Note: We'll need to implement OIDC identity management in the database interface

	// For now, try to find by email if provided
	if email != "" {
		user, err := s.db.GetUserByEmail(ctx, email)
		if err != nil && err != sql.ErrNoRows {
			return nil, fmt.Errorf("failed to check existing email: %w", err)
		}
		if user != nil {
			// User exists, link OIDC identity if not already linked
			slog.Info("found existing user for OIDC", "user_id", user.ID, "provider", providerName)
			return user, nil
		}
	}

	// Create new user from OIDC
	var emailPtr *string
	if email != "" {
		emailPtr = &email
	}
	var displayNamePtr *string
	if displayName != "" {
		displayNamePtr = &displayName
	}

	// Generate username from email or subject
	username := email
	if username == "" {
		username = fmt.Sprintf("%s_%s", providerName, subject)
	}

	user := models.NewUser(username, emailPtr)
	user.ID = uuid.New().String()
	user.DisplayName = displayNamePtr

	if err := s.db.CreateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to create user from OIDC: %w", err)
	}

	slog.Info("created new user from OIDC", "user_id", user.ID, "provider", providerName, "subject", subject)
	return user, nil
}
