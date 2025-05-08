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

	// Initialize Spoolman if enabled
	var spoolmanService *spoolman.Service
	if cfg.Spoolman.Enabled {
		spoolmanClient := spoolman.New(cfg.Spoolman.Endpoint)
		spoolmanService = spoolman.NewService(spoolmanClient)
		slog.Info("Spoolman integration enabled", "endpoint", cfg.Spoolman.Endpoint)
	} else {
		slog.Info("Spoolman integration disabled")
	}

	// Create handlers
	printRequestHandler := handlers.NewPrintRequestHandler(printRequestService)
	var spoolmanHandler *api.SpoolmanHandler
	if spoolmanService != nil {
		spoolmanHandler = api.NewSpoolmanHandler(spoolmanService)
	}

	// Create a new server
	addr := cfg.Server.Host + ":" + cfg.Server.Port
	server := &http.Server{
		Addr: addr,
	}

	// Serve static files
	fs := http.FileServer(http.Dir("static"))
	http.Handle("/", fs)

	// Set up API routes
	http.HandleFunc("/api/print-requests", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			printRequestHandler.CreatePrintRequest(w, r)
		case http.MethodGet:
			if r.URL.Query().Get("id") != "" {
				printRequestHandler.GetPrintRequest(w, r)
			} else {
				printRequestHandler.ListPrintRequests(w, r)
			}
		case http.MethodPut:
			printRequestHandler.UpdatePrintRequest(w, r)
		case http.MethodDelete:
			printRequestHandler.DeletePrintRequest(w, r)
		default:
			slog.Warn("invalid method for print requests endpoint",
				"method", r.Method,
				"path", r.URL.Path,
			)
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Add Spoolman routes if enabled
	if spoolmanHandler != nil {
		http.HandleFunc("/api/spoolman/spools", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				spoolmanHandler.GetSpools(w, r)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		})

		http.HandleFunc("/api/spoolman/spool", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				spoolmanHandler.GetSpool(w, r)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		})

		http.HandleFunc("/api/spoolman/materials", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				spoolmanHandler.GetMaterials(w, r)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		})
	}

	// Add route for status updates
	http.HandleFunc("/api/print-requests/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			printRequestHandler.UpdatePrintRequestStatus(w, r)
		} else {
			slog.Warn("invalid method for print request status endpoint",
				"method", r.Method,
				"path", r.URL.Path,
			)
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

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
