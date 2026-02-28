package core

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/prittamravi/converge/internal/db"
	"github.com/prittamravi/converge/internal/eval"
	"github.com/prittamravi/converge/internal/snapshot"
	"github.com/prittamravi/converge/internal/store"
)

const defaultBranchName = "main"

type Service struct {
	DB        *db.DB
	Store     *store.Store
	Snapshot  *snapshot.Snapshot
	Evaluator *eval.Runner

	ProjectDir string
}

func NewService(projectDir string, database *db.DB, objectStore *store.Store, evaluator *eval.Runner) *Service {
	return &Service{
		DB:         database,
		Store:      objectStore,
		Snapshot:   snapshot.New(objectStore),
		Evaluator:  evaluator,
		ProjectDir: projectDir,
	}
}

func (s *Service) ActiveBranch() (string, error) {
	branch, err := s.DB.GetMeta("active_branch")
	if err == db.ErrNotFound || strings.TrimSpace(branch) == "" {
		if setErr := s.DB.SetMeta("active_branch", defaultBranchName); setErr != nil {
			return "", fmt.Errorf("set default active branch: %w", setErr)
		}
		branch = defaultBranchName
	} else if err != nil {
		return "", fmt.Errorf("get active branch: %w", err)
	}
	branch = strings.TrimSpace(branch)
	if err := s.ensureBranchExists(branch); err != nil {
		return "", err
	}
	return branch, nil
}

func (s *Service) ForkBranch(name string, switchNow bool) (*db.Branch, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("branch name cannot be empty")
	}
	if strings.EqualFold(name, defaultBranchName) {
		if _, err := s.DB.GetBranch(defaultBranchName); err == nil {
			return nil, fmt.Errorf("branch %q already exists", defaultBranchName)
		}
	}

	activeBranch, err := s.ActiveBranch()
	if err != nil {
		return nil, err
	}

	headCell, err := s.branchHeadCell(activeBranch)
	if err != nil {
		return nil, fmt.Errorf("active branch head: %w", err)
	}

	var headCellID *string
	if headCell != nil {
		headCellID = &headCell.ID
	}
	createdAt := time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.DB.CreateBranch(name, headCellID, createdAt); err != nil {
		return nil, err
	}

	if switchNow {
		if err := s.DB.SetMeta("active_branch", name); err != nil {
			return nil, fmt.Errorf("set active branch: %w", err)
		}
		if err := s.setHeadCellMeta(headCellID); err != nil {
			return nil, err
		}
	}

	branch, err := s.DB.GetBranch(name)
	if err != nil {
		return nil, fmt.Errorf("load created branch: %w", err)
	}
	return branch, nil
}

func (s *Service) SwitchBranch(ctx context.Context, name string) (*db.Cell, *db.Cell, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil, fmt.Errorf("branch name cannot be empty")
	}
	branch, err := s.DB.GetBranch(name)
	if err != nil {
		if err == db.ErrNotFound {
			return nil, nil, fmt.Errorf("branch %q not found", name)
		}
		return nil, nil, err
	}
	if branch.HeadCellID == nil || strings.TrimSpace(*branch.HeadCellID) == "" {
		return nil, nil, fmt.Errorf("branch %q has no head cell to restore", name)
	}

	activeBranch, err := s.ActiveBranch()
	if err != nil {
		return nil, nil, err
	}
	if activeBranch == name {
		target, err := s.DB.GetCell(*branch.HeadCellID)
		if err != nil {
			return nil, nil, fmt.Errorf("load branch head %s: %w", *branch.HeadCellID, err)
		}
		return nil, target, nil
	}

	currentHead, err := s.branchHeadCell(activeBranch)
	if err != nil {
		return nil, nil, fmt.Errorf("current branch head %s: %w", activeBranch, err)
	}

	safety, err := s.CreateCell(ctx, SnapOptions{
		Message: fmt.Sprintf("safety snapshot before switch to %s", name),
		Source:  "restore_safety",
		RunEval: false,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create safety cell: %w", err)
	}

	cleanup, err := s.writeRestoreLock()
	if err != nil {
		return nil, nil, fmt.Errorf("write restore lock: %w", err)
	}
	defer cleanup()

	if err := s.restoreTrackedFilesToCell(*branch.HeadCellID, currentHead); err != nil {
		return nil, nil, err
	}

	if err := s.DB.SetMeta("active_branch", name); err != nil {
		return nil, nil, fmt.Errorf("set active branch: %w", err)
	}
	if err := s.setHeadCellMeta(branch.HeadCellID); err != nil {
		return nil, nil, err
	}

	target, err := s.DB.GetCell(*branch.HeadCellID)
	if err != nil {
		return nil, nil, fmt.Errorf("load target branch head: %w", err)
	}
	return safety, target, nil
}

