package ui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/prit3010/converge/internal/db"
	"github.com/prit3010/converge/internal/diff"
	"github.com/prit3010/converge/internal/llm"
	"github.com/prit3010/converge/internal/snapshot"
)

type cellJSON struct {
	ID            string  `json:"id"`
	Sequence      int     `json:"sequence"`
	ParentID      *string `json:"parent_id"`
	Timestamp     string  `json:"timestamp"`
	Message       string  `json:"message"`
	Source        string  `json:"source"`
	Branch        string  `json:"branch"`
	FilesAdded    int     `json:"files_added"`
	FilesModified int     `json:"files_modified"`
	FilesRemoved  int     `json:"files_removed"`
	LinesAdded    int     `json:"lines_added"`
	LinesRemoved  int     `json:"lines_removed"`
	TotalLOC      int     `json:"total_loc"`
	LOCDelta      int     `json:"loc_delta"`
	TotalFiles    int     `json:"total_files"`
	TestsPassed   *int    `json:"tests_passed"`
	TestsFailed   *int    `json:"tests_failed"`
	LintErrors    *int    `json:"lint_errors"`
	TypeErrors    *int    `json:"type_errors"`
}

type fileJSON struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type cellDetailJSON struct {
	cellJSON
	Files []fileJSON `json:"files"`
}

type branchJSON struct {
	Name       string `json:"name"`
	HeadCellID string `json:"head_cell_id"`
	Active     bool   `json:"active"`
}

type archiveJSON struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Current     bool   `json:"current"`
	ReadOnly    bool   `json:"read_only"`
	CommitSHA   string `json:"commit_sha,omitempty"`
	Branch      string `json:"branch,omitempty"`
	Subject     string `json:"subject,omitempty"`
	CommittedAt string `json:"committed_at,omitempty"`
	ArchivedAt  string `json:"archived_at,omitempty"`
	CellCount   int    `json:"cell_count"`
}

type diffJSON struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Diff   string `json:"diff,omitempty"`
}

type uiSummaryJSON struct {
	TotalCells     int     `json:"total_cells"`
	TotalBranches  int     `json:"total_branches"`
	ActiveBranch   string  `json:"active_branch"`
	WinnerCellID   string  `json:"winner_cell_id"`
	BaselineCellID string  `json:"baseline_cell_id"`
	PassRate       float64 `json:"pass_rate"`
	ForkPoints     int     `json:"fork_points"`
}

const apiCompareTimeout = 45 * time.Second

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if err := s.tmpl.ExecuteTemplate(w, "index.html", nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleAPICells(w http.ResponseWriter, r *http.Request) {
	src, ok := s.dataSourceFromRequest(w, r)
	if !ok {
		return
	}
	defer src.Close()

	cells, err := src.DB.ListAllCells()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]cellJSON, 0, len(cells))
	for _, c := range cells {
		out = append(out, toCellJSON(c))
	}
	writeJSON(w, out)
}

func (s *Server) handleAPIArchives(w http.ResponseWriter, r *http.Request) {
	currentCells, err := s.svc.DB.CountCells()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	archives, err := s.svc.ListArchiveMetadata()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	out := make([]archiveJSON, 0, len(archives)+1)
	out = append(out, archiveJSON{
		ID:        "current",
		Label:     "Current",
		Current:   true,
		ReadOnly:  false,
		CellCount: currentCells,
	})
	for _, archive := range archives {
		out = append(out, toArchiveOptionJSON(archive))
	}
	writeJSON(w, out)
}

