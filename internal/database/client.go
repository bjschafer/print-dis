package database

import (
	"context"
	"fmt"

	"github.com/bjschafer/print-dis/internal/models"
	"github.com/jmoiron/sqlx"
)

// DBClient defines the interface for database operations
type DBClient interface {
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

	// Close closes the database connection
	Close() error
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

// Close implements the DBClient interface
func (c *baseClient) Close() error {
	return c.db.Close()
}
