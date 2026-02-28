package config

import "time"

const (
	StateDirName   = ".converge"
	ObjectsDirName = "objects"
	DBFileName     = "converge.db"
	RestoreLock    = "restore.lock"
)

var IgnoredDirNames = map[string]struct{}{
	StateDirName:   {},
	".git":         {},
	"node_modules": {},
	"__pycache__":  {},
}

var IgnoredFileNames = map[string]struct{}{
	".DS_Store": {},
}

const DefaultWatchDebounce = 3 * time.Second
