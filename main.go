package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bjschafer/print-dis/internal/database"
	"github.com/bjschafer/print-dis/internal/handlers"
	"github.com/bjschafer/print-dis/internal/services"
)

func main() {
	// Define command-line flags
	host := flag.String("host", getEnv("HOST", "0.0.0.0"), "Host to bind the server to")
	port := flag.String("port", getEnv("PORT", "8080"), "Port to bind the server to")
	dbType := flag.String("db-type", getEnv("DB_TYPE", "sqlite"), "Database type (sqlite or postgres)")
	dbPath := flag.String("db-path", getEnv("DB_PATH", "print-dis.db"), "Database path (for SQLite) or name (for PostgreSQL)")
	dbHost := flag.String("db-host", getEnv("DB_HOST", "localhost"), "Database host (for PostgreSQL)")
	dbPort := flag.Int("db-port", getEnvInt("DB_PORT", 5432), "Database port (for PostgreSQL)")
	dbUser := flag.String("db-user", getEnv("DB_USER", "postgres"), "Database user (for PostgreSQL)")
	dbPass := flag.String("db-pass", getEnv("DB_PASS", ""), "Database password (for PostgreSQL)")
	dbSSLMode := flag.String("db-ssl-mode", getEnv("DB_SSL_MODE", "disable"), "Database SSL mode (for PostgreSQL)")
	showHelp := flag.Bool("help", false, "Show help information")
	showHelpShort := flag.Bool("h", false, "Show help information (shorthand)")

	// Custom usage message
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Options:")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "\nEnvironment Variables:")
		fmt.Fprintln(os.Stderr, "  HOST        Host to bind the server to (default: 0.0.0.0)")
		fmt.Fprintln(os.Stderr, "  PORT        Port to bind the server to (default: 8080)")
		fmt.Fprintln(os.Stderr, "  DB_TYPE     Database type (sqlite or postgres) (default: sqlite)")
		fmt.Fprintln(os.Stderr, "  DB_PATH     Database path (for SQLite) or name (for PostgreSQL) (default: print-dis.db)")
		fmt.Fprintln(os.Stderr, "  DB_HOST     Database host (for PostgreSQL) (default: localhost)")
		fmt.Fprintln(os.Stderr, "  DB_PORT     Database port (for PostgreSQL) (default: 5432)")
		fmt.Fprintln(os.Stderr, "  DB_USER     Database user (for PostgreSQL) (default: postgres)")
		fmt.Fprintln(os.Stderr, "  DB_PASS     Database password (for PostgreSQL)")
		fmt.Fprintln(os.Stderr, "  DB_SSL_MODE Database SSL mode (for PostgreSQL) (default: disable)")
		fmt.Fprintln(os.Stderr, "\nExample:")
		fmt.Fprintln(os.Stderr, "  ./print-dis --port 3000")
		fmt.Fprintln(os.Stderr, "  HOST=localhost PORT=3000 ./print-dis")
	}

	// Parse flags
	flag.Parse()

	// Show help if requested
	if *showHelp || *showHelpShort {
		flag.Usage()
		os.Exit(0)
	}

	// Create database configuration
	dbConfig := &database.Config{
		Type:     *dbType,
		Host:     *dbHost,
		Port:     *dbPort,
		User:     *dbUser,
		Password: *dbPass,
		Database: *dbPath,
		SSLMode:  *dbSSLMode,
	}

	// Create database client
	db, err := database.NewDBClient(dbConfig)
	if err != nil {
		log.Fatalf("Failed to create database client: %v", err)
	}
	defer db.Close()

	// Create service layer
	printRequestService := services.NewPrintRequestService(db)

	// Create handlers
	printRequestHandler := handlers.NewPrintRequestHandler(printRequestService)

	// Create a new server
	addr := *host + ":" + *port
	server := &http.Server{
		Addr: addr,
	}

	// Set up routes
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
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Create a channel to listen for errors coming from the server
	serverErrors := make(chan error, 1)

	// Start the server in a goroutine
	go func() {
		log.Printf("Starting server on %s", addr)
		serverErrors <- server.ListenAndServe()
	}()

	// Create a channel to listen for an interrupt or terminate signal from the OS
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Blocking select waiting for either a server error or a shutdown signal
	select {
	case err := <-serverErrors:
		log.Fatalf("Server error: %v", err)
	case sig := <-shutdown:
		log.Printf("Received signal: %v", sig)
		log.Println("Shutting down server...")

		// Create a deadline for server shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Attempt graceful shutdown
		if err := server.Shutdown(ctx); err != nil {
			log.Fatalf("Server shutdown error: %v", err)
		}
	}
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// getEnvInt gets an environment variable as an integer or returns a default value
func getEnvInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intValue, err := fmt.Sscanf(value, "%d", &defaultValue); err == nil {
			return intValue
		}
	}
	return defaultValue
}
