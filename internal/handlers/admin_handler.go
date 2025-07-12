package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/bjschafer/print-dis/internal/config"
	"github.com/bjschafer/print-dis/internal/middleware"
	"github.com/bjschafer/print-dis/internal/models"
	"github.com/bjschafer/print-dis/internal/response"
	"github.com/bjschafer/print-dis/internal/services"
)

// AdminHandler handles admin-related HTTP requests
type AdminHandler struct {
	userService *services.UserService
	config      *config.Config
}

// NewAdminHandler creates a new AdminHandler
func NewAdminHandler(userService *services.UserService, config *config.Config) *AdminHandler {
	return &AdminHandler{
		userService: userService,
		config:      config,
	}
}

// ListUsers handles GET /api/admin/users
func (h *AdminHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.userService.ListUsers(r.Context())
	if err != nil {
		slog.Error("failed to list users", "error", err)
		response.WriteErrorResponse(w, http.StatusInternalServerError, response.InternalError, "Failed to list users", "")
		return
	}

	response.WriteSuccessResponse(w, users, "")
}

// UpdateUserRole handles PUT /api/admin/users/{id}/role
func (h *AdminHandler) UpdateUserRole(w http.ResponseWriter, r *http.Request) {
	// Get the target user ID from the URL path
	userID := r.URL.Query().Get("id")
	if userID == "" {
		response.WriteErrorResponse(w, http.StatusBadRequest, response.BadRequest, "User ID required", "")
		return
	}

	// Get the current user from context
	currentUser, ok := r.Context().Value(middleware.UserKey).(*models.User)
	if !ok {
		response.WriteErrorResponse(w, http.StatusUnauthorized, response.Unauthorized, "Unauthorized", "")
		return
	}

	// Parse the new role from request body
	var req struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteErrorResponse(w, http.StatusBadRequest, response.BadRequest, "Invalid request body", "")
		return
	}

	// Validate the role
	newRole := models.Role(req.Role)
	if !newRole.IsValid() {
		response.WriteErrorResponse(w, http.StatusBadRequest, response.BadRequest, "Invalid role", "")
		return
	}

	// Get the target user
	targetUser, err := h.userService.GetUser(r.Context(), userID)
	if err != nil {
		slog.Error("failed to get target user", "error", err, "user_id", userID)
		response.WriteErrorResponse(w, http.StatusNotFound, response.NotFound, "User not found", "")
		return
	}

	// Check if current user can manage the target user
	if !currentUser.CanManageUser(targetUser) {
		response.WriteErrorResponse(w, http.StatusForbidden, response.Forbidden, "Cannot manage this user", "")
		return
	}

	// Additional check: only admins can promote to admin
	if newRole == models.RoleAdmin && currentUser.Role != models.RoleAdmin {
		response.WriteErrorResponse(w, http.StatusForbidden, response.Forbidden, "Only admins can promote to admin", "")
		return
	}

	// Update the user's role
	targetUser.Role = newRole
	targetUser.UpdatedAt = time.Now()

	if err := h.userService.UpdateUser(r.Context(), targetUser); err != nil {
		slog.Error("failed to update user role", "error", err, "user_id", userID, "new_role", newRole)
		response.WriteErrorResponse(w, http.StatusInternalServerError, response.InternalError, "Failed to update user role", "")
		return
	}

	slog.Info("user role updated",
		"admin_user_id", currentUser.ID,
		"target_user_id", userID,
		"old_role", targetUser.Role,
		"new_role", newRole)

	userResponse := map[string]interface{}{
		"user": targetUser,
	}
	response.WriteSuccessResponse(w, userResponse, "User role updated successfully")
}

// ToggleUserStatus handles PUT /api/admin/users/{id}/status
func (h *AdminHandler) ToggleUserStatus(w http.ResponseWriter, r *http.Request) {
	// Get the target user ID from the URL path
	userID := r.URL.Query().Get("id")
	if userID == "" {
		response.WriteErrorResponse(w, http.StatusBadRequest, response.BadRequest, "User ID required", "")
		return
	}

	// Get the current user from context
	currentUser, ok := r.Context().Value(middleware.UserKey).(*models.User)
	if !ok {
		response.WriteErrorResponse(w, http.StatusUnauthorized, response.Unauthorized, "Unauthorized", "")
		return
	}

	// Parse the new status from request body
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteErrorResponse(w, http.StatusBadRequest, response.BadRequest, "Invalid request body", "")
		return
	}

	// Get the target user
	targetUser, err := h.userService.GetUser(r.Context(), userID)
	if err != nil {
		slog.Error("failed to get target user", "error", err, "user_id", userID)
		response.WriteErrorResponse(w, http.StatusNotFound, response.NotFound, "User not found", "")
		return
	}

	// Check if current user can manage the target user
	if !currentUser.CanManageUser(targetUser) {
		response.WriteErrorResponse(w, http.StatusForbidden, response.Forbidden, "Cannot manage this user", "")
		return
	}

	// Update the user's status
	if req.Enabled {
		err = h.userService.EnableUser(r.Context(), userID)
	} else {
		err = h.userService.DisableUser(r.Context(), userID)
	}

	if err != nil {
		slog.Error("failed to update user status", "error", err, "user_id", userID, "enabled", req.Enabled)
		response.WriteErrorResponse(w, http.StatusInternalServerError, response.InternalError, "Failed to update user status", "")
		return
	}

	slog.Info("user status updated",
		"admin_user_id", currentUser.ID,
		"target_user_id", userID,
		"enabled", req.Enabled)

	response.WriteSuccessResponse(w, nil, "User status updated successfully")
}

// GetUserStats handles GET /api/admin/stats
func (h *AdminHandler) GetUserStats(w http.ResponseWriter, r *http.Request) {
	users, err := h.userService.ListUsers(r.Context())
	if err != nil {
		slog.Error("failed to get users for stats", "error", err)
		response.WriteErrorResponse(w, http.StatusInternalServerError, response.InternalError, "Failed to get user statistics", "")
		return
	}

	// Calculate statistics
	stats := map[string]interface{}{
		"total_users":    len(users),
		"enabled_users":  0,
		"disabled_users": 0,
		"roles": map[string]int{
			"user":      0,
			"moderator": 0,
			"admin":     0,
		},
	}

	for _, user := range users {
		if user.Enabled {
			stats["enabled_users"] = stats["enabled_users"].(int) + 1
		} else {
			stats["disabled_users"] = stats["disabled_users"].(int) + 1
		}

		roleStats := stats["roles"].(map[string]int)
		roleStats[string(user.Role)]++
	}

	response.WriteSuccessResponse(w, stats, "")
}

// GetSpoolmanConfig returns the spoolman configuration status
func (h *AdminHandler) GetSpoolmanConfig(w http.ResponseWriter, r *http.Request) {
	spoolmanConfig := struct {
		Enabled bool   `json:"enabled"`
		BaseURL string `json:"base_url,omitempty"`
	}{
		Enabled: h.config.Spoolman.Enabled,
	}

	// If spoolman is enabled, provide the base URL (without /api/v1 suffix)
	if h.config.Spoolman.Enabled {
		spoolmanConfig.BaseURL = h.config.Spoolman.Endpoint
		// Remove /api/v1 suffix if present to get the base URL
		if len(spoolmanConfig.BaseURL) > 7 && spoolmanConfig.BaseURL[len(spoolmanConfig.BaseURL)-7:] == "/api/v1" {
			spoolmanConfig.BaseURL = spoolmanConfig.BaseURL[:len(spoolmanConfig.BaseURL)-7]
		}
	}

	response.WriteSuccessResponse(w, spoolmanConfig, "")
}
