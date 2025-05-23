package middleware

import (
	"net/http"

	"github.com/bjschafer/print-dis/internal/config"
	"github.com/bjschafer/print-dis/internal/models"
)

// RequirePermission creates middleware that requires the user to have a specific permission
func RequirePermission(sessionStore *SessionStore, cfg *config.Config, permission models.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get user from context (set by auth middleware)
			user, ok := r.Context().Value(UserKey).(*models.User)
			if !ok {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Check if user has the required permission
			if !user.HasPermission(permission) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireRole creates middleware that requires the user to have a specific role or higher
func RequireRole(sessionStore *SessionStore, cfg *config.Config, requiredRole models.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get user from context (set by auth middleware)
			user, ok := r.Context().Value(UserKey).(*models.User)
			if !ok {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Check role hierarchy
			if !hasRoleOrHigher(user.Role, requiredRole) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAdmin creates middleware that requires admin role
func RequireAdmin(sessionStore *SessionStore, cfg *config.Config) func(http.Handler) http.Handler {
	return RequireRole(sessionStore, cfg, models.RoleAdmin)
}

// RequireModerator creates middleware that requires moderator role or higher
func RequireModerator(sessionStore *SessionStore, cfg *config.Config) func(http.Handler) http.Handler {
	return RequireRole(sessionStore, cfg, models.RoleModerator)
}

// hasRoleOrHigher checks if the user role is equal to or higher than the required role
func hasRoleOrHigher(userRole, requiredRole models.Role) bool {
	roleHierarchy := map[models.Role]int{
		models.RoleUser:      1,
		models.RoleModerator: 2,
		models.RoleAdmin:     3,
	}

	userLevel, userExists := roleHierarchy[userRole]
	requiredLevel, requiredExists := roleHierarchy[requiredRole]

	if !userExists || !requiredExists {
		return false
	}

	return userLevel >= requiredLevel
}

// CanManageUsersMiddleware checks if the user can manage other users
func CanManageUsersMiddleware(sessionStore *SessionStore, cfg *config.Config) func(http.Handler) http.Handler {
	return RequirePermission(sessionStore, cfg, models.PermissionManageUsers)
}

// CanAccessAdminMiddleware checks if the user can access admin features
func CanAccessAdminMiddleware(sessionStore *SessionStore, cfg *config.Config) func(http.Handler) http.Handler {
	return RequirePermission(sessionStore, cfg, models.PermissionAccessAdmin)
}
