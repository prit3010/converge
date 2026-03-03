package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/prit3010/converge/internal/config"
	"github.com/prit3010/converge/internal/db"
	"github.com/prit3010/converge/internal/snapshot"
	"github.com/prit3010/converge/internal/store"
)

const gitCommitBaselineSource = "git_commit_baseline"

type GitCommitMetadata struct {
	SHA         string
	Branch      string
	Subject     string
	CommittedAt string
}

type ArchiveMeta struct {
	ArchiveID   string `json:"archive_id"`
	CommitSHA   string `json:"commit_sha"`
	Branch      string `json:"branch"`
	Subject     string `json:"subject"`
	CommittedAt string `json:"committed_at"`
	ArchivedAt  string `json:"archived_at"`
	CellCount   int    `json:"cell_count"`
}

type ArchiveRotationResult struct {
	Archive      *ArchiveMeta
	BaselineCell *db.Cell
}

func (s *Service) RotateOnGitCommit(ctx context.Context, meta GitCommitMetadata) (*ArchiveRotationResult, error) {
	sha := strings.TrimSpace(meta.SHA)
	if sha == "" {
		return nil, fmt.Errorf("commit sha cannot be empty")
	}
	branch := strings.TrimSpace(meta.Branch)
	if branch == "" {
		branch = "detached"
	}
	subject := strings.TrimSpace(meta.Subject)
	if subject == "" {
		subject = "(no subject)"
	}
	committedAt := strings.TrimSpace(meta.CommittedAt)
	if committedAt == "" {
		committedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}

	if s.IsRestoreInProgress() {
		return nil, fmt.Errorf("cannot rotate while restore is in progress")
	}

	unlock, err := s.writeArchiveLock()
	if err != nil {
		return nil, err
	}
	defer unlock()

	cellCount, err := s.DB.CountCells()
	if err != nil {
		return nil, fmt.Errorf("count active cells: %w", err)
	}

	stateDir := filepath.Join(s.ProjectDir, config.StateDirName)
	dbPath := filepath.Join(stateDir, config.DBFileName)
	objectsPath := filepath.Join(stateDir, config.ObjectsDirName)

	if err := s.DB.Close(); err != nil {
		return nil, fmt.Errorf("close active db before rotate: %w", err)
	}
	s.DB = nil

	var archiveMeta *ArchiveMeta
	if cellCount > 0 {
		metaCopy, rotateErr := s.rotateStateToArchive(stateDir, dbPath, objectsPath, sha, branch, subject, committedAt, cellCount)
		if rotateErr != nil {
			return nil, rotateErr
		}
		archiveMeta = metaCopy
	} else {
		if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("remove active db: %w", err)
		}
		if err := os.RemoveAll(objectsPath); err != nil {
			return nil, fmt.Errorf("remove active objects dir: %w", err)
		}
	}

	if err := os.MkdirAll(objectsPath, 0o755); err != nil {
		return nil, fmt.Errorf("create active objects dir: %w", err)
	}

	freshDB, err := db.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open fresh active db: %w", err)
	}
	s.DB = freshDB
	s.Store = store.New(objectsPath)
	s.Snapshot = snapshot.NewWithPolicy(s.Store, s.Policy)

	baseline, err := s.createCommitBaselineCell(ctx, sha, subject)
	if err != nil {
		return nil, err
	}

	return &ArchiveRotationResult{Archive: archiveMeta, BaselineCell: baseline}, nil
}

func (s *Service) ListArchiveMetadata() ([]ArchiveMeta, error) {
	archivesDir := filepath.Join(s.ProjectDir, config.StateDirName, config.ArchivesDirName)
	entries, err := os.ReadDir(archivesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []ArchiveMeta{}, nil
		}
		return nil, fmt.Errorf("read archives directory: %w", err)
	}

	out := make([]ArchiveMeta, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		metaPath := filepath.Join(archivesDir, entry.Name(), config.ArchiveMetaFile)
		data, readErr := os.ReadFile(metaPath)
		if readErr != nil {
			continue
		}
		var meta ArchiveMeta
		if unmarshalErr := json.Unmarshal(data, &meta); unmarshalErr != nil {
			continue
		}
		if strings.TrimSpace(meta.ArchiveID) == "" {
			meta.ArchiveID = entry.Name()
		}
		out = append(out, meta)
	}

	sort.Slice(out, func(i, j int) bool {
		ti := parseRFC3339NanoOrZero(out[i].ArchivedAt)
		tj := parseRFC3339NanoOrZero(out[j].ArchivedAt)
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return out[i].ArchiveID > out[j].ArchiveID
	})

	return out, nil
}

func (s *Service) ArchiveStatePaths(archiveID string) (dbPath string, objectsPath string, err error) {
	archiveID = strings.TrimSpace(archiveID)
	if archiveID == "" || archiveID == "current" {
		return "", "", fmt.Errorf("archive id must be non-empty and not current")
	}
	if strings.Contains(archiveID, "..") || strings.ContainsRune(archiveID, os.PathSeparator) || strings.ContainsRune(archiveID, '/') {
		return "", "", fmt.Errorf("invalid archive id %q", archiveID)
	}

	archiveDir := filepath.Join(s.ProjectDir, config.StateDirName, config.ArchivesDirName, archiveID)
	dbPath = filepath.Join(archiveDir, config.DBFileName)
	objectsPath = filepath.Join(archiveDir, config.ObjectsDirName)
	if _, statErr := os.Stat(dbPath); statErr != nil {
		if os.IsNotExist(statErr) {
			return "", "", db.ErrNotFound
		}
		return "", "", fmt.Errorf("stat archive db: %w", statErr)
	}
	if _, statErr := os.Stat(objectsPath); statErr != nil {
		if os.IsNotExist(statErr) {
			return "", "", db.ErrNotFound
		}
		return "", "", fmt.Errorf("stat archive objects: %w", statErr)
	}
	return dbPath, objectsPath, nil
}

