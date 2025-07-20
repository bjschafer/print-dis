package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Config holds all configuration for the application
type Config struct {
	Server   ServerConfig
	DB       DBConfig
	Log      LogConfig
	Spoolman SpoolmanConfig
	Auth     AuthConfig
}

// ServerConfig holds server-related configuration
type ServerConfig struct {
	Host  string
	Port  string
	HTTPS *bool // Optional explicit HTTPS setting
}

// DBConfig holds database-related configuration
type DBConfig struct {
	Type     string
	Host     string
	Port     int
	User     string
	Password string
	Database string
	SSLMode  string
}

// LogConfig holds logging-related configuration
type LogConfig struct {
	Level string // "debug", "info", "warn", "error"
}

// SpoolmanConfig holds Spoolman-related configuration
type SpoolmanConfig struct {
	Enabled  bool
	Endpoint string
}

// AuthConfig holds authentication-related configuration
type AuthConfig struct {
	Enabled        bool            `json:"enabled"`
	SessionSecret  string          `json:"session_secret"`
	SessionTimeout time.Duration   `json:"session_timeout"`
	LocalAuth      LocalAuthConfig `json:"local_auth"`
	OIDC           OIDCConfig      `json:"oidc"`
}

// LocalAuthConfig holds local authentication configuration
type LocalAuthConfig struct {
	Enabled           bool `json:"enabled"`
	AllowRegistration bool `json:"allow_registration"`
}

// OIDCConfig holds OIDC authentication configuration
type OIDCConfig struct {
	Providers []OIDCProviderConfig `json:"providers"`
}

