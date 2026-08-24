package query

import (
	"fmt"
	"strings"

	"github.com/CJhuochai/project-brain/internal/storage"
)

type TraceResult struct {
	Target     string       `json:"target"`
	Candidates []Evidence   `json:"candidates,omitempty"`
	Paths      [][]Evidence `json:"paths,omitempty"`
}

type ImpactResult struct {
	Target    string     `json:"target"`
	Relations []Evidence `json:"relations,omitempty"`
}

func Trace(db *storage.DB, target string, maxDepth int) (TraceResult, error) {
	result := TraceResult{Target: target}
	starts, err := symbols(db, target)
	if err != nil {
		return result, err
	}
	if len(starts) == 0 && strings.HasPrefix(target, "/") {
		starts, err = sourcesForTarget(db, target)
		if err != nil {
			return result, err
		}
	}
	if len(starts) != 1 {
		result.Candidates = starts
		return result, nil
	}
	paths, err := walk(db, starts[0].Name, maxDepth, false)
	result.Paths = paths
	return result, err
}

func sourcesForTarget(db *storage.DB, target string) ([]Evidence, error) {
	rows, err := db.Query(`SELECT repository_id, path, line, source, kind, confidence FROM edges WHERE target = ? ORDER BY source`, target)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []Evidence
	seen := map[string]bool{}
	for rows.Next() {
		var item Evidence
		if err := rows.Scan(&item.Repository, &item.File, &item.Line, &item.Name, &item.Kind, &item.Confidence); err != nil {
			return nil, err
		}
		if !seen[item.Name] {
			results, seen[item.Name] = append(results, item), true
		}
	}
	return results, rows.Err()
}

func Impact(db *storage.DB, target string, maxDepth int) (ImpactResult, error) {
	result := ImpactResult{Target: target}
	name := target
	if matches, err := symbols(db, target); err != nil {
		return result, err
	} else if len(matches) == 1 {
		name = matches[0].Name
	}
	paths, err := walk(db, name, maxDepth, true)
	if err != nil {
		return result, err
	}
	for _, path := range paths {
		result.Relations = append(result.Relations, path...)
	}
	return result, nil
}

func symbols(db *storage.DB, target string) ([]Evidence, error) {
	rows, err := db.Query(`SELECT repository_id, path, line, name, kind, 'certain' FROM symbols WHERE name = ? OR name LIKE ? ORDER BY name`, target, "%."+target)
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

func walk(db *storage.DB, start string, maxDepth int, reverse bool) ([][]Evidence, error) {
	if maxDepth < 1 {
		return nil, fmt.Errorf("maxDepth must be positive")
	}
	var paths [][]Evidence
	var visit func(string, int, []Evidence) error
	visit = func(current string, depth int, path []Evidence) error {
		if depth == maxDepth {
			if len(path) > 0 {
				paths = append(paths, path)
			}
			return nil
		}
		edges, err := linkedEdges(db, current, reverse)
		if err != nil {
			return err
		}
		if len(edges) == 0 && len(path) > 0 {
			paths = append(paths, path)
		}
		for _, edge := range edges {
			next := edge.Target
			if reverse {
				next = edge.Source
			} else if matches, err := symbols(db, next); err != nil {
				return err
			} else if len(matches) == 1 {
				next = matches[0].Name
			}
			if seen(path, next) {
				continue
			}
			if err := visit(next, depth+1, append(path, edge)); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(start, 0, nil); err != nil {
		return nil, err
	}
	return paths, nil
}

func linkedEdges(db *storage.DB, current string, reverse bool) ([]Evidence, error) {
	column, value := "source", current
	if reverse {
		column, value = "target", shortName(current)
	}
	rows, err := db.Query(`SELECT repository_id, path, line, source, target, kind, confidence FROM edges WHERE `+column+` = ? ORDER BY repository_id, path, line`, value)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []Evidence
	for rows.Next() {
		var item Evidence
		if err := rows.Scan(&item.Repository, &item.File, &item.Line, &item.Source, &item.Target, &item.Kind, &item.Confidence); err != nil {
			return nil, err
		}
		item.Name = item.Target
		results = append(results, item)
	}
	return results, rows.Err()
}

func shortName(name string) string {
	if index := strings.LastIndex(name, "."); index >= 0 {
		return name[index+1:]
	}
	return name
}

func seen(path []Evidence, name string) bool {
	for _, item := range path {
		if item.Source == name || item.Target == name {
			return true
		}
	}
	return false
}
