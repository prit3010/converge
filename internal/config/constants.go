package config

import "time"

const (
	StateDirName       = ".converge"
	ObjectsDirName     = "objects"
	ArchivesDirName    = "archives"
	ArchiveMetaFile    = "meta.json"
	DBFileName         = "converge.db"
	RestoreLock        = "restore.lock"
	ArchiveLock        = "archive.lock"
	ConfigFileName     = "config.toml"
	IgnoreFileName     = ".convergeignore"
	DefaultJSONVersion = "v1"
)

const DefaultWatchDebounce = 3 * time.Second

var BuiltinIgnorePatterns = []string{
	StateDirName + "/",
	".git/",
	"node_modules/",
	"__pycache__/",
	".DS_Store",
}

const DefaultConvergeIgnoreTemplate = `# Converge ignore rules (gitignore-style)
# Secrets and local env
.env
.env.*
*.pem
*.key
*.p12
*.pfx
*.crt
*.cer

# Dependencies and virtual environments
node_modules/
.venv/
venv/
vendor/

# Build and cache artifacts
dist/
build/
out/
target/
.cache/
.pytest_cache/
.mypy_cache/
.ruff_cache/
.next/
.nuxt/

# Logs and local databases
*.log
*.db
*.sqlite
*.sqlite3

# Archives, media, and large generated assets
*.zip
*.tar
*.tar.gz
*.tgz
*.7z
*.mp4
*.mov
*.avi
*.mkv
*.mp3
*.wav
*.flac
*.png
*.jpg
*.jpeg
*.gif
*.webp
*.pdf
*.parquet
*.feather
*.npy
*.npz
`
