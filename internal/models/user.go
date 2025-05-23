package models

import (
	"time"

	"golang.org/x/crypto/bcrypt"
)

// User represents a user account in the system
type User struct {
	ID           string    `json:"id" db:"id"`
	Username     string    `json:"username" db:"username"`
	Email        *string   `json:"email,omitempty" db:"email"`
	PasswordHash *string   `json:"-" db:"password_hash"` // Never serialize password hash
	DisplayName  *string   `json:"display_name,omitempty" db:"display_name"`
	Role         Role      `json:"role" db:"role"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
	Enabled      bool      `json:"enabled" db:"enabled"`
}

// UserOIDCIdentity represents a user's OIDC identity mapping
type UserOIDCIdentity struct {
	ID           string    `json:"id" db:"id"`
	UserID       string    `json:"user_id" db:"user_id"`
	ProviderName string    `json:"provider_name" db:"provider_name"`
	Subject      string    `json:"subject" db:"subject"` // OIDC 'sub' claim
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// UserSession represents an active user session
type UserSession struct {
	ID           string    `json:"id" db:"id"`
	UserID       string    `json:"user_id" db:"user_id"`
	SessionToken string    `json:"-" db:"session_token"` // Never serialize session token
	ExpiresAt    time.Time `json:"expires_at" db:"expires_at"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// NewUser creates a new user with default values
func NewUser(username string, email *string) *User {
	now := time.Now()
	return &User{
		Username:  username,
		Email:     email,
		Role:      DefaultRole(),
		CreatedAt: now,
		UpdatedAt: now,
		Enabled:   true,
	}
}

// SetPassword hashes and sets the user's password
func (u *User) SetPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	hashStr := string(hash)
	u.PasswordHash = &hashStr
	return nil
}

// CheckPassword verifies if the provided password matches the user's password hash
func (u *User) CheckPassword(password string) bool {
	if u.PasswordHash == nil {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(*u.PasswordHash), []byte(password)) == nil
}

// HasPassword returns true if the user has a password set (local auth)
func (u *User) HasPassword() bool {
	return u.PasswordHash != nil && *u.PasswordHash != ""
}

// IsExpired returns true if the session has expired
func (s *UserSession) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// HasPermission checks if the user has the specified permission
func (u *User) HasPermission(permission Permission) bool {
	if !u.Enabled {
		return false
	}
	return u.Role.HasPermission(permission)
}

// IsAdmin returns true if the user has admin role
func (u *User) IsAdmin() bool {
	return u.Role == RoleAdmin
}

// IsModerator returns true if the user has moderator role or higher
func (u *User) IsModerator() bool {
	return u.Role == RoleModerator || u.Role == RoleAdmin
}

// CanManageUser returns true if this user can manage the target user
func (u *User) CanManageUser(targetUser *User) bool {
	// Can't manage yourself (for role changes)
	if u.ID == targetUser.ID {
		return false
	}

	// Admin can manage anyone
	if u.Role == RoleAdmin {
		return true
	}

	// Moderators can manage regular users but not other moderators or admins
	if u.Role == RoleModerator && targetUser.Role == RoleUser {
		return true
	}

	return false
}