// OIDCProviderConfig holds configuration for a single OIDC provider
type OIDCProviderConfig struct {
	Name         string `json:"name"`
	IssuerURL    string `json:"issuer_url"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Enabled      bool   `json:"enabled"`
}

// Load loads configuration from multiple sources in the following order:
// 1. Default values
// 2. Config file (if specified)
// 3. Environment variables
// 4. Command-line flags
func Load() (*Config, error) {
	v := viper.New()

	// Set default values
	setDefaults(v)

	// Set up environment variable handling
	v.SetEnvPrefix("PRINT_DIS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Set up config file handling
	v.SetConfigName("config")           // name of config file (without extension)
	v.SetConfigType("yaml")             // REQUIRED if the config file does not have the extension in the name
	v.AddConfigPath(".")                // look for config in the working directory
	v.AddConfigPath("$HOME/.print-dis") // call multiple times to add many search paths
	v.AddConfigPath("/etc/print-dis/")  // path to look for the config file in

	// Read config file if it exists
	if err := v.ReadInConfig(); err != nil {
		// It's okay if the config file doesn't exist
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Set up command-line flags
	setupFlags(v)

	// Parse session timeout
	sessionTimeout, err := time.ParseDuration(v.GetString("auth.session_timeout"))
	if err != nil {
		return nil, fmt.Errorf("invalid session timeout: %w", err)
	}

	// Handle session secret with security checks
	sessionSecret, err := getSessionSecret(v)
	if err != nil {
		return nil, fmt.Errorf("failed to get session secret: %w", err)
	}

	// Create config struct
	config := &Config{
		Server: ServerConfig{
			Host: v.GetString("server.host"),
			Port: v.GetString("server.port"),
		},
		DB: DBConfig{
			Type:     v.GetString("db.type"),
			Host:     v.GetString("db.host"),
			Port:     v.GetInt("db.port"),
			User:     v.GetString("db.user"),
			Password: v.GetString("db.password"),
			Database: v.GetString("db.database"),
			SSLMode:  v.GetString("db.ssl_mode"),
		},
		Log: LogConfig{
			Level: v.GetString("log.level"),
		},
		Spoolman: SpoolmanConfig{
			Enabled:  v.GetBool("spoolman.enabled"),
			Endpoint: v.GetString("spoolman.endpoint"),
		},
		Auth: AuthConfig{
			Enabled:        v.GetBool("auth.enabled"),
			SessionSecret:  sessionSecret,
			SessionTimeout: sessionTimeout,
			LocalAuth: LocalAuthConfig{
				Enabled:           v.GetBool("auth.local_auth.enabled"),
				AllowRegistration: v.GetBool("auth.local_auth.allow_registration"),
			},
			OIDC: OIDCConfig{
				Providers: parseOIDCProviders(v),
			},
		},
	}

	return config, nil
}

// parseOIDCProviders parses OIDC provider configurations from viper
func parseOIDCProviders(v *viper.Viper) []OIDCProviderConfig {
	// For now, we'll support a simple configuration structure
	// This can be extended later to support dynamic provider lists from config
	var providers []OIDCProviderConfig

	// Check if there's a Google provider configured
	if v.GetString("auth.oidc.google.client_id") != "" {
		providers = append(providers, OIDCProviderConfig{
			Name:         "google",
			IssuerURL:    "https://accounts.google.com",
			ClientID:     v.GetString("auth.oidc.google.client_id"),
			ClientSecret: v.GetString("auth.oidc.google.client_secret"),
			Enabled:      v.GetBool("auth.oidc.google.enabled"),
		})
	}

	// Check if there's a Microsoft provider configured
	if v.GetString("auth.oidc.microsoft.client_id") != "" {
		providers = append(providers, OIDCProviderConfig{
			Name:         "microsoft",
			IssuerURL:    "https://login.microsoftonline.com/common/v2.0",
			ClientID:     v.GetString("auth.oidc.microsoft.client_id"),
			ClientSecret: v.GetString("auth.oidc.microsoft.client_secret"),
			Enabled:      v.GetBool("auth.oidc.microsoft.enabled"),
		})
	}

	return providers
}

// setDefaults sets default values for configuration
func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", "8080")

	// Database defaults
	v.SetDefault("db.type", "sqlite")
	v.SetDefault("db.host", "localhost")
	v.SetDefault("db.port", 5432)
	v.SetDefault("db.user", "postgres")
	v.SetDefault("db.database", "print-dis.db")
	v.SetDefault("db.ssl_mode", "disable")

	// Log defaults
	v.SetDefault("log.level", "info")

	// Spoolman defaults
	v.SetDefault("spoolman.enabled", false)
	v.SetDefault("spoolman.endpoint", "http://localhost:8000")

	// Auth defaults
	v.SetDefault("auth.enabled", true)
	v.SetDefault("auth.session_secret", "") // Empty by default, will be auto-generated if needed
	v.SetDefault("auth.session_timeout", "24h")
	v.SetDefault("auth.local_auth.enabled", true)
	v.SetDefault("auth.local_auth.allow_registration", false)
	v.SetDefault("auth.oidc.google.enabled", false)
	v.SetDefault("auth.oidc.microsoft.enabled", false)
}

// setupFlags sets up command-line flags
func setupFlags(v *viper.Viper) {
	// Create a new flag set
	flags := pflag.NewFlagSet("print-dis", pflag.ExitOnError)

	// Server flags
	flags.String("host", v.GetString("server.host"), "Host to bind the server to")
	flags.String("port", v.GetString("server.port"), "Port to bind the server to")

	// Database flags
	flags.String("db-type", v.GetString("db.type"), "Database type (sqlite or postgres)")
	flags.String("db-host", v.GetString("db.host"), "Database host (for PostgreSQL)")
	flags.Int("db-port", v.GetInt("db.port"), "Database port (for PostgreSQL)")
	flags.String("db-user", v.GetString("db.user"), "Database user (for PostgreSQL)")
	flags.String("db-pass", v.GetString("db.password"), "Database password (for PostgreSQL)")
	flags.String("db-path", v.GetString("db.database"), "Database path (for SQLite) or name (for PostgreSQL)")
	flags.String("db-ssl-mode", v.GetString("db.ssl_mode"), "Database SSL mode (for PostgreSQL)")

	// Log flags
	flags.String("log-level", v.GetString("log.level"), "Log level (debug, info, warn, error)")

	// Spoolman flags
	flags.Bool("spoolman-enabled", v.GetBool("spoolman.enabled"), "Enable Spoolman integration")
	flags.String("spoolman-endpoint", v.GetString("spoolman.endpoint"), "Spoolman API endpoint")

	// Auth flags
	flags.Bool("auth-enabled", v.GetBool("auth.enabled"), "Enable authentication")
	flags.String("auth-session-secret", v.GetString("auth.session_secret"), "Session secret key")
	flags.String("auth-session-timeout", v.GetString("auth.session_timeout"), "Session timeout duration")
	flags.Bool("auth-local-enabled", v.GetBool("auth.local_auth.enabled"), "Enable local authentication")
	flags.Bool("auth-local-registration", v.GetBool("auth.local_auth.allow_registration"), "Allow user registration")

	// Parse flags
	flags.Parse(os.Args[1:])

	// Bind flags to viper
	v.BindPFlag("server.host", flags.Lookup("host"))
	v.BindPFlag("server.port", flags.Lookup("port"))
	v.BindPFlag("db.type", flags.Lookup("db-type"))
	v.BindPFlag("db.host", flags.Lookup("db-host"))
	v.BindPFlag("db.port", flags.Lookup("db-port"))
	v.BindPFlag("db.user", flags.Lookup("db-user"))
	v.BindPFlag("db.password", flags.Lookup("db-pass"))
	v.BindPFlag("db.database", flags.Lookup("db-path"))
	v.BindPFlag("db.ssl_mode", flags.Lookup("db-ssl-mode"))
	v.BindPFlag("log.level", flags.Lookup("log-level"))
	v.BindPFlag("spoolman.enabled", flags.Lookup("spoolman-enabled"))
	v.BindPFlag("spoolman.endpoint", flags.Lookup("spoolman-endpoint"))
	v.BindPFlag("auth.enabled", flags.Lookup("auth-enabled"))
	v.BindPFlag("auth.session_secret", flags.Lookup("auth-session-secret"))
	v.BindPFlag("auth.session_timeout", flags.Lookup("auth-session-timeout"))
	v.BindPFlag("auth.local_auth.enabled", flags.Lookup("auth-local-enabled"))
	v.BindPFlag("auth.local_auth.allow_registration", flags.Lookup("auth-local-registration"))
}

// getSessionSecret handles session secret retrieval with security checks and auto-generation
func getSessionSecret(v *viper.Viper) (string, error) {
	secret := v.GetString("auth.session_secret")
	
	// Check for insecure default
	if secret == "change-me-in-production" {
		slog.Error("Using insecure default session secret. Please set PRINT_DIS_AUTH_SESSION_SECRET environment variable or auth.session_secret in config file.")
		return "", fmt.Errorf("insecure default session secret detected")
	}
	
	// If no secret provided, auto-generate one with warning
	if secret == "" {
		slog.Warn("No session secret provided. Auto-generating one. For production, please set PRINT_DIS_AUTH_SESSION_SECRET environment variable or auth.session_secret in config file.")
		generatedSecret, err := generateSessionSecret()
		if err != nil {
			return "", fmt.Errorf("failed to generate session secret: %w", err)
		}
		slog.Info("Generated session secret. Sessions will not persist across application restarts.")
		return generatedSecret, nil
	}
	
	// Validate secret length (minimum 32 bytes when base64 decoded)
	if len(secret) < 32 {
		slog.Warn("Session secret is shorter than recommended 32 characters. Consider using a longer, randomly generated secret.")
	}
	
	return secret, nil
}

// generateSessionSecret creates a cryptographically secure random session secret
func generateSessionSecret() (string, error) {
	// Generate 32 random bytes (256 bits)
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	
	// Encode as base64 for easy handling
	return base64.URLEncoding.EncodeToString(bytes), nil
}
