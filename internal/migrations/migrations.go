package migrations

import (
	"database/sql"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
)

// Migration represents a database migration
type Migration struct {
	Version     int
	Description string
	UpSQL       string
	DownSQL     string
}

// Migrator handles database migrations with versioning and rollback support
type Migrator struct {
	db         *sql.DB
	dbType     string // "sqlite" or "postgres"
	migrations []Migration
}

// NewMigrator creates a new migration manager
func NewMigrator(db *sql.DB, dbType string) *Migrator {
	return &Migrator{
		db:         db,
		dbType:     dbType,
		migrations: getAllMigrations(dbType),
	}
}

// EnsureSchemaVersionTable creates the schema version tracking table if it doesn't exist
func (m *Migrator) EnsureSchemaVersionTable() error {
	var createTableSQL string

	switch m.dbType {
	case "sqlite":
		createTableSQL = `
			CREATE TABLE IF NOT EXISTS schema_versions (
				version INTEGER PRIMARY KEY,
				description TEXT NOT NULL,
				applied_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				checksum TEXT
			)`
	case "postgres":
		createTableSQL = `
			CREATE TABLE IF NOT EXISTS schema_versions (
				version INTEGER PRIMARY KEY,
				description TEXT NOT NULL,
				applied_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
				checksum TEXT
			)`
	default:
		return fmt.Errorf("unsupported database type: %s", m.dbType)
	}

	if _, err := m.db.Exec(createTableSQL); err != nil {
		return fmt.Errorf("failed to create schema_versions table: %w", err)
	}

	return nil
}

// GetCurrentVersion returns the current schema version
func (m *Migrator) GetCurrentVersion() (int, error) {
	var version int
	err := m.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_versions").Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("failed to get current version: %w", err)
	}
	return version, nil
}

// GetAppliedMigrations returns a list of applied migration versions
func (m *Migrator) GetAppliedMigrations() ([]int, error) {
	rows, err := m.db.Query("SELECT version FROM schema_versions ORDER BY version")
	if err != nil {
		return nil, fmt.Errorf("failed to query applied migrations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var versions []int
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("failed to scan migration version: %w", err)
		}
		versions = append(versions, version)
	}

	return versions, nil
}

// Up runs all pending migrations up to the latest version
func (m *Migrator) Up() error {
	return m.MigrateTo(-1) // -1 means migrate to latest
}

// MigrateTo migrates to a specific version
func (m *Migrator) MigrateTo(targetVersion int) error {
	if err := m.EnsureSchemaVersionTable(); err != nil {
		return err
	}

	currentVersion, err := m.GetCurrentVersion()
	if err != nil {
		return err
	}

	// If targetVersion is -1, migrate to latest
	if targetVersion == -1 {
		if len(m.migrations) == 0 {
			targetVersion = 0
		} else {
			targetVersion = m.migrations[len(m.migrations)-1].Version
		}
	}

	slog.Info("Starting migration", "current_version", currentVersion, "target_version", targetVersion)

	if currentVersion == targetVersion {
		slog.Info("Database is already at target version", "version", targetVersion)
		return nil
	}

	// Sort migrations by version
	sort.Slice(m.migrations, func(i, j int) bool {
		return m.migrations[i].Version < m.migrations[j].Version
	})

	if currentVersion < targetVersion {
		// Migrate up
		return m.migrateUp(currentVersion, targetVersion)
	} else {
		// Migrate down
		return m.migrateDown(currentVersion, targetVersion)
	}
}

// migrateUp applies forward migrations
func (m *Migrator) migrateUp(currentVersion, targetVersion int) error {
	for _, migration := range m.migrations {
		if migration.Version <= currentVersion || migration.Version > targetVersion {
			continue
		}

		slog.Info("Applying migration", "version", migration.Version, "description", migration.Description)

		// Start transaction
		tx, err := m.db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction for migration %d: %w", migration.Version, err)
		}

		// Execute migration
		if _, err := tx.Exec(migration.UpSQL); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("failed to execute migration %d: %w", migration.Version, err)
		}

		// Record migration in schema_versions table
		checksum := calculateChecksum(migration.UpSQL)
		if _, err := tx.Exec(
			"INSERT INTO schema_versions (version, description, checksum) VALUES (?, ?, ?)",
			migration.Version, migration.Description, checksum,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("failed to record migration %d: %w", migration.Version, err)
		}

		// Commit transaction
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %d: %w", migration.Version, err)
		}

		slog.Info("Successfully applied migration", "version", migration.Version)
	}

	return nil
}

