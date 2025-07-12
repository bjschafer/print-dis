# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Print-dis is a Go web application for managing 3D printing requests with user authentication, role-based access control, and optional Spoolman integration. It serves static HTML/JS/CSS frontend files and provides a REST API backend.

## Development Commands

### Building and Running
- `make build` - Build the application binary to `bin/print-dis`
- `make clean` - Remove build artifacts
- `./bin/print-dis` - Run the application with default configuration
- `./bin/print-dis --config /path/to/config.yaml` - Run with custom config file

### Testing and Quality
- `make test` - Run all tests with race detection and coverage
- `make lint` - Run golangci-lint on all packages
- `make check` - Run both tests and linting
- `go test ./internal/services/...` - Run tests for a specific package
- `go test -run TestSpecificFunction` - Run a specific test function

### Code Generation
- `make generate` - Run go generate to update generated code (including enums)

## Architecture

### Backend Structure
- **main.go** - Application entry point with HTTP server setup and routing
- **internal/config/** - Configuration management using Viper (supports config files, env vars, CLI flags)
- **internal/database/** - Database abstraction layer supporting SQLite and PostgreSQL
- **internal/models/** - Data models and database schema definitions
- **internal/services/** - Business logic layer
- **internal/handlers/** - HTTP request handlers
- **internal/middleware/** - Authentication, session management, and permission middleware
- **internal/api/** - External API integrations (Spoolman)
- **internal/spoolman/** - Spoolman 3D printing management system integration

### Frontend Structure
- **static/** - All frontend assets (HTML, CSS, JavaScript)
- Single-page applications with vanilla JavaScript
- Session-based authentication with role-based UI elements

### Database Support
- SQLite (default) and PostgreSQL support
- Database migrations and schema handled in database package
- Uses sqlx for database operations

### Configuration
Configuration sources in order of precedence:
1. Command-line flags
2. Environment variables (prefixed with `PRINT_DIS_`)
3. Configuration file (`config.yaml`)
4. Default values

### Authentication & Authorization
- Session-based authentication using gorilla/sessions
- Three roles: User, Moderator, Admin
- Role-based middleware for API endpoints
- Password hashing with bcrypt

### Key Features
- Print request lifecycle management (Pending → Enqueued → In Progress → Done)
- User dashboard with filtering and search
- Admin interface for user and request management
- Optional Spoolman integration for filament/material data
- Responsive web interface

### External Integrations
- **Spoolman** (optional) - 3D printing management system for filament tracking
- Integration controlled by `spoolman.enabled` config option

## Testing
- Test files follow `*_test.go` naming convention
- Uses testify for assertions and test utilities
- Current test coverage includes database, services, and spoolman client
- Run with race detection enabled by default

## Enum Generation
- Uses `github.com/dmarkham/enumer` for generating enum methods
- Generated files have `_gen.go` suffix
- Run `make generate` after modifying enum types