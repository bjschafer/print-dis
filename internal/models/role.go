package models

// Role represents the different user roles in the system
type Role string

const (
	// RoleUser is the default role for regular users
	RoleUser Role = "user"
	// RoleModerator can manage print requests and view user information
	RoleModerator Role = "moderator"
	// RoleAdmin has full system access including user management
	RoleAdmin Role = "admin"
)

// IsValid checks if the role is a valid role value
func (r Role) IsValid() bool {
	switch r {
	case RoleUser, RoleModerator, RoleAdmin:
		return true
	default:
		return false
	}
}

// String returns the string representation of the role
func (r Role) String() string {
	return string(r)
}

// HasPermission checks if the role has the specified permission
func (r Role) HasPermission(permission Permission) bool {
	switch r {
	case RoleAdmin:
		// Admin has all permissions
		return true
	case RoleModerator:
		// Moderator can manage print requests and view users
		return permission == PermissionViewUsers ||
			permission == PermissionManagePrintRequests ||
			permission == PermissionViewOwnPrintRequests ||
			permission == PermissionCreatePrintRequests
	case RoleUser:
		// User can only manage their own print requests
		return permission == PermissionViewOwnPrintRequests ||
			permission == PermissionCreatePrintRequests
	default:
		return false
	}
}

// Permission represents specific system permissions
type Permission string

const (
	// Print request permissions
	PermissionCreatePrintRequests  Permission = "create_print_requests"
	PermissionViewOwnPrintRequests Permission = "view_own_print_requests"
	PermissionManagePrintRequests  Permission = "manage_print_requests"

	// User management permissions
	PermissionViewUsers    Permission = "view_users"
	PermissionManageUsers  Permission = "manage_users"
	PermissionPromoteUsers Permission = "promote_users"

	// System permissions
	PermissionAccessAdmin     Permission = "access_admin"
	PermissionViewSystemStats Permission = "view_system_stats"
)

// DefaultRole returns the default role for new users
func DefaultRole() Role {
	return RoleUser
}

// AllRoles returns all available roles
func AllRoles() []Role {
	return []Role{RoleUser, RoleModerator, RoleAdmin}
}
