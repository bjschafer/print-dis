package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/bjschafer/print-dis/internal/models"
	"github.com/jmoiron/sqlx"

	_ "github.com/mattn/go-sqlite3"
)

type sqliteClient struct {
	baseClient
	logger *slog.Logger
}

func newSQLiteClient(cfg *Config) (DBClient, error) {
	logger := slog.Default()
	logger.Info("connecting to SQLite database",
		"database", cfg.Database,
	)

	db, err := sqlx.Open("sqlite3", cfg.Database)
	if err != nil {
		logger.Error("failed to open SQLite database",
			"error", err,
			"database", cfg.Database,
		)
		return nil, fmt.Errorf("failed to open SQLite database: %w", err)
	}

	// Note: Database schema is now handled by the migration system in main.go

	return &sqliteClient{
		baseClient: baseClient{db: db},
		logger:     logger,
	}, nil
}

// Printer operations
func (c *sqliteClient) CreatePrinter(ctx context.Context, printer *models.Printer) error {
	query := `INSERT INTO printers (name, dim_x, dim_y, dim_z, url) VALUES (?, ?, ?, ?, ?)`
	result, err := c.db.ExecContext(ctx, query, printer.Name, printer.Dimensions.X, printer.Dimensions.Y, printer.Dimensions.Z, printer.Url)
	if err != nil {
		return fmt.Errorf("failed to create printer: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	printer.Id = int(id)
	return nil
}

func (c *sqliteClient) GetPrinter(ctx context.Context, id int) (*models.Printer, error) {
	query := `SELECT id, name, dim_x as "dimensions.x", dim_y as "dimensions.y", dim_z as "dimensions.z", url FROM printers WHERE id = ?`
	printer := &models.Printer{
		Dimensions: models.Dimension{},
	}
	err := c.db.GetContext(ctx, printer, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get printer: %w", err)
	}
	return printer, nil
}

func (c *sqliteClient) UpdatePrinter(ctx context.Context, printer *models.Printer) error {
	query := `UPDATE printers SET name = ?, dim_x = ?, dim_y = ?, dim_z = ?, url = ? WHERE id = ?`
	_, err := c.db.ExecContext(ctx, query,
		printer.Name,
		printer.Dimensions.X,
		printer.Dimensions.Y,
		printer.Dimensions.Z,
		printer.Url,
		printer.Id,
	)
	if err != nil {
		return fmt.Errorf("failed to update printer: %w", err)
	}
	return nil
}

func (c *sqliteClient) DeletePrinter(ctx context.Context, id int) error {
	query := `DELETE FROM printers WHERE id = ?`
	_, err := c.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete printer: %w", err)
	}
	return nil
}

func (c *sqliteClient) ListPrinters(ctx context.Context) ([]*models.Printer, error) {
	query := `SELECT id, name, dim_x as "dimensions.x", dim_y as "dimensions.y", dim_z as "dimensions.z", url FROM printers`
	printers := []*models.Printer{}
	err := c.db.SelectContext(ctx, &printers, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query printers: %w", err)
	}
	return printers, nil
}

// Filament operations
func (c *sqliteClient) CreateFilament(ctx context.Context, filament *models.Filament) error {
	query := `INSERT INTO filaments (name, material_id) VALUES (?, ?)`
	result, err := c.db.ExecContext(ctx, query, filament.Name, filament.Material.Id)
	if err != nil {
		return fmt.Errorf("failed to create filament: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	filament.Id = int(id)
	return nil
}

func (c *sqliteClient) GetFilament(ctx context.Context, id int) (*models.Filament, error) {
	query := `
		SELECT f.id, f.name, m.id as "material.id", m.name as "material.name" 
		FROM filaments f 
		JOIN materials m ON f.material_id = m.id 
		WHERE f.id = ?`

	filament := &models.Filament{}
	err := c.db.GetContext(ctx, filament, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get filament: %w", err)
	}
	return filament, nil
}

func (c *sqliteClient) UpdateFilament(ctx context.Context, filament *models.Filament) error {
	query := `UPDATE filaments SET name = ?, material_id = ? WHERE id = ?`
	_, err := c.db.ExecContext(ctx, query, filament.Name, filament.Material.Id, filament.Id)
	if err != nil {
		return fmt.Errorf("failed to update filament: %w", err)
	}
	return nil
}

func (c *sqliteClient) DeleteFilament(ctx context.Context, id int) error {
	query := `DELETE FROM filaments WHERE id = ?`
	_, err := c.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete filament: %w", err)
	}
	return nil
}

func (c *sqliteClient) ListFilaments(ctx context.Context) ([]*models.Filament, error) {
	query := `
		SELECT f.id, f.name, m.id as "material.id", m.name as "material.name" 
		FROM filaments f 
		JOIN materials m ON f.material_id = m.id`

	filaments := []*models.Filament{}
	err := c.db.SelectContext(ctx, &filaments, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query filaments: %w", err)
	}
	return filaments, nil
}

// Job operations
func (c *sqliteClient) CreateJob(ctx context.Context, job *models.Job) error {
	query := `INSERT INTO jobs (printer_id, filament_id, material_id) VALUES (?, ?, ?)`
	result, err := c.db.ExecContext(ctx, query, job.Printer.Id, job.Filament.Id, job.Material.Id)
	if err != nil {
		return fmt.Errorf("failed to create job: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	job.Id = int(id)
	return nil
}

func (c *sqliteClient) GetJob(ctx context.Context, id int) (*models.Job, error) {
	query := `
		SELECT 
			j.id,
			p.id as "printer.id", p.name as "printer.name", 
			p.dim_x as "printer.dimensions.x", p.dim_y as "printer.dimensions.y", 
			p.dim_z as "printer.dimensions.z", p.url as "printer.url",
			f.id as "filament.id", f.name as "filament.name",
			m1.id as "filament.material.id", m1.name as "filament.material.name",
			m2.id as "material.id", m2.name as "material.name"
		FROM jobs j
		JOIN printers p ON j.printer_id = p.id
		JOIN filaments f ON j.filament_id = f.id
		JOIN materials m1 ON f.material_id = m1.id
		JOIN materials m2 ON j.material_id = m2.id
		WHERE j.id = ?`

	job := &models.Job{
		Printer: &models.Printer{
			Dimensions: models.Dimension{},
		},
		Filament: &models.Filament{
			Material: models.Material{},
		},
		Material: &models.Material{},
	}
	err := c.db.GetContext(ctx, job, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get job: %w", err)
	}
	return job, nil
}

func (c *sqliteClient) UpdateJob(ctx context.Context, job *models.Job) error {
	query := `UPDATE jobs SET printer_id = ?, filament_id = ?, material_id = ? WHERE id = ?`
	_, err := c.db.ExecContext(ctx, query, job.Printer.Id, job.Filament.Id, job.Material.Id, job.Id)
	if err != nil {
		return fmt.Errorf("failed to update job: %w", err)
	}
	return nil
}

func (c *sqliteClient) DeleteJob(ctx context.Context, id int) error {
	query := `DELETE FROM jobs WHERE id = ?`
	_, err := c.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete job: %w", err)
	}
	return nil
}

func (c *sqliteClient) ListJobs(ctx context.Context) ([]*models.Job, error) {
	query := `
		SELECT 
			j.id,
			p.id as "printer.id", p.name as "printer.name", 
			p.dim_x as "printer.dimensions.x", p.dim_y as "printer.dimensions.y", 
			p.dim_z as "printer.dimensions.z", p.url as "printer.url",
			f.id as "filament.id", f.name as "filament.name",
			m1.id as "filament.material.id", m1.name as "filament.material.name",
			m2.id as "material.id", m2.name as "material.name"
		FROM jobs j
		JOIN printers p ON j.printer_id = p.id
		JOIN filaments f ON j.filament_id = f.id
		JOIN materials m1 ON f.material_id = m1.id
		JOIN materials m2 ON j.material_id = m2.id`

	jobs := []*models.Job{}
	err := c.db.SelectContext(ctx, &jobs, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query jobs: %w", err)
	}
	return jobs, nil
}

// Material operations
func (c *sqliteClient) CreateMaterial(ctx context.Context, material *models.Material) error {
	query := `INSERT INTO materials (name) VALUES (?)`
	result, err := c.db.ExecContext(ctx, query, material.Name)
	if err != nil {
		return fmt.Errorf("failed to create material: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	material.Id = int(id)
	return nil
}

func (c *sqliteClient) GetMaterial(ctx context.Context, id int) (*models.Material, error) {
	query := `SELECT id, name FROM materials WHERE id = ?`
	material := &models.Material{}
	err := c.db.GetContext(ctx, material, query, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get material: %w", err)
	}
	return material, nil
}

func (c *sqliteClient) UpdateMaterial(ctx context.Context, material *models.Material) error {
	query := `UPDATE materials SET name = ? WHERE id = ?`
	_, err := c.db.ExecContext(ctx, query, material.Name, material.Id)
	if err != nil {
		return fmt.Errorf("failed to update material: %w", err)
	}
	return nil
}

func (c *sqliteClient) DeleteMaterial(ctx context.Context, id int) error {
	query := `DELETE FROM materials WHERE id = ?`
	_, err := c.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete material: %w", err)
	}
	return nil
}

func (c *sqliteClient) ListMaterials(ctx context.Context) ([]*models.Material, error) {
	query := `SELECT id, name FROM materials`
	materials := []*models.Material{}
	err := c.db.SelectContext(ctx, &materials, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query materials: %w", err)
	}
	return materials, nil
}

// PrintRequest operations
func (c *sqliteClient) CreatePrintRequest(ctx context.Context, request *models.PrintRequest) error {
	query := `
		INSERT INTO print_requests (
			id, user_id, file_link, notes, spool_id, color, material, status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	c.logger.Debug("executing create print request query",
		"id", request.ID,
		"user_id", request.UserID,
		"status", request.Status,
	)

	_, err := c.db.ExecContext(ctx, query,
		request.ID,
		request.UserID,
		request.FileLink,
		request.Notes,
		request.SpoolID,
		request.Color,
		request.Material,
		request.Status,
		request.CreatedAt,
		request.UpdatedAt,
	)
	if err != nil {
		c.logger.Error("failed to create print request",
			"error", err,
			"id", request.ID,
		)
		return fmt.Errorf("failed to create print request: %w", err)
	}

	return nil
}

func (c *sqliteClient) GetPrintRequest(ctx context.Context, id string) (*models.PrintRequest, error) {
	query := `
		SELECT id, user_id, file_link, notes, spool_id, color, material, status, created_at, updated_at
		FROM print_requests
		WHERE id = ?`

	c.logger.Debug("executing get print request query", "id", id)

	request := &models.PrintRequest{}
	err := c.db.GetContext(ctx, request, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			c.logger.Debug("print request not found", "id", id)
			return nil, nil
		}
		c.logger.Error("failed to get print request",
			"error", err,
			"id", id,
		)
		return nil, fmt.Errorf("failed to get print request: %w", err)
	}

	return request, nil
}

func (c *sqliteClient) UpdatePrintRequest(ctx context.Context, request *models.PrintRequest) error {
	query := `
		UPDATE print_requests
		SET user_id = ?, file_link = ?, notes = ?, spool_id = ?, color = ?,
			material = ?, status = ?, updated_at = ?
		WHERE id = ?`

	c.logger.Debug("executing update print request query",
		"id", request.ID,
		"user_id", request.UserID,
		"status", request.Status,
	)

	_, err := c.db.ExecContext(ctx, query,
		request.UserID,
		request.FileLink,
		request.Notes,
		request.SpoolID,
		request.Color,
		request.Material,
		request.Status,
		request.UpdatedAt,
		request.ID,
	)
	if err != nil {
		c.logger.Error("failed to update print request",
			"error", err,
			"id", request.ID,
		)
		return fmt.Errorf("failed to update print request: %w", err)
	}

	return nil
}

func (c *sqliteClient) DeletePrintRequest(ctx context.Context, id string) error {
	query := `DELETE FROM print_requests WHERE id = ?`

	c.logger.Debug("executing delete print request query", "id", id)

	_, err := c.db.ExecContext(ctx, query, id)
	if err != nil {
		c.logger.Error("failed to delete print request",
			"error", err,
			"id", id,
		)
		return fmt.Errorf("failed to delete print request: %w", err)
	}

	return nil
}

func (c *sqliteClient) ListPrintRequests(ctx context.Context) ([]*models.PrintRequest, error) {
	query := `
		SELECT id, user_id, file_link, notes, spool_id, color, material, status, created_at, updated_at
		FROM print_requests
		ORDER BY created_at DESC`

	c.logger.Debug("executing list print requests query")

	requests := []*models.PrintRequest{}
	err := c.db.SelectContext(ctx, &requests, query)
	if err != nil {
		c.logger.Error("failed to query print requests",
			"error", err,
		)
		return nil, fmt.Errorf("failed to query print requests: %w", err)
	}

	c.logger.Debug("retrieved print requests",
		"count", len(requests),
	)

	return requests, nil
}

// User operations
func (c *sqliteClient) CreateUser(ctx context.Context, user *models.User) error {
	query := `
		INSERT INTO users (id, username, email, password_hash, display_name, role, created_at, updated_at, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	c.logger.Debug("executing create user query", "username", user.Username)

	_, err := c.db.ExecContext(ctx, query,
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
		c.logger.Error("failed to create user", "error", err, "username", user.Username)
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

func (c *sqliteClient) GetUser(ctx context.Context, id string) (*models.User, error) {
	query := `
		SELECT id, username, email, password_hash, display_name, role, created_at, updated_at, enabled
		FROM users
		WHERE id = ?`

	user := &models.User{}
	err := c.db.GetContext(ctx, user, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

func (c *sqliteClient) GetUserByUsername(ctx context.Context, username string) (*models.User, error) {
	query := `
		SELECT id, username, email, password_hash, display_name, role, created_at, updated_at, enabled
		FROM users
		WHERE username = ?`

	user := &models.User{}
	err := c.db.GetContext(ctx, user, query, username)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user by username: %w", err)
	}

	return user, nil
}

func (c *sqliteClient) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	query := `
		SELECT id, username, email, password_hash, display_name, role, created_at, updated_at, enabled
		FROM users
		WHERE email = ?`

	user := &models.User{}
	err := c.db.GetContext(ctx, user, query, email)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}

	return user, nil
}

func (c *sqliteClient) UpdateUser(ctx context.Context, user *models.User) error {
	query := `
		UPDATE users
		SET username = ?, email = ?, password_hash = ?, display_name = ?, role = ?, updated_at = ?, enabled = ?
		WHERE id = ?`

	_, err := c.db.ExecContext(ctx, query,
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

func (c *sqliteClient) DeleteUser(ctx context.Context, id string) error {
	query := `DELETE FROM users WHERE id = ?`
	_, err := c.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	return nil
}

func (c *sqliteClient) ListUsers(ctx context.Context) ([]*models.User, error) {
	query := `
		SELECT id, username, email, password_hash, display_name, role, created_at, updated_at, enabled
		FROM users
		ORDER BY created_at DESC`

	users := []*models.User{}
	err := c.db.SelectContext(ctx, &users, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query users: %w", err)
	}

	return users, nil
}
