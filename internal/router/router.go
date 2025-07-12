package router

import (
	"log/slog"
	"net/http"

	"github.com/bjschafer/print-dis/internal/api"
	"github.com/bjschafer/print-dis/internal/config"
	"github.com/bjschafer/print-dis/internal/handlers"
	"github.com/bjschafer/print-dis/internal/middleware"
	"github.com/bjschafer/print-dis/internal/response"
)

// Dependencies holds all the dependencies needed for route setup
type Dependencies struct {
	Config               *config.Config
	SessionStore         *middleware.SessionStore
	PrintRequestHandler  *handlers.PrintRequestHandler
	AuthHandler          *handlers.AuthHandler
	AdminHandler         *handlers.AdminHandler
	SpoolmanHandler      *api.SpoolmanHandler
}

// SetupRoutes configures all application routes
func SetupRoutes(mux *http.ServeMux, deps *Dependencies) {
	// Serve static files
	fs := http.FileServer(http.Dir("static"))
	mux.Handle("/", fs)

	// Setup API routes
	setupAuthRoutes(mux, deps)
	setupPrintRequestRoutes(mux, deps)
	setupUserRoutes(mux, deps)
	setupAdminRoutes(mux, deps)
	setupSpoolmanRoutes(mux, deps)
}

// setupAuthRoutes configures authentication-related routes
func setupAuthRoutes(mux *http.ServeMux, deps *Dependencies) {
	sessionMW := deps.SessionStore.SessionMiddleware()
	authMW := deps.SessionStore.AuthMiddleware(deps.Config)
	authRateLimit := middleware.AuthRateLimit() // Strict rate limiting for auth endpoints

	// Public auth endpoints (no auth required, but rate limited)
	mux.Handle("/api/auth/login", authRateLimit(sessionMW(http.HandlerFunc(deps.AuthHandler.Login))))
	mux.Handle("/api/auth/logout", sessionMW(http.HandlerFunc(deps.AuthHandler.Logout)))
	mux.Handle("/api/auth/register", authRateLimit(sessionMW(http.HandlerFunc(deps.AuthHandler.Register))))

	// Protected auth endpoints (auth required)
	mux.Handle("/api/auth/me", sessionMW(authMW(http.HandlerFunc(deps.AuthHandler.GetCurrentUser))))
	mux.Handle("/api/auth/change-password", sessionMW(authMW(http.HandlerFunc(deps.AuthHandler.ChangePassword))))
}

// setupPrintRequestRoutes configures print request-related routes
func setupPrintRequestRoutes(mux *http.ServeMux, deps *Dependencies) {
	sessionMW := deps.SessionStore.SessionMiddleware()
	authMW := deps.SessionStore.AuthMiddleware(deps.Config)
	apiRateLimit := middleware.APIRateLimit() // General API rate limiting

	// Main print requests endpoint with method-based routing
	printRequestHandler := createPrintRequestHandler(deps.PrintRequestHandler)
	mux.Handle("/api/print-requests", apiRateLimit(sessionMW(authMW(printRequestHandler))))

	// Print request status updates
	statusHandler := createPrintRequestStatusHandler(deps.PrintRequestHandler)
	mux.Handle("/api/print-requests/status", apiRateLimit(sessionMW(authMW(statusHandler))))
}

// setupUserRoutes configures user-specific routes
func setupUserRoutes(mux *http.ServeMux, deps *Dependencies) {
	sessionMW := deps.SessionStore.SessionMiddleware()
	authMW := deps.SessionStore.AuthMiddleware(deps.Config)
	apiRateLimit := middleware.APIRateLimit() // General API rate limiting

	// User-specific print requests
	userRequestsHandler := createUserRequestsHandler(deps.PrintRequestHandler)
	mux.Handle("/api/user/print-requests", apiRateLimit(sessionMW(authMW(userRequestsHandler))))
}

// setupAdminRoutes configures admin-only routes
func setupAdminRoutes(mux *http.ServeMux, deps *Dependencies) {
	sessionMW := deps.SessionStore.SessionMiddleware()
	authMW := deps.SessionStore.AuthMiddleware(deps.Config)
	modMW := middleware.RequireModerator(deps.SessionStore, deps.Config)
	adminMW := middleware.RequireAdmin(deps.SessionStore, deps.Config)
	apiRateLimit := middleware.APIRateLimit() // General API rate limiting

	// Admin user management
	adminUsersHandler := createAdminUsersHandler(deps.AdminHandler)
	mux.Handle("/api/admin/users", apiRateLimit(sessionMW(authMW(modMW(adminUsersHandler)))))

	// Admin user role updates (admin only)
	adminUserRoleHandler := createAdminUserRoleHandler(deps.AdminHandler)
	mux.Handle("/api/admin/users/role", apiRateLimit(sessionMW(authMW(adminMW(adminUserRoleHandler)))))

	// Admin user status updates (moderator+)
	adminUserStatusHandler := createAdminUserStatusHandler(deps.AdminHandler)
	mux.Handle("/api/admin/users/status", apiRateLimit(sessionMW(authMW(modMW(adminUserStatusHandler)))))

	// Admin stats (moderator+)
	adminStatsHandler := createAdminStatsHandler(deps.AdminHandler)
	mux.Handle("/api/admin/stats", apiRateLimit(sessionMW(authMW(modMW(adminStatsHandler)))))

	// Admin print requests with enhanced details (moderator+)
	adminPrintRequestsHandler := createAdminPrintRequestsHandler(deps.PrintRequestHandler)
	mux.Handle("/api/admin/print-requests", apiRateLimit(sessionMW(authMW(modMW(adminPrintRequestsHandler)))))

	// Admin spoolman config (moderator+)
	adminSpoolmanConfigHandler := createAdminSpoolmanConfigHandler(deps.AdminHandler)
	mux.Handle("/api/admin/spoolman-config", apiRateLimit(sessionMW(authMW(modMW(adminSpoolmanConfigHandler)))))
}

