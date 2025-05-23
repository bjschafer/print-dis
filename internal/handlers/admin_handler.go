package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/bjschafer/print-dis/internal/middleware"
	"github.com/bjschafer/print-dis/internal/models"
	"github.com/bjschafer/print-dis/internal/services"
)

// AdminHandler handles admin-related HTTP requests
type AdminHandler struct {
	userService *services.UserService
}

// NewAdminHandler creates a new AdminHandler
func NewAdminHandler(userService *services.UserService) *AdminHandler {
	return &AdminHandler{
		userService: userService,
	}
}

// ListUsers handles GET /api/admin/users
func (h *AdminHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.userService.ListUsers(r.Context())
	if err != nil {
		slog.Error("failed to list users", "error", err)
		http.Error(w, "Failed to list users", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(users); err != nil {
		slog.Error("failed to encode users response", "error", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// UpdateUserRole handles PUT /api/admin/users/{id}/role
func (h *AdminHandler) UpdateUserRole(w http.ResponseWriter, r *http.Request) {
	// Get the target user ID from the URL path
	userID := r.URL.Query().Get("id")
	if userID == "" {
		http.Error(w, "User ID required", http.StatusBadRequest)
		return
	}

	// Get the current user from context
	currentUser, ok := r.Context().Value(middleware.UserKey).(*models.User)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse the new role from request body
	var req struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate the role
	newRole := models.Role(req.Role)
	if !newRole.IsValid() {
		http.Error(w, "Invalid role", http.StatusBadRequest)
		return
	}

	// Get the target user
	targetUser, err := h.userService.GetUser(r.Context(), userID)
	if err != nil {
		slog.Error("failed to get target user", "error", err, "user_id", userID)
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Check if current user can manage the target user
	if !currentUser.CanManageUser(targetUser) {
		http.Error(w, "Forbidden: Cannot manage this user", http.StatusForbidden)
		return
	}

	// Additional check: only admins can promote to admin
	if newRole == models.RoleAdmin && currentUser.Role != models.RoleAdmin {
		http.Error(w, "Forbidden: Only admins can promote to admin", http.StatusForbidden)
		return
	}

	// Update the user's role
	targetUser.Role = newRole
	targetUser.UpdatedAt = time.Now()

	if err := h.userService.UpdateUser(r.Context(), targetUser); err != nil {
		slog.Error("failed to update user role", "error", err, "user_id", userID, "new_role", newRole)
		http.Error(w, "Failed to update user role", http.StatusInternalServerError)
		return
	}

	slog.Info("user role updated",
		"admin_user_id", currentUser.ID,
		"target_user_id", userID,
		"old_role", targetUser.Role,
		"new_role", newRole)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "User role updated successfully",
		"user":    targetUser,
	})
}

// ToggleUserStatus handles PUT /api/admin/users/{id}/status
func (h *AdminHandler) ToggleUserStatus(w http.ResponseWriter, r *http.Request) {
	// Get the target user ID from the URL path
	userID := r.URL.Query().Get("id")
	if userID == "" {
		http.Error(w, "User ID required", http.StatusBadRequest)
		return
	}

	// Get the current user from context
	currentUser, ok := r.Context().Value(middleware.UserKey).(*models.User)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse the new status from request body
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get the target user
	targetUser, err := h.userService.GetUser(r.Context(), userID)
	if err != nil {
		slog.Error("failed to get target user", "error", err, "user_id", userID)
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Check if current user can manage the target user
	if !currentUser.CanManageUser(targetUser) {
		http.Error(w, "Forbidden: Cannot manage this user", http.StatusForbidden)
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
		http.Error(w, "Failed to update user status", http.StatusInternalServerError)
		return
	}

	slog.Info("user status updated",
		"admin_user_id", currentUser.ID,
		"target_user_id", userID,
		"enabled", req.Enabled)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "User status updated successfully",
	})
}

// GetUserStats handles GET /api/admin/stats
func (h *AdminHandler) GetUserStats(w http.ResponseWriter, r *http.Request) {
	users, err := h.userService.ListUsers(r.Context())
	if err != nil {
		slog.Error("failed to get users for stats", "error", err)
		http.Error(w, "Failed to get user statistics", http.StatusInternalServerError)
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
