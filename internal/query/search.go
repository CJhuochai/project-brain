package query

import (
	"strings"

	"github.com/CJhuochai/project-brain/internal/storage"
)

type Evidence struct {
	Repository string `json:"repository"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Confidence string `json:"confidence"`
	Source     string `json:"source,omitempty"`
	Target     string `json:"target,omitempty"`
}

func Search(db *storage.DB, text string) ([]Evidence, error) {
	needle := "%" + strings.ToLower(text) + "%"
	rows, err := db.Query(`
SELECT repository_id, path, line, name, kind, 'certain' FROM symbols WHERE lower(name) LIKE ?
UNION ALL
SELECT repository_id, path, line, target, kind, confidence FROM edges WHERE lower(target) LIKE ?
ORDER BY repository_id, path, line`, needle, needle)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []Evidence
	for rows.Next() {
		var item Evidence
		if err := rows.Scan(&item.Repository, &item.File, &item.Line, &item.Name, &item.Kind, &item.Confidence); err != nil {
			return nil, err
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	fileRows, err := db.Query(`SELECT repository_id, path, content FROM files WHERE lower(content) LIKE ? ORDER BY repository_id, path`, needle)
	if err != nil {
		return nil, err
	}
	defer fileRows.Close()
	for fileRows.Next() {
		var item Evidence
		var content string
		if err := fileRows.Scan(&item.Repository, &item.File, &content); err != nil {
			return nil, err
		}
		item.Name, item.Kind, item.Confidence = text, "text", "certain"
		item.Line = strings.Count(content[:strings.Index(strings.ToLower(content), strings.ToLower(text))], "\n") + 1
		results = append(results, item)
	}
	return results, fileRows.Err()
}

func FileEvidence(db *storage.DB, repository, path string) ([]Evidence, error) {
	rows, err := db.Query(`
SELECT repository_id, path, line, name, kind, 'certain' FROM symbols WHERE repository_id = ? AND path = ?
UNION ALL
SELECT repository_id, path, line, target, kind, confidence FROM edges WHERE repository_id = ? AND path = ?
ORDER BY line`, repository, path, repository, path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []Evidence
	for rows.Next() {
		var item Evidence
		if err := rows.Scan(&item.Repository, &item.File, &item.Line, &item.Name, &item.Kind, &item.Confidence); err != nil {
			return nil, err
		}
		results = append(results, item)
	}
	return results, rows.Err()
}
