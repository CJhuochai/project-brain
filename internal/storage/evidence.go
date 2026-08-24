package storage

import (
	"database/sql"

	"github.com/CJhuochai/project-brain/internal/extract"
)

func (db *DB) ReplaceEvidenceTx(transaction *sql.Tx, repositoryID, path string, result extract.Result) error {
	if _, err := transaction.Exec(`DELETE FROM symbols WHERE repository_id = ? AND path = ?`, repositoryID, path); err != nil {
		return err
	}
	if _, err := transaction.Exec(`DELETE FROM edges WHERE repository_id = ? AND path = ?`, repositoryID, path); err != nil {
		return err
	}
	if _, err := transaction.Exec(`DELETE FROM diagnostics WHERE repository_id = ? AND path = ?`, repositoryID, path); err != nil {
		return err
	}
	for _, symbol := range result.Symbols {
		if _, err := transaction.Exec(`INSERT INTO symbols(repository_id, path, name, kind, line) VALUES(?, ?, ?, ?, ?)`, repositoryID, path, symbol.Name, symbol.Kind, symbol.Line); err != nil {
			return err
		}
	}
	for _, edge := range result.Edges {
		if _, err := transaction.Exec(`INSERT INTO edges(repository_id, path, source, target, kind, line, confidence) VALUES(?, ?, ?, ?, ?, ?, ?)`, repositoryID, path, edge.Source, edge.Target, edge.Kind, edge.Line, edge.Confidence); err != nil {
			return err
		}
	}
	for _, diagnostic := range result.Diagnostics {
		if _, err := transaction.Exec(`INSERT INTO diagnostics(repository_id, path, message, line, confidence) VALUES(?, ?, ?, ?, ?)`, repositoryID, path, diagnostic.Message, diagnostic.Line, diagnostic.Confidence); err != nil {
			return err
		}
	}
	return nil
}