func (s *Service) CreateCell(ctx context.Context, opts SnapOptions) (*db.Cell, error) {
	manifest, err := s.Snapshot.Capture(s.ProjectDir)
	if err != nil {
		return nil, fmt.Errorf("capture snapshot: %w", err)
	}
	return s.createCellFromManifest(ctx, manifest, opts, nil, nil)
}

func (s *Service) CreateCellIfChanged(ctx context.Context, opts SnapOptions) (*db.Cell, bool, error) {
	manifest, err := s.Snapshot.Capture(s.ProjectDir)
	if err != nil {
		return nil, false, fmt.Errorf("capture snapshot: %w", err)
	}

	branch, err := s.ActiveBranch()
	if err != nil {
		return nil, false, err
	}

	parent, err := s.branchHeadCell(branch)
	if err != nil {
		return nil, false, fmt.Errorf("branch head for %s: %w", branch, err)
	}

	var parentEntries []db.ManifestEntry
	if parent != nil {
		parentEntries, err = s.DB.GetManifest(parent.ID)
		if err != nil {
			return nil, false, fmt.Errorf("get parent manifest: %w", err)
		}
		parentMap := manifestHashesFromEntries(parentEntries)
		if snapshot.EqualToEntries(manifest, parentMap) {
			return nil, false, nil
		}
	}

	cell, err := s.createCellFromManifest(ctx, manifest, opts, parent, parentEntries)
	if err != nil {
		return nil, false, err
	}
	return cell, true, nil
}

