package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/prittamravi/converge/internal/config"
	"github.com/prittamravi/converge/internal/db"
)

func (s *Service) RestoreCell(ctx context.Context, targetID string) (*db.Cell, error) {
	if _, err := s.DB.GetCell(targetID); err != nil {
		if err == db.ErrNotFound {
			return nil, fmt.Errorf("cell %s not found", targetID)
		}
		return nil, err
	}

	activeBranch, err := s.ActiveBranch()
	if err != nil {
		return nil, err
	}

	latestBeforeRestore, err := s.DB.LatestCellByBranch(activeBranch)
	if err != nil {
		return nil, fmt.Errorf("latest cell before restore: %w", err)
	}

	safety, err := s.CreateCell(ctx, SnapOptions{
		Message: fmt.Sprintf("safety snapshot before restore to %s", targetID),
		Source:  "restore_safety",
		RunEval: false,
	})
	if err != nil {
		return nil, fmt.Errorf("create safety cell: %w", err)
	}

	cleanup, err := s.writeRestoreLock()
	if err != nil {
		return nil, fmt.Errorf("write restore lock: %w", err)
	}
	defer cleanup()

	if err := s.restoreTrackedFilesToCell(targetID, latestBeforeRestore); err != nil {
		return nil, err
	}

	targetCellID := targetID
	if err := s.DB.UpdateBranchHead(activeBranch, &targetCellID); err != nil {
		if err == db.ErrNotFound {
			if createErr := s.DB.CreateBranch(activeBranch, &targetCellID, safety.Timestamp); createErr != nil {
				return nil, fmt.Errorf("create missing active branch %s: %w", activeBranch, createErr)
			}
		} else {
			return nil, fmt.Errorf("update active branch head: %w", err)
		}
	}
	if err := s.setHeadCellMeta(&targetCellID); err != nil {
		return nil, err
	}

	return safety, nil
}

func (s *Service) restoreTrackedFilesToCell(targetID string, currentTrackedHead *db.Cell) error {
	targetManifest, err := s.DB.GetManifest(targetID)
	if err != nil {
		return fmt.Errorf("target manifest: %w", err)
	}

	currentTrackedManifest := make([]db.ManifestEntry, 0)
	if currentTrackedHead != nil {
		currentTrackedManifest, err = s.DB.GetManifest(currentTrackedHead.ID)
		if err != nil {
			return fmt.Errorf("current tracked manifest: %w", err)
		}
	}

	targetPaths := make(map[string]struct{}, len(targetManifest))
	for _, entry := range targetManifest {
		targetPaths[entry.Path] = struct{}{}
		data, err := s.Store.Read(entry.Hash)
		if err != nil {
			return fmt.Errorf("read object for %s: %w", entry.Path, err)
		}
		fullPath := filepath.Join(s.ProjectDir, entry.Path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return fmt.Errorf("mkdir for %s: %w", entry.Path, err)
		}
		if err := os.WriteFile(fullPath, data, os.FileMode(entry.Mode)); err != nil {
			return fmt.Errorf("write file %s: %w", entry.Path, err)
		}
	}

	for _, entry := range currentTrackedManifest {
		if _, exists := targetPaths[entry.Path]; exists {
			continue
		}
		fullPath := filepath.Join(s.ProjectDir, entry.Path)
		err := os.Remove(fullPath)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale tracked file %s: %w", entry.Path, err)
		}
	}

	return nil
}

func (s *Service) writeRestoreLock() (func(), error) {
	stateDir := filepath.Join(s.ProjectDir, config.StateDirName)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(stateDir, config.RestoreLock)
	if err := os.WriteFile(lockPath, []byte("restoring"), 0o644); err != nil {
		return nil, err
	}
	return func() {
		_ = os.Remove(lockPath)
	}, nil
}

func (s *Service) IsRestoreInProgress() bool {
	lockPath := filepath.Join(s.ProjectDir, config.StateDirName, config.RestoreLock)
	_, err := os.Stat(lockPath)
	return err == nil
}