// setupSpoolmanRoutes configures Spoolman integration routes (if enabled)
func setupSpoolmanRoutes(mux *http.ServeMux, deps *Dependencies) {
	if deps.SpoolmanHandler == nil {
		return
	}

	sessionMW := deps.SessionStore.SessionMiddleware()
	authMW := deps.SessionStore.AuthMiddleware(deps.Config)
	apiRateLimit := middleware.APIRateLimit() // General API rate limiting

	// Spoolman spools endpoints
	spoolsHandler := createSpoolsHandler(deps.SpoolmanHandler)
	mux.Handle("/api/spoolman/spools", apiRateLimit(sessionMW(authMW(spoolsHandler))))

	spoolHandler := createSpoolHandler(deps.SpoolmanHandler)
	mux.Handle("/api/spoolman/spool", apiRateLimit(sessionMW(authMW(spoolHandler))))

	materialsHandler := createMaterialsHandler(deps.SpoolmanHandler)
	mux.Handle("/api/spoolman/materials", apiRateLimit(sessionMW(authMW(materialsHandler))))
}

// Handler creation functions with proper method routing and error handling

func createPrintRequestHandler(handler *handlers.PrintRequestHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			handler.CreatePrintRequest(w, r)
		case http.MethodGet:
			if r.URL.Query().Get("id") != "" {
				handler.GetPrintRequest(w, r)
			} else {
				handler.ListPrintRequests(w, r)
			}
		case http.MethodPut:
			handler.UpdatePrintRequest(w, r)
		case http.MethodDelete:
			handler.DeletePrintRequest(w, r)
		default:
			slog.Warn("invalid method for print requests endpoint",
				"method", r.Method,
				"path", r.URL.Path,
			)
			response.WriteErrorResponse(w, http.StatusMethodNotAllowed, response.BadRequest, "Method not allowed", "")
		}
	})
}

func createPrintRequestStatusHandler(handler *handlers.PrintRequestHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			handler.UpdatePrintRequestStatus(w, r)
		} else {
			slog.Warn("invalid method for print request status endpoint",
				"method", r.Method,
				"path", r.URL.Path,
			)
			response.WriteErrorResponse(w, http.StatusMethodNotAllowed, response.BadRequest, "Method not allowed", "")
		}
	})
}

func createUserRequestsHandler(handler *handlers.PrintRequestHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handler.ListUserPrintRequests(w, r)
		} else {
			response.WriteErrorResponse(w, http.StatusMethodNotAllowed, response.BadRequest, "Method not allowed", "")
		}
	})
}

func createAdminUsersHandler(handler *handlers.AdminHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handler.ListUsers(w, r)
		default:
			response.WriteErrorResponse(w, http.StatusMethodNotAllowed, response.BadRequest, "Method not allowed", "")
		}
	})
}

func createAdminUserRoleHandler(handler *handlers.AdminHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			handler.UpdateUserRole(w, r)
		default:
			response.WriteErrorResponse(w, http.StatusMethodNotAllowed, response.BadRequest, "Method not allowed", "")
		}
	})
}

func createAdminUserStatusHandler(handler *handlers.AdminHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			handler.ToggleUserStatus(w, r)
		default:
			response.WriteErrorResponse(w, http.StatusMethodNotAllowed, response.BadRequest, "Method not allowed", "")
		}
	})
}

func createAdminStatsHandler(handler *handlers.AdminHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handler.GetUserStats(w, r)
		default:
			response.WriteErrorResponse(w, http.StatusMethodNotAllowed, response.BadRequest, "Method not allowed", "")
		}
	})
}

func createAdminPrintRequestsHandler(handler *handlers.PrintRequestHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handler.ListPrintRequestsEnhanced(w, r)
		} else {
			response.WriteErrorResponse(w, http.StatusMethodNotAllowed, response.BadRequest, "Method not allowed", "")
		}
	})
}

func createAdminSpoolmanConfigHandler(handler *handlers.AdminHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handler.GetSpoolmanConfig(w, r)
		} else {
			response.WriteErrorResponse(w, http.StatusMethodNotAllowed, response.BadRequest, "Method not allowed", "")
		}
	})
}

func createSpoolsHandler(handler *api.SpoolmanHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handler.GetSpools(w, r)
		} else {
			response.WriteErrorResponse(w, http.StatusMethodNotAllowed, response.BadRequest, "Method not allowed", "")
		}
	})
}

func createSpoolHandler(handler *api.SpoolmanHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handler.GetSpool(w, r)
		} else {
			response.WriteErrorResponse(w, http.StatusMethodNotAllowed, response.BadRequest, "Method not allowed", "")
		}
	})
}

func createMaterialsHandler(handler *api.SpoolmanHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handler.GetMaterials(w, r)
		} else {
			response.WriteErrorResponse(w, http.StatusMethodNotAllowed, response.BadRequest, "Method not allowed", "")
		}
	})
}