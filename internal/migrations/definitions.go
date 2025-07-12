package migrations

// getAllMigrations returns all defined migrations in order
func getAllMigrations(dbType string) []Migration {
	// Get database-specific SQL for migrations
	migration001Up, migration001Down := getMigration001SQL(dbType)
	migration002Up, migration002Down := getMigration002SQL(dbType)
	migration003Up, migration003Down := getMigration003SQL(dbType)
	migration004Up, migration004Down := GetMigration004SQL(dbType)
	
	return []Migration{
		{
			Version:     1,
			Description: "Create initial tables for users and print requests",
			UpSQL:       migration001Up,
			DownSQL:     migration001Down,
		},
		{
			Version:     2,
			Description: "Add role column to users table",
			UpSQL:       migration002Up,
			DownSQL:     migration002Down,
		},
		{
			Version:     3,
			Description: "Create materials and printers tables",
			UpSQL:       migration003Up,
			DownSQL:     migration003Down,
		},
		{
			Version:     4,
			Description: "Change spool_id column from TEXT to INTEGER",
			UpSQL:       migration004Up,
			DownSQL:     migration004Down,
		},
	}
}

// getMigration001SQL returns database-specific SQL for migration 001
func getMigration001SQL(dbType string) (string, string) {
	switch dbType {
	case "postgres":
		return migration001Up_Postgres, migration001Down
	default: // sqlite
		return migration001Up_SQLite, migration001Down
	}
}

// getMigration002SQL returns database-specific SQL for migration 002
func getMigration002SQL(dbType string) (string, string) {
	switch dbType {
	case "postgres":
		return migration002Up_Postgres, migration002Down_Postgres
	default: // sqlite
		return migration002Up_SQLite, migration002Down_SQLite
	}
}

// getMigration003SQL returns database-specific SQL for migration 003
func getMigration003SQL(dbType string) (string, string) {
	switch dbType {
	case "postgres":
		return migration003Up_Postgres, migration003Down
	default: // sqlite
		return migration003Up_SQLite, migration003Down
	}
}

// GetMigration004SQL returns the appropriate SQL for migration 004 based on database type
func GetMigration004SQL(dbType string) (string, string) {
	switch dbType {
	case "postgres":
		return migration004Up_Postgres, migration004Down_Postgres
	default: // sqlite
		return migration004Up_SQLite, migration004Down_SQLite
	}
}