func (s *Server) handleAPICell(w http.ResponseWriter, r *http.Request) {
	src, ok := s.dataSourceFromRequest(w, r)
	if !ok {
		return
	}
	defer src.Close()

	id := r.PathValue("id")
	cell, err := src.DB.GetCell(id)
	if err != nil {
		http.Error(w, "cell not found", http.StatusNotFound)
		return
	}
	manifest, err := src.DB.GetManifest(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	files := make([]fileJSON, 0, len(manifest))
	for _, m := range manifest {
		files = append(files, fileJSON{Path: m.Path, Size: m.Size})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	writeJSON(w, cellDetailJSON{cellJSON: toCellJSON(*cell), Files: files})
}

func (s *Server) handleAPIDiff(w http.ResponseWriter, r *http.Request) {
	src, ok := s.dataSourceFromRequest(w, r)
	if !ok {
		return
	}
	defer src.Close()

	cellA := r.PathValue("cellA")
	cellB := r.PathValue("cellB")

	manifestA, err := src.DB.GetManifest(cellA)
	if err != nil {
		http.Error(w, "cell A not found", http.StatusNotFound)
		return
	}
	manifestB, err := src.DB.GetManifest(cellB)
	if err != nil {
		http.Error(w, "cell B not found", http.StatusNotFound)
		return
	}

	mapA := make(map[string]string, len(manifestA))
	for _, e := range manifestA {
		mapA[e.Path] = e.Hash
	}
	mapB := make(map[string]string, len(manifestB))
	for _, e := range manifestB {
		mapB[e.Path] = e.Hash
	}

	result := diff.CompareManifests(mapA, mapB)
	diffs := make([]diffJSON, 0, len(result.Added)+len(result.Modified)+len(result.Removed))
	for _, p := range result.Added {
		diffs = append(diffs, diffJSON{Path: p, Status: "added"})
	}
	for _, p := range result.Removed {
		diffs = append(diffs, diffJSON{Path: p, Status: "removed"})
	}
	for _, p := range result.Modified {
		oldData, errOld := src.Store.Read(mapA[p])
		newData, errNew := src.Store.Read(mapB[p])
		patch := ""
		if errOld == nil && errNew == nil && snapshot.IsText(oldData) && snapshot.IsText(newData) {
			patch = diff.UnifiedDiff(p, string(oldData), string(newData))
		}
		diffs = append(diffs, diffJSON{Path: p, Status: "modified", Diff: patch})
	}
	writeJSON(w, diffs)
}

func (s *Server) handleAPIBranches(w http.ResponseWriter, r *http.Request) {
	src, ok := s.dataSourceFromRequest(w, r)
	if !ok {
		return
	}
	defer src.Close()

	activeBranch := activeBranchForDB(src.DB)
	branches, err := src.DB.ListBranches()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]branchJSON, 0, len(branches))
	for _, b := range branches {
		head := ""
		if b.HeadCellID != nil {
			head = *b.HeadCellID
		}
		out = append(out, branchJSON{Name: b.Name, HeadCellID: head, Active: b.Name == activeBranch})
	}
	writeJSON(w, out)
}

func (s *Server) handleAPIUISummary(w http.ResponseWriter, r *http.Request) {
	src, ok := s.dataSourceFromRequest(w, r)
	if !ok {
		return
	}
	defer src.Close()

	activeBranch := activeBranchForDB(src.DB)
	cells, err := src.DB.ListAllCells()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	branches, err := src.DB.ListBranches()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	summary := uiSummaryJSON{
		TotalCells:    len(cells),
		TotalBranches: len(branches),
		ActiveBranch:  activeBranch,
		PassRate:      calculatePassRate(cells),
		ForkPoints:    countForkPoints(cells),
	}

	if len(cells) > 0 {
		summary.BaselineCellID = cells[0].ID
		if winner := pickWinnerCell(cells); winner != nil {
			summary.WinnerCellID = winner.ID
		}
	}

	writeJSON(w, summary)
}

