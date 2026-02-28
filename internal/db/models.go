package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const defaultBranchName = "main"

type Cell struct {
	ID            string
	Sequence      int
	ParentID      *string
	Timestamp     string
	Message       string
	Source        string
	Agent         *string
	Tags          *string
	Branch        string
	FilesAdded    int
	FilesModified int
	FilesRemoved  int
	LinesAdded    int
	LinesRemoved  int
	TotalLOC      int
	LOCDelta      int
	TotalFiles    int
	EvalRequested bool
	EvalRan       bool
	TestsPassed   *int
	TestsFailed   *int
	LintErrors    *int
	TypeErrors    *int
	EvalSkipped   *string
	EvalError     *string
}

type Branch struct {
	Name       string
	HeadCellID *string
	CreatedAt  string
}

type ManifestEntry struct {
	CellID string
	Path   string
	Hash   string
	Mode   int
	Size   int64
}

type AgentRun struct {
	RunID     string
	Agent     string
	Message   string
	Tags      *string
	Source    string
	Status    string
	Branch    *string
	CellID    *string
	Error     *string
	CreatedAt string
	UpdatedAt string
}

func (d *DB) AllocateSequence() (int, error) {
	tx, err := d.sql.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin allocate sequence tx: %w", err)
	}
	defer tx.Rollback()

	var seq int
	if err := tx.QueryRow(`UPDATE cell_sequences SET last_sequence = last_sequence + 1 WHERE name = 'default' RETURNING last_sequence`).Scan(&seq); err != nil {
		return 0, fmt.Errorf("allocate sequence: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit allocate sequence tx: %w", err)
	}
	return seq, nil
}

func (d *DB) NextSequence() (int, error) {
	return d.AllocateSequence()
}

func (d *DB) InsertCellWithManifest(cell Cell, entries []ManifestEntry) error {
	return d.InsertCellWithManifestAndAdvanceBranch(cell, entries)
}

