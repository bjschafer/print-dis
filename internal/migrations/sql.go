package migrations

// Migration 001: Initial schema - SQLite version
const migration001Up_SQLite = `
-- Users table
CREATE TABLE IF NOT EXISTS users (
	id TEXT PRIMARY KEY,
	username TEXT UNIQUE NOT NULL,
	email TEXT UNIQUE,
	password_hash TEXT,
	display_name TEXT,
	enabled BOOLEAN DEFAULT TRUE,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Print requests table
CREATE TABLE IF NOT EXISTS print_requests (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL REFERENCES users(id),
	submitter TEXT NOT NULL,
	description TEXT,
	file_link TEXT,
	status TEXT NOT NULL DEFAULT 'StatusPendingApproval',
	material TEXT,
	color TEXT,
	spool_id TEXT,
	comments TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- User OIDC identities table
CREATE TABLE IF NOT EXISTS user_oidc_identities (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL,
	provider_name TEXT NOT NULL,
	subject TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	UNIQUE(provider_name, subject)
);

-- User sessions table
CREATE TABLE IF NOT EXISTS user_sessions (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL,
	session_token TEXT UNIQUE NOT NULL,
	expires_at DATETIME NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Indexes for better performance
CREATE INDEX IF NOT EXISTS idx_print_requests_user_id ON print_requests(user_id);
CREATE INDEX IF NOT EXISTS idx_print_requests_status ON print_requests(status);
CREATE INDEX IF NOT EXISTS idx_print_requests_created_at ON print_requests(created_at);
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
`

// Migration 001: Initial schema - PostgreSQL version
const migration001Up_Postgres = `
-- Users table
CREATE TABLE IF NOT EXISTS users (
	id TEXT PRIMARY KEY,
	username TEXT UNIQUE NOT NULL,
	email TEXT UNIQUE,
	password_hash TEXT,
	display_name TEXT,
	enabled BOOLEAN DEFAULT TRUE,
	created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
	updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Print requests table
CREATE TABLE IF NOT EXISTS print_requests (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL REFERENCES users(id),
	submitter TEXT NOT NULL,
	description TEXT,
	file_link TEXT,
	status TEXT NOT NULL DEFAULT 'StatusPendingApproval',
	material TEXT,
	color TEXT,
	spool_id TEXT,
	comments TEXT,
	created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
	updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- User OIDC identities table
CREATE TABLE IF NOT EXISTS user_oidc_identities (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	provider_name TEXT NOT NULL,
	subject TEXT NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
	UNIQUE(provider_name, subject)
);

-- User sessions table
CREATE TABLE IF NOT EXISTS user_sessions (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	session_token TEXT UNIQUE NOT NULL,
	expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for better performance
CREATE INDEX IF NOT EXISTS idx_print_requests_user_id ON print_requests(user_id);
CREATE INDEX IF NOT EXISTS idx_print_requests_status ON print_requests(status);
CREATE INDEX IF NOT EXISTS idx_print_requests_created_at ON print_requests(created_at);
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
`

const migration001Down = `
DROP INDEX IF EXISTS idx_users_email;
DROP INDEX IF EXISTS idx_users_username;
DROP INDEX IF EXISTS idx_print_requests_created_at;
DROP INDEX IF EXISTS idx_print_requests_status;
DROP INDEX IF EXISTS idx_print_requests_user_id;
DROP TABLE IF EXISTS user_sessions;
DROP TABLE IF EXISTS user_oidc_identities;
DROP TABLE IF EXISTS print_requests;
DROP TABLE IF EXISTS users;
`

// Migration 002: Add role column - SQLite version
const migration002Up_SQLite = `
ALTER TABLE users ADD COLUMN role TEXT DEFAULT 'user';
CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);
`

// Migration 002: Add role column - PostgreSQL version
const migration002Up_Postgres = `
ALTER TABLE users ADD COLUMN role TEXT DEFAULT 'user';
CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);
`