// migrateDown applies rollback migrations
func (m *Migrator) migrateDown(currentVersion, targetVersion int) error {
	// Sort migrations in reverse order for rollback
	sort.Slice(m.migrations, func(i, j int) bool {
		return m.migrations[i].Version > m.migrations[j].Version
	})

	for _, migration := range m.migrations {
		if migration.Version > currentVersion || migration.Version <= targetVersion {
			continue
		}

		if migration.DownSQL == "" {
			return fmt.Errorf("migration %d has no rollback SQL defined", migration.Version)
		}

		slog.Info("Rolling back migration", "version", migration.Version, "description", migration.Description)

		// Start transaction
		tx, err := m.db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction for rollback %d: %w", migration.Version, err)
		}

		// Execute rollback
		if _, err := tx.Exec(migration.DownSQL); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("failed to execute rollback %d: %w", migration.Version, err)
		}

		// Remove migration record from schema_versions table
		if _, err := tx.Exec("DELETE FROM schema_versions WHERE version = ?", migration.Version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("failed to remove migration record %d: %w", migration.Version, err)
		}

		// Commit transaction
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit rollback %d: %w", migration.Version, err)
		}

		slog.Info("Successfully rolled back migration", "version", migration.Version)
	}

	return nil
}

// Status shows the current migration status
func (m *Migrator) Status() ([]MigrationStatus, error) {
	if err := m.EnsureSchemaVersionTable(); err != nil {
		return nil, err
	}

	appliedVersions, err := m.GetAppliedMigrations()
	if err != nil {
		return nil, err
	}

	appliedMap := make(map[int]bool)
	for _, version := range appliedVersions {
		appliedMap[version] = true
	}

	var status []MigrationStatus
	for _, migration := range m.migrations {
		applied := appliedMap[migration.Version]
		status = append(status, MigrationStatus{
			Version:     migration.Version,
			Description: migration.Description,
			Applied:     applied,
		})
	}

	// Sort by version
	sort.Slice(status, func(i, j int) bool {
		return status[i].Version < status[j].Version
	})

	return status, nil
}

// MigrationStatus represents the status of a migration
type MigrationStatus struct {
	Version     int
	Description string
	Applied     bool
}

// calculateChecksum creates a simple checksum for migration verification
func calculateChecksum(sql string) string {
	// Simple hash of the SQL content
	hash := 0
	for _, char := range sql {
		hash = hash*31 + int(char)
	}
	return strconv.Itoa(hash)
}

// Validate checks that all applied migrations match their checksums
func (m *Migrator) Validate() error {
	rows, err := m.db.Query("SELECT version, checksum FROM schema_versions ORDER BY version")
	if err != nil {
		return fmt.Errorf("failed to query migration checksums: %w", err)
	}
	defer func() { _ = rows.Close() }()

	migrationMap := make(map[int]Migration)
	for _, migration := range m.migrations {
		migrationMap[migration.Version] = migration
	}

	for rows.Next() {
		var version int
		var storedChecksum string
		if err := rows.Scan(&version, &storedChecksum); err != nil {
			return fmt.Errorf("failed to scan migration checksum: %w", err)
		}

		migration, exists := migrationMap[version]
		if !exists {
			return fmt.Errorf("applied migration %d not found in current migration files", version)
		}

		expectedChecksum := calculateChecksum(migration.UpSQL)
		if storedChecksum != expectedChecksum {
			return fmt.Errorf("migration %d checksum mismatch: expected %s, got %s",
				version, expectedChecksum, storedChecksum)
		}
	}

	return nil
}

// Reset drops all tables and reapplies all migrations (DANGEROUS - use with caution)
func (m *Migrator) Reset() error {
	slog.Warn("Resetting database - this will drop all tables!")

	// Get current version first
	currentVersion, err := m.GetCurrentVersion()
	if err != nil {
		slog.Info("Could not get current version, assuming empty database")
		currentVersion = 0
	}

	// If we have migrations applied, roll them all back
	if currentVersion > 0 {
		if err := m.MigrateTo(0); err != nil {
			return fmt.Errorf("failed to rollback migrations during reset: %w", err)
		}
	}

	// Drop schema_versions table
	if _, err := m.db.Exec("DROP TABLE IF EXISTS schema_versions"); err != nil {
		return fmt.Errorf("failed to drop schema_versions table: %w", err)
	}

	// Reapply all migrations
	return m.Up()
}