func (d *DB) InsertCellWithManifestAndAdvanceBranch(cell Cell, entries []ManifestEntry) error {
	branch := strings.TrimSpace(cell.Branch)
	if branch == "" {
		branch = defaultBranchName
	}
	cell.Branch = branch
	if strings.TrimSpace(cell.Timestamp) == "" {
		cell.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}

	tx, err := d.sql.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if err := insertCell(tx, cell); err != nil {
		return err
	}
	if err := syncSequenceAllocatorTx(tx, cell.Sequence); err != nil {
		return err
	}
	if err := insertManifest(tx, entries); err != nil {
		return err
	}
	if err := upsertBranchHeadTx(tx, branch, cell.ID, cell.Timestamp); err != nil {
		return err
	}
	if err := setMetaTx(tx, "head_cell", cell.ID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func insertCell(tx *sql.Tx, cell Cell) error {
	evalReq := 0
	if cell.EvalRequested {
		evalReq = 1
	}
	evalRan := 0
	if cell.EvalRan {
		evalRan = 1
	}
	_, err := tx.Exec(`
INSERT INTO cells (
	id, sequence, parent_id, timestamp, message, source, agent, tags, branch,
	files_added, files_modified, files_removed, lines_added, lines_removed,
	total_loc, loc_delta, total_files,
	eval_requested, eval_ran, tests_passed, tests_failed, lint_errors, type_errors,
	eval_skipped, eval_error
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`,
		cell.ID,
		cell.Sequence,
		cell.ParentID,
		cell.Timestamp,
		cell.Message,
		cell.Source,
		cell.Agent,
		cell.Tags,
		cell.Branch,
		cell.FilesAdded,
		cell.FilesModified,
		cell.FilesRemoved,
		cell.LinesAdded,
		cell.LinesRemoved,
		cell.TotalLOC,
		cell.LOCDelta,
		cell.TotalFiles,
		evalReq,
		evalRan,
		cell.TestsPassed,
		cell.TestsFailed,
		cell.LintErrors,
		cell.TypeErrors,
		cell.EvalSkipped,
		cell.EvalError,
	)
	if err != nil {
		return fmt.Errorf("insert cell: %w", err)
	}
	return nil
}

func insertManifest(tx *sql.Tx, entries []ManifestEntry) error {
	for _, e := range entries {
		_, err := tx.Exec(`
INSERT INTO manifest_entries (cell_id, path, hash, mode, size)
VALUES (?, ?, ?, ?, ?)
`, e.CellID, e.Path, e.Hash, e.Mode, e.Size)
		if err != nil {
			return fmt.Errorf("insert manifest entry %s: %w", e.Path, err)
		}
	}
	return nil
}

func upsertBranchHeadTx(tx *sql.Tx, branch string, headCellID string, createdAt string) error {
	_, err := tx.Exec(`
INSERT INTO branches (name, head_cell_id, created_at) VALUES (?, ?, ?)
ON CONFLICT(name) DO UPDATE SET head_cell_id = excluded.head_cell_id
`, branch, headCellID, createdAt)
	if err != nil {
		return fmt.Errorf("upsert branch head %s: %w", branch, err)
	}
	return nil
}

func setMetaTx(tx *sql.Tx, key, value string) error {
	_, err := tx.Exec(`INSERT OR REPLACE INTO meta (key, value) VALUES (?, ?)`, key, value)
	if err != nil {
		return fmt.Errorf("set meta %s: %w", key, err)
	}
	return nil
}

func syncSequenceAllocatorTx(tx *sql.Tx, sequence int) error {
	if _, err := tx.Exec(`INSERT OR IGNORE INTO cell_sequences (name, last_sequence) VALUES ('default', 0)`); err != nil {
		return fmt.Errorf("ensure sequence allocator row: %w", err)
	}
	if _, err := tx.Exec(`
UPDATE cell_sequences
SET last_sequence = CASE
	WHEN last_sequence < ? THEN ?
	ELSE last_sequence
END
WHERE name = 'default'
`, sequence, sequence); err != nil {
		return fmt.Errorf("sync sequence allocator: %w", err)
	}
	return nil
}

func (d *DB) GetCell(id string) (*Cell, error) {
	row := d.sql.QueryRow(cellSelect+` WHERE id = ?`, id)
	cell, err := scanCell(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get cell %s: %w", id, err)
	}
	return cell, nil
}

func (d *DB) LatestCell() (*Cell, error) {
	row := d.sql.QueryRow(cellSelect + ` ORDER BY sequence DESC LIMIT 1`)
	cell, err := scanCell(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("latest cell: %w", err)
	}
	return cell, nil
}

func (d *DB) LatestCellByBranch(branch string) (*Cell, error) {
	row := d.sql.QueryRow(cellSelect+` WHERE branch = ? ORDER BY sequence DESC LIMIT 1`, branch)
	cell, err := scanCell(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("latest cell by branch %s: %w", branch, err)
	}
	return cell, nil
}

func (d *DB) ListCells(limit int) ([]Cell, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := d.sql.Query(cellSelect+` ORDER BY sequence DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list cells: %w", err)
	}
	defer rows.Close()

	out := make([]Cell, 0, limit)
	for rows.Next() {
		cell, err := scanCell(rows)
		if err != nil {
			return nil, fmt.Errorf("scan cell: %w", err)
		}
		out = append(out, *cell)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cells: %w", err)
	}
	return out, nil
}

func (d *DB) ListCellsByBranch(branch string, limit int) ([]Cell, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := d.sql.Query(cellSelect+` WHERE branch = ? ORDER BY sequence DESC LIMIT ?`, branch, limit)
	if err != nil {
		return nil, fmt.Errorf("list cells by branch %s: %w", branch, err)
	}
	defer rows.Close()

	out := make([]Cell, 0, limit)
	for rows.Next() {
		cell, err := scanCell(rows)
		if err != nil {
			return nil, fmt.Errorf("scan cell: %w", err)
		}
		out = append(out, *cell)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate branch cells: %w", err)
	}
	return out, nil
}

func (d *DB) ListAllCells() ([]Cell, error) {
	rows, err := d.sql.Query(cellSelect + ` ORDER BY sequence ASC`)
	if err != nil {
		return nil, fmt.Errorf("list all cells: %w", err)
	}
	defer rows.Close()

	out := make([]Cell, 0)
	for rows.Next() {
		cell, err := scanCell(rows)
		if err != nil {
			return nil, fmt.Errorf("scan cell: %w", err)
		}
		out = append(out, *cell)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate all cells: %w", err)
	}
	return out, nil
}

func (d *DB) GetManifest(cellID string) ([]ManifestEntry, error) {
	rows, err := d.sql.Query(`
SELECT cell_id, path, hash, mode, size
FROM manifest_entries
WHERE cell_id = ?
ORDER BY path ASC
`, cellID)
	if err != nil {
		return nil, fmt.Errorf("get manifest %s: %w", cellID, err)
	}
	defer rows.Close()
	entries := make([]ManifestEntry, 0)
	for rows.Next() {
		var e ManifestEntry
		if err := rows.Scan(&e.CellID, &e.Path, &e.Hash, &e.Mode, &e.Size); err != nil {
			return nil, fmt.Errorf("scan manifest entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate manifest entries: %w", err)
	}
	return entries, nil
}

func (d *DB) GetMeta(key string) (string, error) {
	var value string
	err := d.sql.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get meta %s: %w", key, err)
	}
	return value, nil
}

func (d *DB) SetMeta(key, value string) error {
	_, err := d.sql.Exec(`INSERT OR REPLACE INTO meta (key, value) VALUES (?, ?)`, key, value)
	if err != nil {
		return fmt.Errorf("set meta %s: %w", key, err)
	}
	return nil
}

func (d *DB) CreateBranch(name string, headCellID *string, createdAt string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("branch name cannot be empty")
	}
	if strings.TrimSpace(createdAt) == "" {
		createdAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	_, err := d.sql.Exec(
		`INSERT INTO branches (name, head_cell_id, created_at) VALUES (?, ?, ?)`,
		name, headCellID, createdAt,
	)
	if err != nil {
		return fmt.Errorf("create branch %s: %w", name, err)
	}
	return nil
}

func (d *DB) GetBranch(name string) (*Branch, error) {
	var branch Branch
	err := d.sql.QueryRow(`SELECT name, head_cell_id, created_at FROM branches WHERE name = ?`, name).Scan(
		&branch.Name,
		&branch.HeadCellID,
		&branch.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get branch %s: %w", name, err)
	}
	return &branch, nil
}

func (d *DB) UpdateBranchHead(name string, headCellID *string) error {
	res, err := d.sql.Exec(`UPDATE branches SET head_cell_id = ? WHERE name = ?`, headCellID, name)
	if err != nil {
		return fmt.Errorf("update branch head %s: %w", name, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected for branch %s: %w", name, err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (d *DB) ListBranches() ([]Branch, error) {
	rows, err := d.sql.Query(`SELECT name, head_cell_id, created_at FROM branches ORDER BY created_at ASC, name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list branches: %w", err)
	}
	defer rows.Close()

	out := make([]Branch, 0)
	for rows.Next() {
		var branch Branch
		if err := rows.Scan(&branch.Name, &branch.HeadCellID, &branch.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan branch: %w", err)
		}
		out = append(out, branch)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate branches: %w", err)
	}
	return out, nil
}

func (d *DB) UpdateCellEval(
	id string,
	testsPassed *int,
	testsFailed *int,
	lintErrors *int,
	typeErrors *int,
	skipped *string,
	evalErr *string,
) error {
	_, err := d.sql.Exec(`
UPDATE cells
SET eval_requested = 1, eval_ran = 1, tests_passed = ?, tests_failed = ?, lint_errors = ?, type_errors = ?, eval_skipped = ?, eval_error = ?
WHERE id = ?
`, testsPassed, testsFailed, lintErrors, typeErrors, skipped, evalErr, id)
	if err != nil {
		return fmt.Errorf("update cell eval %s: %w", id, err)
	}
	return nil
}

const cellSelect = `
SELECT
	id, sequence, parent_id, timestamp, message, source, agent, tags, branch,
	files_added, files_modified, files_removed, lines_added, lines_removed,
	total_loc, loc_delta, total_files,
	eval_requested, eval_ran, tests_passed, tests_failed, lint_errors, type_errors,
	eval_skipped, eval_error
FROM cells`