const migration002Down_SQLite = `
DROP INDEX IF EXISTS idx_users_role;
-- Note: SQLite doesn't support DROP COLUMN, so we'd need to recreate the table
-- For simplicity, we'll leave the column but remove the index
`

const migration002Down_Postgres = `
DROP INDEX IF EXISTS idx_users_role;
ALTER TABLE users DROP COLUMN role;
`

// Migration 003: Create materials and printers tables - SQLite version
const migration003Up_SQLite = `
-- Materials table for 3D printing materials
CREATE TABLE IF NOT EXISTS materials (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT UNIQUE NOT NULL,
	density REAL,
	diameter REAL DEFAULT 1.75,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Printers table
CREATE TABLE IF NOT EXISTS printers (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	dim_x INTEGER NOT NULL,
	dim_y INTEGER NOT NULL,
	dim_z INTEGER NOT NULL,
	url TEXT NOT NULL
);

-- Filaments table
CREATE TABLE IF NOT EXISTS filaments (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	material_id INTEGER NOT NULL,
	FOREIGN KEY (material_id) REFERENCES materials(id)
);

-- Jobs table
CREATE TABLE IF NOT EXISTS jobs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	printer_id INTEGER NOT NULL,
	filament_id INTEGER NOT NULL,
	material_id INTEGER NOT NULL,
	FOREIGN KEY (printer_id) REFERENCES printers(id),
	FOREIGN KEY (filament_id) REFERENCES filaments(id),
	FOREIGN KEY (material_id) REFERENCES materials(id)
);

-- Insert some default materials
INSERT OR IGNORE INTO materials (name, density, diameter) VALUES
	('PLA', 1.24, 1.75),
	('PETG', 1.27, 1.75),
	('ABS', 1.04, 1.75),
	('TPU', 1.20, 1.75);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_materials_name ON materials(name);
CREATE INDEX IF NOT EXISTS idx_printers_name ON printers(name);
`

// Migration 003: Create materials and printers tables - PostgreSQL version
const migration003Up_Postgres = `
-- Materials table for 3D printing materials
CREATE TABLE IF NOT EXISTS materials (
	id SERIAL PRIMARY KEY,
	name TEXT UNIQUE NOT NULL,
	density REAL,
	diameter REAL DEFAULT 1.75,
	created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Printers table
CREATE TABLE IF NOT EXISTS printers (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	dim_x INTEGER NOT NULL,
	dim_y INTEGER NOT NULL,
	dim_z INTEGER NOT NULL,
	url TEXT NOT NULL
);

-- Filaments table
CREATE TABLE IF NOT EXISTS filaments (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	material_id INTEGER NOT NULL,
	FOREIGN KEY (material_id) REFERENCES materials(id)
);

-- Jobs table
CREATE TABLE IF NOT EXISTS jobs (
	id SERIAL PRIMARY KEY,
	printer_id INTEGER NOT NULL,
	filament_id INTEGER NOT NULL,
	material_id INTEGER NOT NULL,
	FOREIGN KEY (printer_id) REFERENCES printers(id),
	FOREIGN KEY (filament_id) REFERENCES filaments(id),
	FOREIGN KEY (material_id) REFERENCES materials(id)
);

-- Insert some default materials
INSERT INTO materials (name, density, diameter) VALUES
	('PLA', 1.24, 1.75),
	('PETG', 1.27, 1.75),
	('ABS', 1.04, 1.75),
	('TPU', 1.20, 1.75)
ON CONFLICT (name) DO NOTHING;

-- Indexes
CREATE INDEX IF NOT EXISTS idx_materials_name ON materials(name);
CREATE INDEX IF NOT EXISTS idx_printers_name ON printers(name);
`

const migration003Down = `
DROP INDEX IF EXISTS idx_printers_name;
DROP INDEX IF EXISTS idx_materials_name;
DROP TABLE IF EXISTS jobs;
DROP TABLE IF EXISTS filaments;
DROP TABLE IF EXISTS printers;
DROP TABLE IF EXISTS materials;
`

