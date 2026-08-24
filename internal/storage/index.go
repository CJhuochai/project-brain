package storage

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
)

func (db *DB) UpsertFile(repositoryID, path string, content []byte) (bool, error) {
	hash := sha256.Sum256(content)
	contentHash := hex.EncodeToString(hash[:])
	var currentHash string
	err := db.QueryRow(`SELECT content_hash FROM files WHERE repository_id = ? AND path = ?`, repositoryID, path).Scan(&currentHash)
	if err == nil && currentHash == contentHash {
		return false, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	transaction, err := db.Begin()
	if err != nil {
		return false, err
	}
	defer func() { _ = transaction.Rollback() }()
	if _, err := transaction.Exec(`INSERT INTO files(repository_id, path, content_hash, content) VALUES(?, ?, ?, ?)
ON CONFLICT(repository_id, path) DO UPDATE SET content_hash=excluded.content_hash, content=excluded.content`, repositoryID, path, contentHash, string(content)); err != nil {
		return false, err
	}
	if _, err := transaction.Exec(`DELETE FROM file_fts WHERE repository_id = ? AND path = ?`, repositoryID, path); err != nil {
		return false, err
	}
	if _, err := transaction.Exec(`INSERT INTO file_fts(repository_id, path, content) VALUES(?, ?, ?)`, repositoryID, path, string(content)); err != nil {
		return false, err
	}
	if err := transaction.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (db *DB) SearchFiles(text string) ([]string, error) {
	rows, err := db.Query(`SELECT path FROM file_fts WHERE file_fts MATCH ? ORDER BY rank`, text)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, rows.Err()
}