func (s *Server) handleAPICompare(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CellA        string `json:"cell_a"`
		CellB        string `json:"cell_b"`
		Model        string `json:"model"`
		MaxDiffLines int    `json:"max_diff_lines"`
		Archive      string `json:"archive"`
		ArchiveA     string `json:"archive_a"`
		ArchiveB     string `json:"archive_b"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.CellA = strings.TrimSpace(req.CellA)
	req.CellB = strings.TrimSpace(req.CellB)
	if req.CellA == "" || req.CellB == "" {
		http.Error(w, "cell_a and cell_b are required", http.StatusBadRequest)
		return
	}

	archiveID := archiveIDFromQuery(req.Archive)
	archiveA := strings.TrimSpace(req.ArchiveA)
	archiveB := strings.TrimSpace(req.ArchiveB)
	if archiveA != "" || archiveB != "" {
		left := archiveID
		right := archiveID
		if archiveA != "" {
			left = archiveIDFromQuery(archiveA)
		}
		if archiveB != "" {
			right = archiveIDFromQuery(archiveB)
		}
		if left != right {
			http.Error(w, "cross-archive compare is not supported", http.StatusBadRequest)
			return
		}
		archiveID = left
	}

	src, err := s.openDataSource(archiveID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			http.Error(w, "archive not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer src.Close()

	ctx, cancel := context.WithTimeout(r.Context(), apiCompareTimeout)
	defer cancel()

	comparer := llm.NewComparer(src.DB, src.Store)
	result, err := comparer.Compare(ctx, req.CellA, req.CellB, llm.CompareOptions{
		Model:        strings.TrimSpace(req.Model),
		MaxDiffLines: req.MaxDiffLines,
	})
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			http.Error(w, "compare request timed out", http.StatusGatewayTimeout)
			return
		}
		if result != nil && result.Error != "" {
			w.WriteHeader(http.StatusBadGateway)
			writeJSON(w, result)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, result)
}

func (s *Server) dataSourceFromRequest(w http.ResponseWriter, r *http.Request) (*dataSource, bool) {
	src, err := s.openDataSource(r.URL.Query().Get("archive"))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			http.Error(w, "archive not found", http.StatusNotFound)
			return nil, false
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil, false
	}
	return src, true
}

func toCellJSON(c db.Cell) cellJSON {
	return cellJSON{
		ID:            c.ID,
		Sequence:      c.Sequence,
		ParentID:      c.ParentID,
		Timestamp:     c.Timestamp,
		Message:       c.Message,
		Source:        c.Source,
		Branch:        c.Branch,
		FilesAdded:    c.FilesAdded,
		FilesModified: c.FilesModified,
		FilesRemoved:  c.FilesRemoved,
		LinesAdded:    c.LinesAdded,
		LinesRemoved:  c.LinesRemoved,
		TotalLOC:      c.TotalLOC,
		LOCDelta:      c.LOCDelta,
		TotalFiles:    c.TotalFiles,
		TestsPassed:   c.TestsPassed,
		TestsFailed:   c.TestsFailed,
		LintErrors:    c.LintErrors,
		TypeErrors:    c.TypeErrors,
	}
}

func pickWinnerCell(cells []db.Cell) *db.Cell {
	if len(cells) == 0 {
		return nil
	}

	evaluated := make([]db.Cell, 0, len(cells))
	for _, cell := range cells {
		if cellHasEval(cell) {
			evaluated = append(evaluated, cell)
		}
	}

	// If there is no evaluation data at all, default to the latest attempt.
	pool := evaluated
	if len(pool) == 0 {
		pool = cells
	}

	best := pool[0]
	for i := 1; i < len(pool); i += 1 {
		if winnerPreferred(pool[i], best) {
			best = pool[i]
		}
	}
	return &best
}

func winnerPreferred(candidate, current db.Cell) bool {
	candidateFailed := ptrInt(candidate.TestsFailed)
	currentFailed := ptrInt(current.TestsFailed)
	if candidateFailed != currentFailed {
		return candidateFailed < currentFailed
	}

	candidateLintType := ptrInt(candidate.LintErrors) + ptrInt(candidate.TypeErrors)
	currentLintType := ptrInt(current.LintErrors) + ptrInt(current.TypeErrors)
	if candidateLintType != currentLintType {
		return candidateLintType < currentLintType
	}

	candidatePassed := ptrInt(candidate.TestsPassed)
	currentPassed := ptrInt(current.TestsPassed)
	if candidatePassed != currentPassed {
		return candidatePassed > currentPassed
	}

	return candidate.Sequence > current.Sequence
}

func calculatePassRate(cells []db.Cell) float64 {
	totalEvaluated := 0
	totalPassed := 0
	for _, cell := range cells {
		if cell.TestsPassed == nil && cell.TestsFailed == nil {
			continue
		}
		totalEvaluated += 1
		if ptrInt(cell.TestsFailed) == 0 {
			totalPassed += 1
		}
	}
	if totalEvaluated == 0 {
		return 0
	}
	return (float64(totalPassed) / float64(totalEvaluated)) * 100
}

func countForkPoints(cells []db.Cell) int {
	childCountByParent := make(map[string]int)
	for _, cell := range cells {
		if cell.ParentID == nil {
			continue
		}
		parentID := strings.TrimSpace(*cell.ParentID)
		if parentID == "" {
			continue
		}
		childCountByParent[parentID] += 1
	}

	forkPoints := 0
	for _, count := range childCountByParent {
		if count > 1 {
			forkPoints += 1
		}
	}
	return forkPoints
}

func cellHasEval(cell db.Cell) bool {
	return cell.TestsPassed != nil ||
		cell.TestsFailed != nil ||
		cell.LintErrors != nil ||
		cell.TypeErrors != nil
}

func ptrInt(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