// Migration 004: Change spool_id from TEXT to INTEGER - SQLite version
const migration004Up_SQLite = `
-- SQLite version: Recreate table with INTEGER spool_id
-- Create new table with correct schema
CREATE TABLE print_requests_new (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL REFERENCES users(id),
	submitter TEXT NOT NULL,
	description TEXT,
	file_link TEXT,
	status TEXT NOT NULL DEFAULT 'StatusPendingApproval',
	material TEXT,
	color TEXT,
	spool_id INTEGER,
	comments TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Copy data, converting spool_id from TEXT to INTEGER
INSERT INTO print_requests_new 
SELECT 
	id, user_id, submitter, description, file_link, status, material, color,
	CASE 
		WHEN spool_id IS NULL OR spool_id = '' THEN NULL
		WHEN spool_id GLOB '[0-9]*' THEN CAST(spool_id AS INTEGER)
		ELSE NULL
	END as spool_id,
	comments, created_at, updated_at
FROM print_requests;

-- Drop old table and rename new one
DROP TABLE print_requests;
ALTER TABLE print_requests_new RENAME TO print_requests;

-- Recreate indexes
CREATE INDEX IF NOT EXISTS idx_print_requests_user_id ON print_requests(user_id);
CREATE INDEX IF NOT EXISTS idx_print_requests_status ON print_requests(status);
CREATE INDEX IF NOT EXISTS idx_print_requests_created_at ON print_requests(created_at);
CREATE INDEX IF NOT EXISTS idx_print_requests_spool_id ON print_requests(spool_id);
`

// Migration 004: Change spool_id from TEXT to INTEGER - PostgreSQL version
const migration004Up_Postgres = `
-- PostgreSQL version: Use ALTER COLUMN with USING clause
ALTER TABLE print_requests 
ALTER COLUMN spool_id TYPE INTEGER 
USING CASE 
	WHEN spool_id ~ '^[0-9]+$' THEN spool_id::INTEGER
	ELSE NULL
END;

-- Add index for spool_id
CREATE INDEX IF NOT EXISTS idx_print_requests_spool_id ON print_requests(spool_id);
`

// Migration 004 rollback - SQLite version
const migration004Down_SQLite = `
-- SQLite version: Recreate table with TEXT spool_id
CREATE TABLE print_requests_new (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL REFERENCES users(id),
	submitter TEXT NOT NULL,
	description TEXT,
	file_link TEXT,
	status TEXT NOT NULL DEFAULT 'StatusPendingApproval',
	material TEXT,
	color TEXT,
	spool_id TEXT,
	comments TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Copy data, converting spool_id from INTEGER to TEXT
INSERT INTO print_requests_new 
SELECT 
	id, user_id, submitter, description, file_link, status, material, color,
	CASE 
		WHEN spool_id IS NULL THEN NULL
		ELSE CAST(spool_id AS TEXT)
	END as spool_id,
	comments, created_at, updated_at
FROM print_requests;

-- Drop old table and rename new one
DROP TABLE print_requests;
ALTER TABLE print_requests_new RENAME TO print_requests;

-- Recreate indexes
CREATE INDEX IF NOT EXISTS idx_print_requests_user_id ON print_requests(user_id);
CREATE INDEX IF NOT EXISTS idx_print_requests_status ON print_requests(status);
CREATE INDEX IF NOT EXISTS idx_print_requests_created_at ON print_requests(created_at);
`

// Migration 004 rollback - PostgreSQL version
const migration004Down_Postgres = `
-- PostgreSQL version: Change back to TEXT
ALTER TABLE print_requests 
ALTER COLUMN spool_id TYPE TEXT 
USING CASE 
	WHEN spool_id IS NULL THEN NULL
	ELSE spool_id::TEXT
END;

-- Drop the spool_id index
DROP INDEX IF EXISTS idx_print_requests_spool_id;
`