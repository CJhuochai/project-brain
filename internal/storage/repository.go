package storage

import "database/sql"

type RepositoryRecord struct {
	FileCount       int    `json:"file_count"`
	DiagnosticCount int    `json:"diagnostic_count"`
	BaselineCommit  string `json:"baseline_commit,omitempty"`
	IndexedAt       string `json:"indexed_at,omitempty"`
}

func (db *DB) RecordIndex(repositoryID, branch, commit, state string) error {
	_, err := db.Exec(`INSERT INTO repositories(repository_id, baseline_branch, baseline_commit, baseline_state, indexed_at)
VALUES(?, ?, ?, ?, datetime('now'))
ON CONFLICT(repository_id) DO UPDATE SET baseline_branch=excluded.baseline_branch, baseline_commit=excluded.baseline_commit, baseline_state=excluded.baseline_state, indexed_at=excluded.indexed_at`, repositoryID, branch, commit, state)
	return err
}

func (db *DB) RepositoryRecord(repositoryID string) (RepositoryRecord, error) {
	var record RepositoryRecord
	err := db.QueryRow(`SELECT (SELECT count(*) FROM files WHERE repository_id = ?), (SELECT count(*) FROM diagnostics WHERE repository_id = ?), baseline_commit, indexed_at FROM repositories WHERE repository_id = ?`, repositoryID, repositoryID, repositoryID).Scan(&record.FileCount, &record.DiagnosticCount, &record.BaselineCommit, &record.IndexedAt)
	if err == sql.ErrNoRows {
		return RepositoryRecord{}, nil
	}
	return record, err
}

func (db *DB) BaselineSnapshots() ([]string, error) {
	rows, err := db.Query(`SELECT repository_id, baseline_branch, baseline_commit, baseline_state FROM repositories ORDER BY repository_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var snapshots []string
	for rows.Next() {
		var repository, branch, commit, state string
		if err := rows.Scan(&repository, &branch, &commit, &state); err != nil {
			return nil, err
		}
		snapshots = append(snapshots, repository+"@"+branch+":"+commit+":"+state)
	}
	return snapshots, rows.Err()
}
