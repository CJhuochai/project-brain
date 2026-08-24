package storage

import "database/sql"

type RepositoryRecord struct {
	FileCount int    `json:"file_count"`
	IndexedAt string `json:"indexed_at,omitempty"`
}

func (db *DB) RecordIndex(repositoryID, branch, commit, state string) error {
	_, err := db.Exec(`INSERT INTO repositories(repository_id, baseline_branch, baseline_commit, baseline_state, indexed_at)
VALUES(?, ?, ?, ?, datetime('now'))
ON CONFLICT(repository_id) DO UPDATE SET baseline_branch=excluded.baseline_branch, baseline_commit=excluded.baseline_commit, baseline_state=excluded.baseline_state, indexed_at=excluded.indexed_at`, repositoryID, branch, commit, state)
	return err
}

func (db *DB) RepositoryRecord(repositoryID string) (RepositoryRecord, error) {
	var record RepositoryRecord
	err := db.QueryRow(`SELECT (SELECT count(*) FROM files WHERE repository_id = ?), indexed_at FROM repositories WHERE repository_id = ?`, repositoryID, repositoryID).Scan(&record.FileCount, &record.IndexedAt)
	if err == sql.ErrNoRows {
		return RepositoryRecord{}, nil
	}
	return record, err
}
