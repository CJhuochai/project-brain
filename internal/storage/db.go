package storage

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
}

func Open(workspaceDir string) (*DB, error) {
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		return nil, err
	}
	database, err := sql.Open("sqlite", filepath.Join(workspaceDir, "index.sqlite"))
	if err != nil {
		return nil, err
	}
	db := &DB{DB: database}
	if err := db.initialize(); err != nil {
		_ = db.Close()
		return nil, err
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
PRAGMA user_version = 7;
`)
	return err
}
