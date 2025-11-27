package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/bjschafer/print-dis/internal/config"
	"github.com/bjschafer/print-dis/internal/database"
	"github.com/bjschafer/print-dis/internal/migrations"
)

func main() {
	var (
		configPath = flag.String("config", "", "Path to configuration file")
		command    = flag.String("command", "up", "Migration command: up, down, status, reset, validate")
		version    = flag.Int("version", -1, "Target version for migration (default: latest)")
		verbose    = flag.Bool("verbose", false, "Verbose output")
	)
	flag.Parse()

	// Set up logging
	logLevel := slog.LevelInfo
	if *verbose {
		logLevel = slog.LevelDebug
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Override config file if provided
	if *configPath != "" {
		// TODO: Load from specific path
		slog.Info("Custom config path not implemented yet", "path", *configPath)
	}

	// Create database configuration
	dbConfig := &database.Config{
		Type:     cfg.DB.Type,
		Host:     cfg.DB.Host,
		Port:     cfg.DB.Port,
		User:     cfg.DB.User,
		Password: cfg.DB.Password,
		Database: cfg.DB.Database,
		SSLMode:  cfg.DB.SSLMode,
	}

	// Initialize database connection
	db, err := database.NewDBClient(dbConfig)
	if err != nil {
		slog.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	// Get raw database connection for migrations
	rawDB := db.GetDB()
	if rawDB == nil {
		slog.Error("Failed to get raw database connection")
		os.Exit(1)
	}

	// Create migrator
	migrator := migrations.NewMigrator(rawDB, cfg.DB.Type)

	// Execute command
	switch *command {
	case "up":
		if *version == -1 {
			slog.Info("Running all pending migrations...")
			err = migrator.Up()
		} else {
			slog.Info("Migrating to version", "version", *version)
			err = migrator.MigrateTo(*version)
		}

	case "down":
		if *version == -1 {
			slog.Error("Version is required for down migration")
			os.Exit(1)
		}
		slog.Info("Rolling back to version", "version", *version)
		err = migrator.MigrateTo(*version)

	case "status":
		status, err := migrator.Status()
		if err != nil {
			slog.Error("Failed to get migration status", "error", err)
			os.Exit(1)
		}

		fmt.Println("Migration Status:")
		fmt.Println("Version | Description                                    | Applied")
		fmt.Println("--------|------------------------------------------------|--------")
		for _, s := range status {
			applied := "No"
			if s.Applied {
				applied = "Yes"
			}
			fmt.Printf("%-7d | %-46s | %s\n", s.Version, s.Description, applied)
		}

		currentVersion, err := migrator.GetCurrentVersion()
		if err != nil {
			slog.Error("Failed to get current version", "error", err)
			os.Exit(1)
		}
		fmt.Printf("\nCurrent version: %d\n", currentVersion)
		return

	case "validate":
		slog.Info("Validating migrations...")
		err = migrator.Validate()
		if err == nil {
			slog.Info("All migrations are valid")
		}

	case "reset":
		slog.Warn("Resetting database - this will drop all tables!")
		fmt.Print("Are you sure you want to reset the database? (y/N): ")
		var confirm string
		_, _ = fmt.Scanln(&confirm)
		if confirm != "y" && confirm != "Y" {
			slog.Info("Reset cancelled")
			return
		}
		err = migrator.Reset()

	default:
		slog.Error("Unknown command", "command", *command)
		fmt.Println("Available commands: up, down, status, reset, validate")
		os.Exit(1)
	}

	if err != nil {
		slog.Error("Migration failed", "command", *command, "error", err)
		os.Exit(1)
	}

	slog.Info("Migration completed successfully", "command", *command)
}
