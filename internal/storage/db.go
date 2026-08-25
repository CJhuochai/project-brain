package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
}

func Open(workspaceDir string) (*DB, error) {
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		return nil, err
	}
	return openSQLite(filepath.Join(workspaceDir, "index.sqlite"), true, true)
}

// OpenSnapshot opens a completed snapshot for querying or a staging snapshot
// for indexing. Completed snapshots are always opened read-only.
func OpenSnapshot(path string, writable bool) (*DB, error) {
	if writable {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
	}
	return openSQLite(path, writable, writable)
}

func openSQLite(path string, writable bool, initialize bool) (*DB, error) {
	dsn := path
	if !writable {
		dsn = fmt.Sprintf("file:%s?mode=ro", strings.ReplaceAll(filepath.ToSlash(path), "#", "%23"))
	}
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db := &DB{DB: database}
	if writable {
		if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;`); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	if initialize {
		if err := db.initialize(); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	return db, nil
}

func (db *DB) initialize() error {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	if version < 7 {
		if _, err := db.Exec(`DROP TABLE IF EXISTS diagnostics; DROP TABLE IF EXISTS repositories; DROP TABLE IF EXISTS edges; DROP TABLE IF EXISTS symbols; DROP TABLE IF EXISTS file_fts; DROP TABLE IF EXISTS files;`); err != nil {
			return err
		}
	}
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS files (
	  id INTEGER PRIMARY KEY,
  repository_id TEXT NOT NULL,
  path TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  content TEXT NOT NULL,
  UNIQUE (repository_id, path)
);
CREATE VIRTUAL TABLE IF NOT EXISTS file_fts USING fts5(repository_id, path, content);
CREATE TABLE IF NOT EXISTS repositories (
  repository_id TEXT PRIMARY KEY,
  baseline_branch TEXT NOT NULL,
  baseline_commit TEXT NOT NULL,
  baseline_state TEXT NOT NULL,
  indexed_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS symbols (
  id INTEGER PRIMARY KEY,
  repository_id TEXT NOT NULL,
  path TEXT NOT NULL,
  name TEXT NOT NULL,
  kind TEXT NOT NULL,
  line INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS symbols_name ON symbols(name);
CREATE TABLE IF NOT EXISTS edges (
  id INTEGER PRIMARY KEY,
  repository_id TEXT NOT NULL,
  path TEXT NOT NULL,
  source TEXT NOT NULL,
  target TEXT NOT NULL,
  kind TEXT NOT NULL,
  line INTEGER NOT NULL,
  confidence TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS edges_target ON edges(target);
CREATE INDEX IF NOT EXISTS edges_source ON edges(source);
CREATE TABLE IF NOT EXISTS diagnostics (id INTEGER PRIMARY KEY, repository_id TEXT NOT NULL, path TEXT NOT NULL, message TEXT NOT NULL, line INTEGER NOT NULL, confidence TEXT NOT NULL);
`)
	if err != nil {
		return err
	}
	if version < 8 {
		if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS reports (
  id TEXT PRIMARY KEY,
  created_at TEXT NOT NULL,
  baseline_snapshot TEXT NOT NULL,
  input_digest TEXT NOT NULL,
  report_json TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS feedback (
  id TEXT PRIMARY KEY,
  report_id TEXT NOT NULL,
  subject_kind TEXT NOT NULL,
  subject_key TEXT NOT NULL,
  decision TEXT NOT NULL,
  note TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS rules (
  id INTEGER PRIMARY KEY,
  kind TEXT NOT NULL,
  pattern TEXT NOT NULL,
  target TEXT NOT NULL,
  confidence TEXT NOT NULL,
  note TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(kind, pattern, target)
);
PRAGMA user_version = 8;
`); err != nil {
			return err
		}
	}
	return nil
}
