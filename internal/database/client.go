package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/bjschafer/print-dis/internal/models"
	"github.com/jmoiron/sqlx"
)

// DBClient defines the interface for database operations
type DBClient interface {
	// User operations
	CreateUser(ctx context.Context, user *models.User) error
	GetUser(ctx context.Context, id string) (*models.User, error)
	GetUserByUsername(ctx context.Context, username string) (*models.User, error)
	GetUserByEmail(ctx context.Context, email string) (*models.User, error)
	UpdateUser(ctx context.Context, user *models.User) error
	DeleteUser(ctx context.Context, id string) error
	ListUsers(ctx context.Context) ([]*models.User, error)

	// Material operations
	CreateMaterial(ctx context.Context, material *models.Material) error
	GetMaterial(ctx context.Context, id int) (*models.Material, error)
	UpdateMaterial(ctx context.Context, material *models.Material) error
	DeleteMaterial(ctx context.Context, id int) error
	ListMaterials(ctx context.Context) ([]*models.Material, error)

	// Printer operations
	CreatePrinter(ctx context.Context, printer *models.Printer) error
	GetPrinter(ctx context.Context, id int) (*models.Printer, error)
	UpdatePrinter(ctx context.Context, printer *models.Printer) error
	DeletePrinter(ctx context.Context, id int) error
	ListPrinters(ctx context.Context) ([]*models.Printer, error)

	// Filament operations
	CreateFilament(ctx context.Context, filament *models.Filament) error
	GetFilament(ctx context.Context, id int) (*models.Filament, error)
	UpdateFilament(ctx context.Context, filament *models.Filament) error
	DeleteFilament(ctx context.Context, id int) error
	ListFilaments(ctx context.Context) ([]*models.Filament, error)

	// Job operations
	CreateJob(ctx context.Context, job *models.Job) error
	GetJob(ctx context.Context, id int) (*models.Job, error)
	UpdateJob(ctx context.Context, job *models.Job) error
	DeleteJob(ctx context.Context, id int) error
	ListJobs(ctx context.Context) ([]*models.Job, error)

	// PrintRequest operations
	CreatePrintRequest(ctx context.Context, request *models.PrintRequest) error
	GetPrintRequest(ctx context.Context, id string) (*models.PrintRequest, error)
	UpdatePrintRequest(ctx context.Context, request *models.PrintRequest) error
	DeletePrintRequest(ctx context.Context, id string) error
	ListPrintRequests(ctx context.Context) ([]*models.PrintRequest, error)

	// Transaction operations
	BeginTx(ctx context.Context) (Tx, error)

	// Close closes the database connection
	Close() error

	// GetDB returns the underlying database connection for migration purposes
	GetDB() *sql.DB
}

// Tx defines the interface for database transactions
type Tx interface {
	// User operations
	CreateUser(ctx context.Context, user *models.User) error
	GetUser(ctx context.Context, id string) (*models.User, error)
	GetUserByUsername(ctx context.Context, username string) (*models.User, error)
	GetUserByEmail(ctx context.Context, email string) (*models.User, error)
	UpdateUser(ctx context.Context, user *models.User) error

	// PrintRequest operations
	CreatePrintRequest(ctx context.Context, request *models.PrintRequest) error
	GetPrintRequest(ctx context.Context, id string) (*models.PrintRequest, error)
	UpdatePrintRequest(ctx context.Context, request *models.PrintRequest) error

	// Transaction control
	Commit() error
	Rollback() error
}

// Config holds the database configuration
type Config struct {
	Type     string // "sqlite" or "postgres"
	Host     string
	Port     int
	User     string
	Password string
	Database string
	SSLMode  string
}

// NewDBClient creates a new database client based on the configuration
func NewDBClient(cfg *Config) (DBClient, error) {
	switch cfg.Type {
	case "sqlite":
		return newSQLiteClient(cfg)
	case "postgres":
		return newPostgresClient(cfg)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Type)
	}
}

// baseClient provides common functionality for all database implementations
type baseClient struct {
	db *sqlx.DB
}

// BeginTx starts a new transaction
func (c *baseClient) BeginTx(ctx context.Context) (Tx, error) {
	tx, err := c.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	return &txWrapper{tx: tx}, nil
}

// Close implements the DBClient interface
func (c *baseClient) Close() error {
	return c.db.Close()
}

