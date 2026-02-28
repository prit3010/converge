package llm

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/prittamravi/converge/internal/db"
	"github.com/prittamravi/converge/internal/store"
)

func TestParseCompareResponse(t *testing.T) {
	content := "SUMMARY: Cell B simplifies auth and keeps tests green.\nWINNER: c_000002 - cleaner middleware architecture\nHIGHLIGHTS:\n- Extracted inline checks into middleware\n- Removed duplicate logic\n- Kept test pass rate"
	result := parseCompareResponse(content)
	if !strings.Contains(result.Summary, "simplifies auth") {
		t.Fatalf("unexpected summary: %q", result.Summary)
	}
	if !strings.Contains(result.Winner, "c_000002") {
		t.Fatalf("unexpected winner: %q", result.Winner)
	}
	if len(result.Highlights) != 3 {
		t.Fatalf("expected 3 highlights, got %d", len(result.Highlights))
	}
}

func TestBuildPromptIncludesMetadataAndDiffs(t *testing.T) {
	tmp := t.TempDir()
	database, err := db.Open(filepath.Join(tmp, "converge.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	objectStore := store.New(filepath.Join(tmp, "objects"))

	oldHash, err := objectStore.Write([]byte("package main\n\nfunc main() {\n\tprintln(\"a\")\n}\n"))
	if err != nil {
		t.Fatalf("write old object: %v", err)
	}
	newHash, err := objectStore.Write([]byte("package main\n\nfunc main() {\n\tprintln(\"b\")\n}\n"))
	if err != nil {
		t.Fatalf("write new object: %v", err)
	}

	c1 := db.Cell{ID: "c_000001", Sequence: 1, Timestamp: "2026-02-28T00:00:00Z", Message: "base", Source: "manual", Branch: "main", TotalLOC: 4, LOCDelta: 4, TotalFiles: 1}
	c2 := db.Cell{ID: "c_000002", Sequence: 2, ParentID: strPtr("c_000001"), Timestamp: "2026-02-28T00:00:01Z", Message: "update", Source: "manual", Branch: "main", TotalLOC: 4, LOCDelta: 0, TotalFiles: 1}

	if err := database.InsertCellWithManifestAndAdvanceBranch(c1, []db.ManifestEntry{{CellID: c1.ID, Path: "main.go", Hash: oldHash, Mode: 0o644, Size: 32}}); err != nil {
		t.Fatalf("insert c1: %v", err)
	}
	if err := database.InsertCellWithManifestAndAdvanceBranch(c2, []db.ManifestEntry{{CellID: c2.ID, Path: "main.go", Hash: newHash, Mode: 0o644, Size: 32}}); err != nil {
		t.Fatalf("insert c2: %v", err)
	}

	comparer := NewComparer(database, objectStore)
	prompt, err := comparer.buildPrompt(&c1, &c2, CompareOptions{MaxDiffLines: 200})
	if err != nil {
		t.Fatalf("build prompt: %v", err)
	}
	if !strings.Contains(prompt, "Cell A: c_000001") {
		t.Fatalf("prompt missing cell A header: %s", prompt)
	}
	if !strings.Contains(prompt, "Cell B: c_000002") {
		t.Fatalf("prompt missing cell B header: %s", prompt)
	}
	if !strings.Contains(prompt, "### main.go") {
		t.Fatalf("prompt missing modified file section: %s", prompt)
	}
}

func strPtr(v string) *string {
	return &v
}
