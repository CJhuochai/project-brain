package storage

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
)

func (db *DB) DeleteFilesNotInTx(transaction *sql.Tx, repositoryID string, paths map[string]bool) error {
	rows, err := transaction.Query(`SELECT id, path FROM files WHERE repository_id = ?`, repositoryID)
	if err != nil {
		return err
	}
	var stale []struct {
		id   int64
		path string
	}
	for rows.Next() {
		var item struct {
			id   int64
			path string
		}
		if err := rows.Scan(&item.id, &item.path); err != nil {
			_ = rows.Close()
			return err
		}
		if !paths[item.path] {
			stale = append(stale, item)
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range stale {
		if _, err := transaction.Exec(`DELETE FROM file_fts WHERE rowid = ?`, item.id); err != nil {
			return err
		}
		if _, err := transaction.Exec(`DELETE FROM files WHERE id = ?`, item.id); err != nil {
			return err
		}
		if _, err := transaction.Exec(`DELETE FROM symbols WHERE repository_id = ? AND path = ?`, repositoryID, item.path); err != nil {
			return err
		}
		if _, err := transaction.Exec(`DELETE FROM edges WHERE repository_id = ? AND path = ?`, repositoryID, item.path); err != nil {
			return err
		}
		if _, err := transaction.Exec(`DELETE FROM diagnostics WHERE repository_id = ? AND path = ?`, repositoryID, item.path); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) UpsertFile(repositoryID, path string, content []byte) (bool, error) {
	transaction, err := db.Begin()
	if err != nil {
		return false, err
	}
	defer func() { _ = transaction.Rollback() }()
	changed, err := db.UpsertFileTx(transaction, repositoryID, path, content)
	if err != nil {
		return false, err
	}
	if err := transaction.Commit(); err != nil {
		return false, err
	}
	return changed, nil
}

func (db *DB) UpsertFileTx(transaction *sql.Tx, repositoryID, path string, content []byte) (bool, error) {
	hash := sha256.Sum256(content)
	contentHash := hex.EncodeToString(hash[:])
	var currentHash string
	var fileID int64
	err := transaction.QueryRow(`SELECT id, content_hash FROM files WHERE repository_id = ? AND path = ?`, repositoryID, path).Scan(&fileID, &currentHash)
	if err == nil && currentHash == contentHash {
		return false, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	if err == sql.ErrNoRows {
		result, insertErr := transaction.Exec(`INSERT INTO files(repository_id, path, content_hash, content) VALUES(?, ?, ?, ?)`, repositoryID, path, contentHash, string(content))
		if insertErr != nil {
			return false, insertErr
		}
		fileID, err = result.LastInsertId()
		if err != nil {
			return false, err
		}
	} else {
		if _, err := transaction.Exec(`UPDATE files SET content_hash = ?, content = ? WHERE id = ?`, contentHash, string(content), fileID); err != nil {
			return false, err
		}
	}
	if _, err := transaction.Exec(`DELETE FROM file_fts WHERE rowid = ?`, fileID); err != nil {
		return false, err
	}
	if _, err := transaction.Exec(`INSERT INTO file_fts(rowid, repository_id, path, content) VALUES(?, ?, ?, ?)`, fileID, repositoryID, path, string(content)); err != nil {
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