// GetDB returns the underlying database connection for migration purposes
func (c *baseClient) GetDB() *sql.DB {
	return c.db.DB
}

// txWrapper wraps an sqlx.Tx to implement the Tx interface
type txWrapper struct {
	tx *sqlx.Tx
}

// Commit commits the transaction
func (t *txWrapper) Commit() error {
	return t.tx.Commit()
}

// Rollback rolls back the transaction
func (t *txWrapper) Rollback() error {
	return t.tx.Rollback()
}

// CreateUser creates a new user within the transaction
func (t *txWrapper) CreateUser(ctx context.Context, user *models.User) error {
	query := `
		INSERT INTO users (id, username, email, password_hash, display_name, role, created_at, updated_at, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := t.tx.ExecContext(ctx, query,
		user.ID,
		user.Username,
		user.Email,
		user.PasswordHash,
		user.DisplayName,
		user.Role,
		user.CreatedAt,
		user.UpdatedAt,
		user.Enabled,
	)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}
	return nil
}

// GetUser retrieves a user by ID within the transaction
func (t *txWrapper) GetUser(ctx context.Context, id string) (*models.User, error) {
	var user models.User
	query := `SELECT id, username, email, password_hash, display_name, role, created_at, updated_at, enabled FROM users WHERE id = ?`
	
	err := t.tx.GetContext(ctx, &user, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return &user, nil
}

// GetUserByUsername retrieves a user by username within the transaction
func (t *txWrapper) GetUserByUsername(ctx context.Context, username string) (*models.User, error) {
	var user models.User
	query := `SELECT id, username, email, password_hash, display_name, role, created_at, updated_at, enabled FROM users WHERE username = ?`
	
	err := t.tx.GetContext(ctx, &user, query, username)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user by username: %w", err)
	}
	return &user, nil
}

// GetUserByEmail retrieves a user by email within the transaction
func (t *txWrapper) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	query := `SELECT id, username, email, password_hash, display_name, role, created_at, updated_at, enabled FROM users WHERE email = ?`
	
	err := t.tx.GetContext(ctx, &user, query, email)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}
	return &user, nil
}

// UpdateUser updates a user within the transaction
func (t *txWrapper) UpdateUser(ctx context.Context, user *models.User) error {
	query := `
		UPDATE users 
		SET username = ?, email = ?, password_hash = ?, display_name = ?, role = ?, updated_at = ?, enabled = ?
		WHERE id = ?`

	_, err := t.tx.ExecContext(ctx, query,
		user.Username,
		user.Email,
		user.PasswordHash,
		user.DisplayName,
		user.Role,
		user.UpdatedAt,
		user.Enabled,
		user.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}
	return nil
}

// CreatePrintRequest creates a new print request within the transaction
func (t *txWrapper) CreatePrintRequest(ctx context.Context, request *models.PrintRequest) error {
	query := `
		INSERT INTO print_requests (id, user_id, file_link, notes, material, color, status, spool_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := t.tx.ExecContext(ctx, query,
		request.ID,
		request.UserID,
		request.FileLink,
		request.Notes,
		request.Material,
		request.Color,
		request.Status,
		request.SpoolID,
		request.CreatedAt,
		request.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create print request: %w", err)
	}
	return nil
}

// GetPrintRequest retrieves a print request by ID within the transaction
func (t *txWrapper) GetPrintRequest(ctx context.Context, id string) (*models.PrintRequest, error) {
	var request models.PrintRequest
	query := `SELECT id, user_id, file_link, notes, material, color, status, spool_id, created_at, updated_at FROM print_requests WHERE id = ?`
	
	err := t.tx.GetContext(ctx, &request, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get print request: %w", err)
	}
	return &request, nil
}

// UpdatePrintRequest updates a print request within the transaction
func (t *txWrapper) UpdatePrintRequest(ctx context.Context, request *models.PrintRequest) error {
	query := `
		UPDATE print_requests 
		SET file_link = ?, notes = ?, material = ?, color = ?, status = ?, spool_id = ?, updated_at = ?
		WHERE id = ?`

	_, err := t.tx.ExecContext(ctx, query,
		request.FileLink,
		request.Notes,
		request.Material,
		request.Color,
		request.Status,
		request.SpoolID,
		request.UpdatedAt,
		request.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update print request: %w", err)
	}
	return nil
}
