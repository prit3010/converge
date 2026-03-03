package ui

import (
	"fmt"
	"strings"

	"github.com/prittamravi/converge/internal/core"
	"github.com/prittamravi/converge/internal/db"
	"github.com/prittamravi/converge/internal/store"
)

type dataSource struct {
	ArchiveID string
	ReadOnly  bool
	DB        *db.DB
	Store     *store.Store
	closeFn   func() error
}

func (d *dataSource) Close() error {
	if d == nil || d.closeFn == nil {
		return nil
	}
	return d.closeFn()
}

func archiveIDFromQuery(raw string) string {
	id := strings.TrimSpace(raw)
	if id == "" {
		return "current"
	}
	return id
}

func (s *Server) openDataSource(archiveID string) (*dataSource, error) {
	archiveID = archiveIDFromQuery(archiveID)
	if archiveID == "current" {
		return &dataSource{
			ArchiveID: "current",
			ReadOnly:  false,
			DB:        s.svc.DB,
			Store:     s.svc.Store,
			closeFn:   nil,
		}, nil
	}

	dbPath, objectsPath, err := s.svc.ArchiveStatePaths(archiveID)
	if err != nil {
		if err == db.ErrNotFound {
			return nil, db.ErrNotFound
		}
		return nil, fmt.Errorf("resolve archive source: %w", err)
	}

	archiveDB, err := db.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open archive db %s: %w", archiveID, err)
	}

	return &dataSource{
		ArchiveID: archiveID,
		ReadOnly:  true,
		DB:        archiveDB,
		Store:     store.New(objectsPath),
		closeFn:   archiveDB.Close,
	}, nil
}

func activeBranchForDB(database *db.DB) string {
	if database == nil {
		return "main"
	}
	branch, err := database.GetMeta("active_branch")
	if err != nil {
		return "main"
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return "main"
	}
	return branch
}

func toArchiveOptionJSON(meta core.ArchiveMeta) archiveJSON {
	return archiveJSON{
		ID:          meta.ArchiveID,
		Label:       fmt.Sprintf("%s (%s)", meta.ArchiveID, shortCommitLabel(meta.CommitSHA)),
		ReadOnly:    true,
		CommitSHA:   meta.CommitSHA,
		Branch:      meta.Branch,
		Subject:     meta.Subject,
		CommittedAt: meta.CommittedAt,
		ArchivedAt:  meta.ArchivedAt,
		CellCount:   meta.CellCount,
	}
}

func shortCommitLabel(sha string) string {
	sha = strings.TrimSpace(sha)
	if len(sha) <= 8 {
		return sha
	}
	return sha[:8]
}
