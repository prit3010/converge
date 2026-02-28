package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

func TestOpenCreatesSchemaAndIsIdempotent(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "converge.db")

	d1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("first open failed: %v", err)
	}
	if err := d1.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	d2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("second open failed: %v", err)
	}
	defer d2.Close()

	if err := d2.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

func TestCellAndManifestRoundTrip(t *testing.T) {
	d := openTestDB(t)
	defer d.Close()

	seq, err := d.NextSequence()
	if err != nil {
		t.Fatalf("next sequence: %v", err)
	}
	if seq != 1 {
		t.Fatalf("expected first sequence 1, got %d", seq)
	}

	cell := Cell{
		ID:         "c_000001",
		Sequence:   1,
		Timestamp:  "2026-02-28T00:00:00Z",
		Message:    "first",
		Source:     "manual",
		TotalLOC:   10,
		LOCDelta:   10,
		TotalFiles: 2,
	}
	entries := []ManifestEntry{
		{CellID: cell.ID, Path: "main.go", Hash: "abc", Mode: 0o644, Size: 10},
		{CellID: cell.ID, Path: "lib.go", Hash: "def", Mode: 0o644, Size: 20},
	}
	if err := d.InsertCellWithManifest(cell, entries); err != nil {
		t.Fatalf("insert cell with manifest: %v", err)
	}

	got, err := d.GetCell(cell.ID)
	if err != nil {
		t.Fatalf("get cell: %v", err)
	}
	if got.ID != cell.ID || got.TotalLOC != 10 || got.TotalFiles != 2 {
		t.Fatalf("unexpected cell: %+v", got)
	}

	manifest, err := d.GetManifest(cell.ID)
	if err != nil {
		t.Fatalf("get manifest: %v", err)
	}
	if len(manifest) != 2 {
		t.Fatalf("expected 2 manifest entries, got %d", len(manifest))
	}

	latest, err := d.LatestCell()
	if err != nil {
		t.Fatalf("latest cell: %v", err)
	}
	if latest == nil || latest.ID != cell.ID {
		t.Fatalf("unexpected latest cell: %+v", latest)
	}

	list, err := d.ListCells(10)
	if err != nil {
		t.Fatalf("list cells: %v", err)
	}
	if len(list) != 1 || list[0].ID != cell.ID {
		t.Fatalf("unexpected list: %+v", list)
	}
}

func TestBranchingDefaultsAndHeadAdvance(t *testing.T) {
	d := openTestDB(t)
	defer d.Close()

	activeBranch, err := d.GetMeta("active_branch")
	if err != nil {
		t.Fatalf("get active_branch: %v", err)
	}
	if activeBranch != "main" {
		t.Fatalf("expected active branch main, got %q", activeBranch)
	}

	cell := Cell{
		ID:         "c_000001",
		Sequence:   1,
		Timestamp:  "2026-02-28T00:00:00Z",
		Message:    "first",
		Source:     "manual",
		TotalLOC:   10,
		LOCDelta:   10,
		TotalFiles: 1,
	}
	entries := []ManifestEntry{
		{CellID: cell.ID, Path: "main.go", Hash: "abc", Mode: 0o644, Size: 10},
	}
	if err := d.InsertCellWithManifestAndAdvanceBranch(cell, entries); err != nil {
		t.Fatalf("insert cell with branch head advance: %v", err)
	}

	inserted, err := d.GetCell(cell.ID)
	if err != nil {
		t.Fatalf("get inserted cell: %v", err)
	}
	if inserted.Branch != "main" {
		t.Fatalf("expected inserted cell on main branch, got %q", inserted.Branch)
	}

	mainBranch, err := d.GetBranch("main")
	if err != nil {
		t.Fatalf("get main branch: %v", err)
	}
	if mainBranch.HeadCellID == nil || *mainBranch.HeadCellID != cell.ID {
		t.Fatalf("expected main branch head %s, got %+v", cell.ID, mainBranch.HeadCellID)
	}

	headMeta, err := d.GetMeta("head_cell")
	if err != nil {
		t.Fatalf("get head_cell: %v", err)
	}
	if headMeta != cell.ID {
		t.Fatalf("expected head_cell meta %s, got %s", cell.ID, headMeta)
	}
}

func TestOpenMigratesLegacySchemaWithoutBranchColumn(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "legacy.db")

	legacyDB, err := sql.Open("sqlite", fmt.Sprintf("file:%s", dbPath))
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	defer legacyDB.Close()

	const legacySchema = `
CREATE TABLE cells (
	id TEXT PRIMARY KEY,
	sequence INTEGER UNIQUE NOT NULL,
	parent_id TEXT,
	timestamp TEXT NOT NULL,
	message TEXT NOT NULL DEFAULT '',
	source TEXT NOT NULL DEFAULT 'manual',
	agent TEXT,
	tags TEXT,
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
CREATE TABLE manifest_entries (
	cell_id TEXT NOT NULL,
	path TEXT NOT NULL,
	hash TEXT NOT NULL,
	mode INTEGER NOT NULL,
	size INTEGER NOT NULL,
	PRIMARY KEY (cell_id, path),
	FOREIGN KEY(cell_id) REFERENCES cells(id) ON DELETE CASCADE
);
CREATE INDEX idx_cells_sequence ON cells(sequence DESC);
CREATE INDEX idx_manifest_cell ON manifest_entries(cell_id);
`
	if _, err := legacyDB.Exec(legacySchema); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	if _, err := legacyDB.Exec(`INSERT INTO cells (id, sequence, timestamp, message) VALUES ('c_000001', 1, '2026-02-28T00:00:00Z', 'legacy')`); err != nil {
		t.Fatalf("insert legacy cell: %v", err)
	}

	d, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open migrated db: %v", err)
	}
	defer d.Close()

	cell, err := d.GetCell("c_000001")
	if err != nil {
		t.Fatalf("get migrated cell: %v", err)
	}
	if cell.Branch != "main" {
		t.Fatalf("expected migrated branch main, got %q", cell.Branch)
	}
}

func TestOpenCreatesSequenceAndAgentRunTables(t *testing.T) {
	d := openTestDB(t)
	defer d.Close()

	assertTable := func(name string) {
		t.Helper()
		var got string
		if err := d.sql.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&got); err != nil {
			t.Fatalf("table %s missing: %v", name, err)
		}
		if got != name {
			t.Fatalf("expected table %s, got %s", name, got)
		}
	}

	assertTable("cell_sequences")
	assertTable("agent_runs")
}

func TestAllocateSequenceConcurrentUnique(t *testing.T) {
	d := openTestDB(t)
	defer d.Close()

	const workers = 24
	var wg sync.WaitGroup
	seqs := make(chan int, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			seq, err := d.AllocateSequence()
			if err != nil {
				t.Errorf("allocate sequence: %v", err)
				return
			}
			seqs <- seq
		}()
	}
	wg.Wait()
	close(seqs)

	seen := make(map[int]bool, workers)
	for seq := range seqs {
		if seen[seq] {
			t.Fatalf("duplicate sequence allocated: %d", seq)
		}
		seen[seq] = true
	}
	if len(seen) != workers {
		t.Fatalf("expected %d unique sequences, got %d", workers, len(seen))
	}
	for i := 1; i <= workers; i++ {
		if !seen[i] {
			t.Fatalf("missing allocated sequence %d", i)
		}
	}
}

func openTestDB(t *testing.T) *DB {
	t.Helper()
	d, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return d
}
