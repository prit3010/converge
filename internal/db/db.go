package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps sqlite access for converge metadata and manifests.
type DB struct {
	sql *sql.DB
}

func Open(path string) (*DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)", path)
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := migrate(sqlDB); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &DB{sql: sqlDB}, nil
}

func (d *DB) Close() error {
	if d == nil || d.sql == nil {
		return nil
	}
	return d.sql.Close()
}

func (d *DB) Ping() error {
	return d.sql.Ping()
}

func migrate(sqlDB *sql.DB) error {
	tx, err := sqlDB.Begin()
	if err != nil {
		return fmt.Errorf("begin migration tx: %w", err)
	}
	defer tx.Rollback()

	const schema = `
CREATE TABLE IF NOT EXISTS cells (
	id TEXT PRIMARY KEY,
	sequence INTEGER UNIQUE NOT NULL,
	parent_id TEXT,
	timestamp TEXT NOT NULL,
	message TEXT NOT NULL DEFAULT '',
	source TEXT NOT NULL DEFAULT 'manual',
	agent TEXT,
	tags TEXT,
	branch TEXT NOT NULL DEFAULT 'main',
	files_added INTEGER NOT NULL DEFAULT 0,
	files_modified INTEGER NOT NULL DEFAULT 0,
	files_removed INTEGER NOT NULL DEFAULT 0,
	lines_added INTEGER NOT NULL DEFAULT 0,
	lines_removed INTEGER NOT NULL DEFAULT 0,
	total_loc INTEGER NOT NULL DEFAULT 0,
	loc_delta INTEGER NOT NULL DEFAULT 0,
	total_files INTEGER NOT NULL DEFAULT 0,
	eval_requested INTEGER NOT NULL DEFAULT 0,
	eval_ran INTEGER NOT NULL DEFAULT 0,
	tests_passed INTEGER,
	tests_failed INTEGER,
	lint_errors INTEGER,
	type_errors INTEGER,
	eval_skipped TEXT,
	eval_error TEXT,
	FOREIGN KEY(parent_id) REFERENCES cells(id)
);

CREATE TABLE IF NOT EXISTS manifest_entries (
	cell_id TEXT NOT NULL,
	path TEXT NOT NULL,
	hash TEXT NOT NULL,
	mode INTEGER NOT NULL,
	size INTEGER NOT NULL,
	PRIMARY KEY (cell_id, path),
	FOREIGN KEY(cell_id) REFERENCES cells(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS branches (
	name TEXT PRIMARY KEY,
	head_cell_id TEXT REFERENCES cells(id),
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS meta (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_cells_sequence ON cells(sequence DESC);
CREATE INDEX IF NOT EXISTS idx_manifest_cell ON manifest_entries(cell_id);

CREATE TABLE IF NOT EXISTS cell_sequences (
	name TEXT PRIMARY KEY,
	last_sequence INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS agent_runs (
	run_id TEXT PRIMARY KEY,
	agent TEXT NOT NULL,
	message TEXT NOT NULL DEFAULT '',
	tags TEXT,
	source TEXT NOT NULL DEFAULT 'agent_complete',
	status TEXT NOT NULL,
	branch TEXT,
	cell_id TEXT,
	error TEXT,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_agent_runs_updated_at ON agent_runs(updated_at DESC);
`
	if _, err := tx.Exec(schema); err != nil {
		return fmt.Errorf("create base schema: %w", err)
	}

	if _, err := tx.Exec(`ALTER TABLE cells ADD COLUMN branch TEXT NOT NULL DEFAULT 'main'`); err != nil && !isDuplicateColumnError(err) {
		return fmt.Errorf("add branch column: %w", err)
	}

	if _, err := tx.Exec(`UPDATE cells SET branch = 'main' WHERE branch IS NULL OR TRIM(branch) = ''`); err != nil {
		return fmt.Errorf("backfill cell branch: %w", err)
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_cells_branch_sequence ON cells(branch, sequence DESC)`); err != nil {
		return fmt.Errorf("create branch index: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	var latestID *string
	err = tx.QueryRow(`SELECT id FROM cells ORDER BY sequence DESC LIMIT 1`).Scan(&latestID)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("read latest cell during migration: %w", err)
	}
	if err == sql.ErrNoRows {
		latestID = nil
	}

	if _, err := tx.Exec(`
INSERT INTO branches (name, head_cell_id, created_at) VALUES ('main', ?, ?)
ON CONFLICT(name) DO NOTHING
`, latestID, now); err != nil {
		return fmt.Errorf("seed main branch: %w", err)
	}

	if latestID != nil {
		if _, err := tx.Exec(`UPDATE branches SET head_cell_id = COALESCE(head_cell_id, ?) WHERE name = 'main'`, *latestID); err != nil {
			return fmt.Errorf("backfill main branch head: %w", err)
		}
	}

	if _, err := tx.Exec(`INSERT OR IGNORE INTO meta (key, value) VALUES ('active_branch', 'main')`); err != nil {
		return fmt.Errorf("seed active_branch meta: %w", err)
	}

	if latestID != nil {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO meta (key, value) VALUES ('head_cell', ?)`, *latestID); err != nil {
			return fmt.Errorf("seed head_cell meta: %w", err)
		}
	} else {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO meta (key, value) VALUES ('head_cell', '')`); err != nil {
			return fmt.Errorf("seed empty head_cell meta: %w", err)
		}
	}

	var maxSequence int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(sequence), 0) FROM cells`).Scan(&maxSequence); err != nil {
		return fmt.Errorf("read max sequence for allocator: %w", err)
	}
	if _, err := tx.Exec(`
INSERT INTO cell_sequences (name, last_sequence)
VALUES ('default', ?)
ON CONFLICT(name) DO UPDATE SET
	last_sequence = CASE
		WHEN cell_sequences.last_sequence < excluded.last_sequence THEN excluded.last_sequence
		ELSE cell_sequences.last_sequence
	END
`, maxSequence); err != nil {
		return fmt.Errorf("seed sequence allocator: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration tx: %w", err)
	}
	return nil
}

var ErrNotFound = errors.New("not found")

func isDuplicateColumnError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "duplicate column name")
}