func (s *Service) IsArchiveInProgress() bool {
	lockPath := filepath.Join(s.ProjectDir, config.StateDirName, config.ArchiveLock)
	_, err := os.Stat(lockPath)
	return err == nil
}

func (s *Service) rotateStateToArchive(
	stateDir, dbPath, objectsPath, sha, branch, subject, committedAt string,
	cellCount int,
) (*ArchiveMeta, error) {
	archiveID := buildArchiveID(sha, time.Now().UTC())
	archivesDir := filepath.Join(stateDir, config.ArchivesDirName)
	if err := os.MkdirAll(archivesDir, 0o755); err != nil {
		return nil, fmt.Errorf("create archives dir: %w", err)
	}

	archiveDir := filepath.Join(archivesDir, archiveID)
	if _, err := os.Stat(archiveDir); err == nil {
		return nil, fmt.Errorf("archive %s already exists", archiveID)
	}

	stageDir := filepath.Join(archivesDir, "."+archiveID+".staging")
	if err := os.RemoveAll(stageDir); err != nil {
		return nil, fmt.Errorf("cleanup archive staging dir: %w", err)
	}
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		return nil, fmt.Errorf("create archive staging dir: %w", err)
	}

	stageDBPath := filepath.Join(stageDir, config.DBFileName)
	stageObjectsPath := filepath.Join(stageDir, config.ObjectsDirName)
	if err := os.Rename(dbPath, stageDBPath); err != nil {
		return nil, fmt.Errorf("move active db to staging: %w", err)
	}
	if err := os.Rename(objectsPath, stageObjectsPath); err != nil {
		_ = os.Rename(stageDBPath, dbPath)
		return nil, fmt.Errorf("move active objects to staging: %w", err)
	}

	archivedAt := time.Now().UTC().Format(time.RFC3339Nano)
	meta := ArchiveMeta{
		ArchiveID:   archiveID,
		CommitSHA:   sha,
		Branch:      branch,
		Subject:     subject,
		CommittedAt: committedAt,
		ArchivedAt:  archivedAt,
		CellCount:   cellCount,
	}

	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		_ = os.Rename(stageDBPath, dbPath)
		_ = os.Rename(stageObjectsPath, objectsPath)
		return nil, fmt.Errorf("marshal archive metadata: %w", err)
	}
	metaBytes = append(metaBytes, '\n')
	if err := os.WriteFile(filepath.Join(stageDir, config.ArchiveMetaFile), metaBytes, 0o644); err != nil {
		_ = os.Rename(stageDBPath, dbPath)
		_ = os.Rename(stageObjectsPath, objectsPath)
		return nil, fmt.Errorf("write archive metadata: %w", err)
	}

	if err := os.Rename(stageDir, archiveDir); err != nil {
		_ = os.Rename(stageDBPath, dbPath)
		_ = os.Rename(stageObjectsPath, objectsPath)
		return nil, fmt.Errorf("finalize archive directory: %w", err)
	}

	return &meta, nil
}

func (s *Service) createCommitBaselineCell(ctx context.Context, sha, subject string) (*db.Cell, error) {
	trackedPaths, err := gitTrackedPaths(ctx, s.ProjectDir)
	if err != nil {
		return nil, fmt.Errorf("resolve git-tracked files: %w", err)
	}

	manifest, err := s.Snapshot.CapturePaths(s.ProjectDir, trackedPaths)
	if err != nil {
		return nil, fmt.Errorf("capture commit baseline snapshot: %w", err)
	}

	message := fmt.Sprintf("baseline after commit %s", shortSHA(sha))
	if strings.TrimSpace(subject) != "" {
		message = fmt.Sprintf("%s: %s", message, subject)
	}

	cell, err := s.createCellFromManifest(ctx, manifest, SnapOptions{
		Message: message,
		Source:  gitCommitBaselineSource,
		RunEval: false,
	}, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create commit baseline cell: %w", err)
	}
	return cell, nil
}

func (s *Service) writeArchiveLock() (func(), error) {
	stateDir := filepath.Join(s.ProjectDir, config.StateDirName)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, fmt.Errorf("create state dir for archive lock: %w", err)
	}
	lockPath := filepath.Join(stateDir, config.ArchiveLock)
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("archive is already in progress")
		}
		return nil, fmt.Errorf("create archive lock: %w", err)
	}
	if _, err := file.WriteString("archiving"); err != nil {
		_ = file.Close()
		_ = os.Remove(lockPath)
		return nil, fmt.Errorf("write archive lock: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(lockPath)
		return nil, fmt.Errorf("close archive lock: %w", err)
	}
	return func() {
		_ = os.Remove(lockPath)
	}, nil
}

func gitTrackedPaths(ctx context.Context, projectDir string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-files", "-z")
	cmd.Dir = projectDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git ls-files failed: %w: %s", err, strings.TrimSpace(string(out)))
	}

	raw := strings.Split(string(out), "\x00")
	paths := make([]string, 0, len(raw))
	for _, entry := range raw {
		path := strings.TrimSpace(entry)
		if path == "" {
			continue
		}
		paths = append(paths, filepath.ToSlash(path))
	}
	return paths, nil
}

func buildArchiveID(sha string, now time.Time) string {
	return fmt.Sprintf("a_%s_%s", now.UTC().Format("20060102T150405Z"), shortSHA(sha))
}

func shortSHA(sha string) string {
	sha = strings.TrimSpace(sha)
	if len(sha) <= 8 {
		return sha
	}
	return sha[:8]
}

func parseRFC3339NanoOrZero(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed
}
