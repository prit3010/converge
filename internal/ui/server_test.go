package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/prittamravi/converge/internal/core"
	"github.com/prittamravi/converge/internal/db"
	"github.com/prittamravi/converge/internal/eval"
	"github.com/prittamravi/converge/internal/store"
)

func TestAPICellsAndBranches(t *testing.T) {
	svc := newUITestService(t)
	if err := os.WriteFile(filepath.Join(svc.ProjectDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	if _, err := svc.CreateCell(context.Background(), core.SnapOptions{Message: "base", RunEval: false}); err != nil {
		t.Fatalf("create cell: %v", err)
	}

	server, err := NewServer(svc)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	cellsReq := httptest.NewRequest(http.MethodGet, "/api/cells", nil)
	cellsRec := httptest.NewRecorder()
	server.ServeHTTP(cellsRec, cellsReq)
	if cellsRec.Code != http.StatusOK {
		t.Fatalf("cells status = %d, body=%s", cellsRec.Code, cellsRec.Body.String())
	}
	var cells []cellJSON
	if err := json.Unmarshal(cellsRec.Body.Bytes(), &cells); err != nil {
		t.Fatalf("decode cells json: %v", err)
	}
	if len(cells) != 1 {
		t.Fatalf("expected 1 cell, got %d", len(cells))
	}
	if cells[0].Branch != "main" {
		t.Fatalf("expected main branch, got %s", cells[0].Branch)
	}

	branchReq := httptest.NewRequest(http.MethodGet, "/api/branches", nil)
	branchRec := httptest.NewRecorder()
	server.ServeHTTP(branchRec, branchReq)
	if branchRec.Code != http.StatusOK {
		t.Fatalf("branches status = %d, body=%s", branchRec.Code, branchRec.Body.String())
	}
	var branches []branchJSON
	if err := json.Unmarshal(branchRec.Body.Bytes(), &branches); err != nil {
		t.Fatalf("decode branches json: %v", err)
	}
	if len(branches) == 0 {
		t.Fatalf("expected at least 1 branch")
	}

	summaryReq := httptest.NewRequest(http.MethodGet, "/api/ui/summary", nil)
	summaryRec := httptest.NewRecorder()
	server.ServeHTTP(summaryRec, summaryReq)
	if summaryRec.Code != http.StatusOK {
		t.Fatalf("summary status = %d, body=%s", summaryRec.Code, summaryRec.Body.String())
	}
	var summary uiSummaryJSON
	if err := json.Unmarshal(summaryRec.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode summary json: %v", err)
	}
	if summary.TotalCells != 1 {
		t.Fatalf("expected total cells 1, got %d", summary.TotalCells)
	}
	if summary.BaselineCellID != "c_000001" {
		t.Fatalf("expected baseline c_000001, got %s", summary.BaselineCellID)
	}
}

func TestAPICompareWithoutKeyReturnsGracefulError(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	svc := newUITestService(t)
	if err := os.WriteFile(filepath.Join(svc.ProjectDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	if _, err := svc.CreateCell(context.Background(), core.SnapOptions{Message: "first", RunEval: false}); err != nil {
		t.Fatalf("create first cell: %v", err)
	}
	if err := os.WriteFile(filepath.Join(svc.ProjectDir, "main.go"), []byte("package main\nfunc main(){}\n"), 0o644); err != nil {
		t.Fatalf("rewrite main.go: %v", err)
	}
	if _, err := svc.CreateCell(context.Background(), core.SnapOptions{Message: "second", RunEval: false}); err != nil {
		t.Fatalf("create second cell: %v", err)
	}

	server, err := NewServer(svc)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	body := bytes.NewBufferString(`{"cell_a":"c_000001","cell_b":"c_000002"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/compare", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected status 502, got %d body=%s", rec.Code, rec.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode compare error json: %v", err)
	}
	if _, ok := result["error"]; !ok {
		t.Fatalf("expected error key in response: %v", result)
	}
}

func TestAPIUISummaryEmptyHistory(t *testing.T) {
	svc := newUITestService(t)
	server, err := NewServer(svc)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/ui/summary", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var summary uiSummaryJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}

	if summary.TotalCells != 0 {
		t.Fatalf("expected no cells, got %d", summary.TotalCells)
	}
	if summary.TotalBranches == 0 {
		t.Fatalf("expected at least one branch, got %d", summary.TotalBranches)
	}
	if summary.ActiveBranch != "main" {
		t.Fatalf("expected active branch main, got %s", summary.ActiveBranch)
	}
	if summary.WinnerCellID != "" {
		t.Fatalf("expected empty winner id, got %s", summary.WinnerCellID)
	}
	if summary.BaselineCellID != "" {
		t.Fatalf("expected empty baseline id, got %s", summary.BaselineCellID)
	}
	if summary.PassRate != 0 {
		t.Fatalf("expected pass rate 0, got %v", summary.PassRate)
	}
	if summary.ForkPoints != 0 {
		t.Fatalf("expected fork points 0, got %d", summary.ForkPoints)
	}
}

func TestAPIUISummaryWinnerHeuristic(t *testing.T) {
	svc := newUITestService(t)
	projectFile := filepath.Join(svc.ProjectDir, "main.go")

	createCell := func(content string, message string) string {
		t.Helper()
		if err := os.WriteFile(projectFile, []byte(content), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}
		cell, err := svc.CreateCell(context.Background(), core.SnapOptions{Message: message, RunEval: false})
		if err != nil {
			t.Fatalf("create cell %s: %v", message, err)
		}
		return cell.ID
	}

	c1 := createCell("package main\n", "baseline")
	c2 := createCell("package main\nfunc a() {}\n", "has failures")
	c3 := createCell("package main\nfunc a() {}\nfunc b() {}\n", "passes with lint")
	c4 := createCell("package main\nfunc a() {}\nfunc b() {}\nfunc c() {}\n", "passes cleaner")
	c5 := createCell("package main\nfunc a() {}\nfunc b() {}\nfunc c() {}\nfunc d() {}\n", "passes cleaner+")
	c6 := createCell("package main\nfunc a() {}\nfunc b() {}\nfunc c() {}\nfunc d() {}\nfunc e() {}\n", "tie breaker latest")

	if err := svc.DB.UpdateCellEval(c2, intPtr(10), intPtr(2), intPtr(0), intPtr(0), nil, nil); err != nil {
		t.Fatalf("eval c2: %v", err)
	}
	if err := svc.DB.UpdateCellEval(c3, intPtr(8), intPtr(0), intPtr(3), intPtr(0), nil, nil); err != nil {
		t.Fatalf("eval c3: %v", err)
	}
	if err := svc.DB.UpdateCellEval(c4, intPtr(9), intPtr(0), intPtr(1), intPtr(0), nil, nil); err != nil {
		t.Fatalf("eval c4: %v", err)
	}
	if err := svc.DB.UpdateCellEval(c5, intPtr(11), intPtr(0), intPtr(1), intPtr(0), nil, nil); err != nil {
		t.Fatalf("eval c5: %v", err)
	}
	if err := svc.DB.UpdateCellEval(c6, intPtr(11), intPtr(0), intPtr(1), intPtr(0), nil, nil); err != nil {
		t.Fatalf("eval c6: %v", err)
	}

	server, err := NewServer(svc)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/ui/summary", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var summary uiSummaryJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}

	if summary.BaselineCellID != c1 {
		t.Fatalf("expected baseline %s, got %s", c1, summary.BaselineCellID)
	}
	if summary.WinnerCellID != c6 {
		t.Fatalf("expected winner %s, got %s", c6, summary.WinnerCellID)
	}
	if summary.PassRate != 80 {
		t.Fatalf("expected pass rate 80, got %v", summary.PassRate)
	}
	if summary.ForkPoints != 0 {
		t.Fatalf("expected 0 fork points for linear history, got %d", summary.ForkPoints)
	}
}

func TestPickWinnerCellFallsBackToLatestWithoutEval(t *testing.T) {
	cells := []db.Cell{
		{ID: "c_000001", Sequence: 1},
		{ID: "c_000002", Sequence: 2},
	}
	winner := pickWinnerCell(cells)
	if winner == nil {
		t.Fatalf("expected winner, got nil")
	}
	if winner.ID != "c_000002" {
		t.Fatalf("expected latest cell c_000002, got %s", winner.ID)
	}
}

func intPtr(v int) *int {
	return &v
}

func newUITestService(t *testing.T) *core.Service {
	t.Helper()
	projectDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectDir, ".converge", "objects"), 0o755); err != nil {
		t.Fatalf("mkdir state dirs: %v", err)
	}
	database, err := db.Open(filepath.Join(projectDir, ".converge", "converge.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})
	objectStore := store.New(filepath.Join(projectDir, ".converge", "objects"))
	return core.NewService(projectDir, database, objectStore, eval.NewRunner())
}
