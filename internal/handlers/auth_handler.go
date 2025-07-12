package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/bjschafer/print-dis/internal/config"
	"github.com/bjschafer/print-dis/internal/middleware"
	"github.com/bjschafer/print-dis/internal/response"
	"github.com/bjschafer/print-dis/internal/services"
	"github.com/bjschafer/print-dis/internal/validation"
)

// AuthHandler handles authentication-related HTTP requests
type AuthHandler struct {
	userService  *services.UserService
	sessionStore *middleware.SessionStore
	config       *config.Config
}

// NewAuthHandler creates a new AuthHandler
func NewAuthHandler(userService *services.UserService, sessionStore *middleware.SessionStore, config *config.Config) *AuthHandler {
	return &AuthHandler{
		userService:  userService,
		sessionStore: sessionStore,
		config:       config,
	}
}

// LoginRequest represents a login request
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Validate validates the login request
func (r *LoginRequest) Validate() validation.ValidationErrors {
	validator := validation.NewValidator()
	
	// Sanitize inputs
	r.Username = validation.SanitizeString(r.Username)
	
	validator.ValidateRequired("username", r.Username)
	validator.ValidateRequired("password", r.Password)
	validator.ValidateLength("username", r.Username, 1, 100)
	validator.ValidateLength("password", r.Password, 1, 500)
	
	return validator.Errors()
}

// RegisterRequest represents a registration request
type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email,omitempty"`
	Password string `json:"password"`
}

// Validate validates the registration request
func (r *RegisterRequest) Validate() validation.ValidationErrors {
	validator := validation.NewValidator()
	
	// Sanitize inputs
	r.Username = validation.SanitizeString(r.Username)
	r.Email = validation.SanitizeString(r.Email)
	
	validator.ValidateRequired("username", r.Username)
	validator.ValidateRequired("password", r.Password)
	validator.ValidateUsername("username", r.Username)
	validator.ValidateEmail("email", r.Email)
	validator.ValidateLength("password", r.Password, 8, 128)
	validator.ValidateNoHTML("username", r.Username)
	validator.ValidateNoHTML("email", r.Email)
	
	return validator.Errors()
}

// UserResponse represents a user in API responses
type UserResponse struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	Email       *string `json:"email,omitempty"`
	DisplayName *string `json:"display_name,omitempty"`
	Enabled     bool    `json:"enabled"`
}

// Login handles user login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteErrorResponse(w, http.StatusMethodNotAllowed, response.BadRequest, "Method not allowed", "")
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Error("failed to decode login request", "error", err)
		response.WriteBadRequestError(w, "Invalid request body", err.Error())
		return
	}

	// Validate input
	if validationErrors := req.Validate(); len(validationErrors) > 0 {
		validation.WriteValidationError(w, validationErrors)
		return
	}

	// Authenticate user
	user, err := h.userService.AuthenticateUser(r.Context(), req.Username, req.Password)
	if err != nil {
		slog.Info("authentication failed", "username", validation.SanitizeLogString(req.Username), "error", err)
		response.WriteUnauthorizedError(w, "Invalid credentials")
		return
	}

	// Create session with regeneration to prevent session fixation
	if err := h.sessionStore.RegenerateSession(w, r, user.ID); err != nil {
		slog.Error("failed to create session", "user_id", user.ID, "error", err)
		response.WriteInternalError(w, "Failed to create session", err.Error())
		return
	}

	// Return user info
	userResponse := UserResponse{
		ID:          user.ID,
		Username:    user.Username,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Enabled:     user.Enabled,
	}

	response.WriteSuccessResponse(w, userResponse, "Login successful")
	slog.Info("user logged in", "user_id", user.ID, "username", user.Username)
}

// Logout handles user logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteErrorResponse(w, http.StatusMethodNotAllowed, response.BadRequest, "Method not allowed", "")
		return
	}

	// Get current user (if any)
	userID := middleware.GetUserID(r)

	// Clear session
	if err := h.sessionStore.LogoutUser(w, r); err != nil {
		slog.Error("failed to clear session", "error", err)
		response.WriteInternalError(w, "Failed to logout", err.Error())
		return
	}

	response.WriteSuccessResponse(w, nil, "Logged out successfully")

	if userID != "" {
		slog.Info("user logged out", "user_id", userID)
	}
}

