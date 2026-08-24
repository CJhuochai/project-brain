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
	if version < 2 {
		if _, err := db.Exec(`DROP TABLE IF EXISTS file_fts; DROP TABLE IF EXISTS files;`); err != nil {
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
PRAGMA user_version = 2;
`)
	return err
}