func (s *Service) createCellFromManifest(
	ctx context.Context,
	manifest snapshot.Manifest,
	opts SnapOptions,
	knownParent *db.Cell,
	knownParentManifest []db.ManifestEntry,
) (*db.Cell, error) {
	branch, err := s.ActiveBranch()
	if err != nil {
		return nil, err
	}

	seq, err := s.DB.AllocateSequence()
	if err != nil {
		return nil, fmt.Errorf("allocate sequence: %w", err)
	}
	cellID := CellID(seq)

	parent := knownParent
	parentEntries := knownParentManifest
	if parent == nil {
		parent, err = s.branchHeadCell(branch)
		if err != nil {
			return nil, fmt.Errorf("branch head for %s: %w", branch, err)
		}
	}
	if parent != nil && parentEntries == nil {
		parentEntries, err = s.DB.GetManifest(parent.ID)
		if err != nil {
			return nil, fmt.Errorf("get parent manifest: %w", err)
		}
	}

	added, modified, removed, linesAdded, linesRemoved := computeDiffStats(manifest, parentEntries, s.Store)
	totalLOC, totalFiles := computeLOC(manifest, s.Store)
	locDelta := totalLOC
	if parent != nil {
		locDelta = totalLOC - parent.TotalLOC
	}

	source := strings.TrimSpace(opts.Source)
	if source == "" {
		source = "manual"
	}

	var parentID *string
	if parent != nil {
		parentID = &parent.ID
	}
	var agent *string
	if strings.TrimSpace(opts.Agent) != "" {
		a := strings.TrimSpace(opts.Agent)
		agent = &a
	}
	var tags *string
	if strings.TrimSpace(opts.Tags) != "" {
		t := strings.TrimSpace(opts.Tags)
		tags = &t
	}

	cell := db.Cell{
		ID:            cellID,
		Sequence:      seq,
		ParentID:      parentID,
		Timestamp:     time.Now().UTC().Format(time.RFC3339Nano),
		Message:       opts.Message,
		Source:        source,
		Agent:         agent,
		Tags:          tags,
		Branch:        branch,
		FilesAdded:    added,
		FilesModified: modified,
		FilesRemoved:  removed,
		LinesAdded:    linesAdded,
		LinesRemoved:  linesRemoved,
		TotalLOC:      totalLOC,
		LOCDelta:      locDelta,
		TotalFiles:    totalFiles,
		EvalRequested: opts.RunEval,
		EvalRan:       false,
	}

	entries := make([]db.ManifestEntry, 0, len(manifest))
	for _, path := range snapshot.SortedPaths(manifest) {
		fe := manifest[path]
		entries = append(entries, db.ManifestEntry{
			CellID: cellID,
			Path:   path,
			Hash:   fe.Hash,
			Mode:   int(fe.Mode),
			Size:   fe.Size,
		})
	}

	if err := s.DB.InsertCellWithManifestAndAdvanceBranch(cell, entries); err != nil {
		return nil, fmt.Errorf("insert cell: %w", err)
	}

	if opts.RunEval {
		if _, err := s.EvaluateCell(ctx, cellID); err != nil {
			// Evaluation is best-effort; persist failure text and still keep snapshot.
			errText := err.Error()
			if updateErr := s.DB.UpdateCellEval(cellID, nil, nil, nil, nil, nil, &errText); updateErr != nil {
				return nil, fmt.Errorf("persist eval failure: %w", updateErr)
			}
		}
	}

	created, err := s.DB.GetCell(cellID)
	if err != nil {
		return nil, fmt.Errorf("load created cell: %w", err)
	}
	return created, nil
}

func (s *Service) EvaluateCell(ctx context.Context, cellID string) (eval.Result, error) {
	if _, err := s.DB.GetCell(cellID); err != nil {
		if err == db.ErrNotFound {
			return eval.Result{}, fmt.Errorf("cell %s not found", cellID)
		}
		return eval.Result{}, err
	}
	if s.Evaluator == nil {
		return eval.Result{}, fmt.Errorf("evaluator is not configured")
	}

	result, err := s.Evaluator.Run(ctx, s.ProjectDir)
	var errText *string
	if err != nil {
		e := err.Error()
		errText = &e
	}
	if updateErr := s.DB.UpdateCellEval(
		cellID,
		result.TestsPassedPtr(),
		result.TestsFailedPtr(),
		result.LintErrorsPtr(),
		result.TypeErrorsPtr(),
		result.SkippedPtr(),
		errText,
	); updateErr != nil {
		return eval.Result{}, updateErr
	}
	return result, err
}

func (s *Service) WorkingTreeDelta(ctx context.Context) (*db.Cell, WorkingTreeDelta, error) {
	branch, err := s.ActiveBranch()
	if err != nil {
		return nil, WorkingTreeDelta{}, err
	}
	latest, err := s.branchHeadCell(branch)
	if err != nil {
		return nil, WorkingTreeDelta{}, err
	}
	if latest == nil {
		return nil, WorkingTreeDelta{}, nil
	}
	manifest, err := s.Snapshot.Capture(s.ProjectDir)
	if err != nil {
		return nil, WorkingTreeDelta{}, fmt.Errorf("capture current snapshot: %w", err)
	}
	latestEntries, err := s.DB.GetManifest(latest.ID)
	if err != nil {
		return nil, WorkingTreeDelta{}, fmt.Errorf("get latest manifest: %w", err)
	}
	latestMap := manifestHashesFromEntries(latestEntries)

	delta := WorkingTreeDelta{}
	for path, fileEntry := range manifest {
		hash, exists := latestMap[path]
		if !exists {
			delta.Added++
			continue
		}
		if hash != fileEntry.Hash {
			delta.Modified++
		}
	}
	for path := range latestMap {
		if _, exists := manifest[path]; !exists {
			delta.Removed++
		}
	}
	_ = ctx
	return latest, delta, nil
}

