package config

import (
	"fmt"
	"os"
	"strings"

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
	Host string
	Port string
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
	HeaderName string // Name of the header containing the username
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
			HeaderName: v.GetString("auth.header_name"),
		},
	}

	return config, nil
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
	v.SetDefault("auth.header_name", "") // Empty string means auth is disabled
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
	flags.String("auth-header", v.GetString("auth.header_name"), "Name of the header containing the username")

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
	v.BindPFlag("auth.header_name", flags.Lookup("auth-header"))
}
