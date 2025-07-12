package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bjschafer/print-dis/internal/api"
	"github.com/bjschafer/print-dis/internal/config"
	"github.com/bjschafer/print-dis/internal/database"
	"github.com/bjschafer/print-dis/internal/handlers"
	"github.com/bjschafer/print-dis/internal/middleware"
	"github.com/bjschafer/print-dis/internal/router"
	"github.com/bjschafer/print-dis/internal/services"
	"github.com/bjschafer/print-dis/internal/spoolman"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Set up log level
	var level slog.Level
	switch cfg.Log.Level {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		fmt.Fprintf(os.Stderr, "Invalid log level: %s\n", cfg.Log.Level)
		os.Exit(1)
	}

	// Initialize logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)

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

	// Create database client
	db, err := database.NewDBClient(dbConfig)
	if err != nil {
		slog.Error("failed to create database client", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Create service layer
	printRequestService := services.NewPrintRequestService(db)
	userService := services.NewUserService(db)

	// Initialize Spoolman if enabled
	var spoolmanService *spoolman.Service
	if cfg.Spoolman.Enabled {
		spoolmanClient := spoolman.New(cfg.Spoolman.Endpoint)
		spoolmanService = spoolman.NewService(spoolmanClient)
		slog.Info("Spoolman integration enabled", "endpoint", cfg.Spoolman.Endpoint)
	} else {
		slog.Info("Spoolman integration disabled")
	}

	// Create session store for authentication
	sessionStore := middleware.NewSessionStore(cfg, db)

	// Create handlers
	printRequestHandler := handlers.NewPrintRequestHandler(printRequestService, spoolmanService)
	authHandler := handlers.NewAuthHandler(userService, sessionStore, cfg)
	adminHandler := handlers.NewAdminHandler(userService, cfg)
	var spoolmanHandler *api.SpoolmanHandler
	if spoolmanService != nil {
		spoolmanHandler = api.NewSpoolmanHandler(spoolmanService)
	}

	// Create a new server
	addr := cfg.Server.Host + ":" + cfg.Server.Port
	server := &http.Server{
		Addr: addr,
	}

	// Setup routes using the router package
	mux := http.NewServeMux()
	
	deps := &router.Dependencies{
		Config:              cfg,
		SessionStore:        sessionStore,
		PrintRequestHandler: printRequestHandler,
		AuthHandler:         authHandler,
		AdminHandler:        adminHandler,
		SpoolmanHandler:     spoolmanHandler,
	}
	
	router.SetupRoutes(mux, deps)

	// Set the server's handler with security middleware
	server.Handler = middleware.SecurityHeaders()(mux)

	// Create a channel to listen for errors coming from the server
	serverErrors := make(chan error, 1)

	// Start the server in a goroutine
	go func() {
		slog.Info("starting server", "addr", addr)
		serverErrors <- server.ListenAndServe()
	}()

	// Create a channel to listen for an interrupt or terminate signal from the OS
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Blocking select waiting for either a server error or a shutdown signal
	select {
	case err := <-serverErrors:
		slog.Error("server error", "error", err)
		os.Exit(1)
	case sig := <-shutdown:
		slog.Info("received signal", "signal", sig)
		slog.Info("shutting down server...")

		// Create a deadline for server shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Attempt graceful shutdown
		if err := server.Shutdown(ctx); err != nil {
			slog.Error("server shutdown error", "error", err)
			os.Exit(1)
		}
	}
}