// Register handles user registration
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteErrorResponse(w, http.StatusMethodNotAllowed, response.BadRequest, "Method not allowed", "")
		return
	}

	// Check if registration is allowed
	if !h.config.Auth.LocalAuth.AllowRegistration {
		response.WriteForbiddenError(w, "Registration is disabled")
		return
	}

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Error("failed to decode register request", "error", err)
		response.WriteBadRequestError(w, "Invalid request body", err.Error())
		return
	}

	// Validate input
	if validationErrors := req.Validate(); len(validationErrors) > 0 {
		validation.WriteValidationError(w, validationErrors)
		return
	}

	// Register user
	user, err := h.userService.RegisterUser(r.Context(), req.Username, req.Email, req.Password)
	if err != nil {
		slog.Info("registration failed", "username", validation.SanitizeLogString(req.Username), "error", err)
		if strings.Contains(err.Error(), "already exists") {
			response.WriteConflictError(w, err.Error(), "User with this username or email already exists")
		} else {
			response.WriteBadRequestError(w, "Registration failed", err.Error())
		}
		return
	}

	// Automatically log in the user after registration
	if err := h.sessionStore.LoginUser(w, r, user.ID); err != nil {
		slog.Error("failed to create session after registration", "user_id", user.ID, "error", err)
		// Don't fail the registration, just log the error
	}

	// Return user info
	userResponse := UserResponse{
		ID:          user.ID,
		Username:    user.Username,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Enabled:     user.Enabled,
	}

	response.WriteCreatedResponse(w, userResponse, "User registered successfully")
	slog.Info("user registered", "user_id", user.ID, "username", user.Username)
}

// GetCurrentUser returns the current authenticated user
func (h *AuthHandler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.WriteErrorResponse(w, http.StatusMethodNotAllowed, response.BadRequest, "Method not allowed", "")
		return
	}

	userID := middleware.GetUserID(r)
	if userID == "" {
		response.WriteUnauthorizedError(w, "Not authenticated")
		return
	}

	user, err := h.userService.GetUser(r.Context(), userID)
	if err != nil {
		slog.Error("failed to get current user", "user_id", userID, "error", err)
		response.WriteNotFoundError(w, "User not found")
		return
	}

	userResponse := UserResponse{
		ID:          user.ID,
		Username:    user.Username,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Enabled:     user.Enabled,
	}

	response.WriteSuccessResponse(w, userResponse, "")
}

// ChangePasswordRequest represents a password change request
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// Validate validates the change password request
func (r *ChangePasswordRequest) Validate() validation.ValidationErrors {
	validator := validation.NewValidator()
	
	validator.ValidateRequired("current_password", r.CurrentPassword)
	validator.ValidateRequired("new_password", r.NewPassword)
	validator.ValidateLength("current_password", r.CurrentPassword, 1, 500)
	validator.ValidateLength("new_password", r.NewPassword, 8, 128)
	
	return validator.Errors()
}

// ChangePassword handles password changes for authenticated users
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteErrorResponse(w, http.StatusMethodNotAllowed, response.BadRequest, "Method not allowed", "")
		return
	}

	userID := middleware.GetUserID(r)
	if userID == "" {
		response.WriteUnauthorizedError(w, "Not authenticated")
		return
	}

	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Error("failed to decode change password request", "error", err)
		response.WriteBadRequestError(w, "Invalid request body", err.Error())
		return
	}

	// Validate input
	if validationErrors := req.Validate(); len(validationErrors) > 0 {
		validation.WriteValidationError(w, validationErrors)
		return
	}

	// Change password
	if err := h.userService.ChangePassword(r.Context(), userID, req.CurrentPassword, req.NewPassword); err != nil {
		slog.Info("password change failed", "user_id", userID, "error", err)
		if strings.Contains(err.Error(), "incorrect") {
			response.WriteBadRequestError(w, "Current password is incorrect", err.Error())
		} else {
			response.WriteInternalError(w, "Failed to change password", err.Error())
		}
		return
	}

	response.WriteSuccessResponse(w, nil, "Password changed successfully")
	slog.Info("password changed", "user_id", userID)
}