func (s *Service) ensureBranchExists(branch string) error {
	_, err := s.DB.GetBranch(branch)
	if err == nil {
		return nil
	}
	if err != db.ErrNotFound {
		return fmt.Errorf("get branch %s: %w", branch, err)
	}

	head, headErr := s.DB.LatestCellByBranch(branch)
	if headErr != nil {
		return fmt.Errorf("latest branch head for %s: %w", branch, headErr)
	}
	var headCellID *string
	if head != nil {
		headCellID = &head.ID
	}
	createdAt := time.Now().UTC().Format(time.RFC3339Nano)
	if createErr := s.DB.CreateBranch(branch, headCellID, createdAt); createErr != nil {
		return fmt.Errorf("create branch %s: %w", branch, createErr)
	}
	return nil
}

func (s *Service) branchHeadCell(branch string) (*db.Cell, error) {
	b, err := s.DB.GetBranch(branch)
	if err == db.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get branch %s: %w", branch, err)
	}
	if b.HeadCellID == nil || strings.TrimSpace(*b.HeadCellID) == "" {
		return nil, nil
	}
	cell, err := s.DB.GetCell(*b.HeadCellID)
	if err == db.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get branch head cell %s: %w", *b.HeadCellID, err)
	}
	return cell, nil
}

func (s *Service) setHeadCellMeta(headCellID *string) error {
	if headCellID == nil {
		if err := s.DB.SetMeta("head_cell", ""); err != nil {
			return fmt.Errorf("set empty head_cell meta: %w", err)
		}
		return nil
	}
	if err := s.DB.SetMeta("head_cell", *headCellID); err != nil {
		return fmt.Errorf("set head_cell meta: %w", err)
	}
	return nil
}

func computeDiffStats(current snapshot.Manifest, parentEntries []db.ManifestEntry, objectStore *store.Store) (added, modified, removed, linesAdded, linesRemoved int) {
	parentMap := manifestHashesFromEntries(parentEntries)

	for path, currentEntry := range current {
		oldHash, exists := parentMap[path]
		if !exists {
			added++
			if data, err := objectStore.Read(currentEntry.Hash); err == nil {
				linesAdded += countLines(data)
			}
			continue
		}
		if oldHash != currentEntry.Hash {
			modified++
			newLines := 0
			oldLines := 0
			if newData, err := objectStore.Read(currentEntry.Hash); err == nil {
				newLines = countLines(newData)
			}
			if oldData, err := objectStore.Read(oldHash); err == nil {
				oldLines = countLines(oldData)
			}
			if newLines >= oldLines {
				linesAdded += newLines - oldLines
			} else {
				linesRemoved += oldLines - newLines
			}
		}
	}

	for _, e := range parentEntries {
		if _, exists := current[e.Path]; !exists {
			removed++
			if data, err := objectStore.Read(e.Hash); err == nil {
				linesRemoved += countLines(data)
			}
		}
	}
	return
}

func computeLOC(manifest snapshot.Manifest, objectStore *store.Store) (int, int) {
	totalLOC := 0
	paths := make([]string, 0, len(manifest))
	for path := range manifest {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		data, err := objectStore.Read(manifest[path].Hash)
		if err != nil {
			continue
		}
		totalLOC += eval.CountLOC(path, string(data))
	}
	return totalLOC, len(manifest)
}

func countLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	count := 1
	for _, b := range data {
		if b == '\n' {
			count++
		}
	}
	if data[len(data)-1] == '\n' {
		count--
	}
	return count
}

func manifestHashesFromEntries(entries []db.ManifestEntry) map[string]string {
	out := make(map[string]string, len(entries))
	for _, entry := range entries {
		out[entry.Path] = entry.Hash
	}
	return out
}
