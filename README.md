# print-dis

A service for managing 3D printing requests.

## Configuration

The application can be configured through multiple sources, which are loaded in the following order:

1. Default values
2. Configuration file
3. Environment variables
4. Command-line flags

### Configuration File

The application looks for a configuration file named `config.yaml` in the following locations:

- Current working directory
- `$HOME/.print-dis/`
- `/etc/print-dis/`

Example configuration file:

```yaml
# Server configuration
server:
  host: "0.0.0.0" # Host to bind the server to
  port: "8080" # Port to bind the server to

# Database configuration
db:
  type: "sqlite" # Database type (sqlite or postgres)
  host: "localhost" # Database host (for PostgreSQL)
  port: 5432 # Database port (for PostgreSQL)
  user: "postgres" # Database user (for PostgreSQL)
  password: "" # Database password (for PostgreSQL)
  database: "print-dis.db" # Database path (for SQLite) or name (for PostgreSQL)
  ssl_mode: "disable" # Database SSL mode (for PostgreSQL)
```

### Environment Variables

All configuration options can be set using environment variables. The environment variables are prefixed with `PRINT_DIS_` and use underscores instead of dots. For example:

```bash
PRINT_DIS_SERVER_HOST=localhost
PRINT_DIS_SERVER_PORT=3000
PRINT_DIS_DB_TYPE=postgres
```

### Command-line Flags

The application supports the following command-line flags:

```bash
--host string        Host to bind the server to (default "0.0.0.0")
--port string        Port to bind the server to (default "8080")
--db-type string     Database type (sqlite or postgres) (default "sqlite")
--db-host string     Database host (for PostgreSQL) (default "localhost")
--db-port int        Database port (for PostgreSQL) (default 5432)
--db-user string     Database user (for PostgreSQL) (default "postgres")
--db-pass string     Database password (for PostgreSQL)
--db-path string     Database path (for SQLite) or name (for PostgreSQL) (default "print-dis.db")
--db-ssl-mode string Database SSL mode (for PostgreSQL) (default "disable")
```

## Building

```bash
make build
```

## Running

```bash
# Using default configuration
./bin/print-dis

# Using a configuration file
./bin/print-dis --config /path/to/config.yaml

# Using environment variables
PRINT_DIS_SERVER_PORT=3000 ./bin/print-dis

# Using command-line flags
./bin/print-dis --port 3000
```

## Note

For now, much of this has been vibe-coded. Once it's closer to a decent MVP, I intend to go back over it with a fine toothed comb.
