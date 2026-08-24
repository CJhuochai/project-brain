package app

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/CJhuochai/project-brain/internal/indexer"
	"github.com/CJhuochai/project-brain/internal/query"
	"github.com/CJhuochai/project-brain/internal/storage"
	"github.com/CJhuochai/project-brain/internal/workspace"
)

func Run(args []string, output io.Writer) error {
	if len(args) < 2 || len(args) > 3 {
		return usage()
	}
	switch args[0] {
	case "discover":
		repositories, err := workspace.Discover(args[1])
		if err != nil {
			return err
		}
		return json.NewEncoder(output).Encode(repositories)
	case "index":
		db, err := openDB(args[1])
		if err != nil {
			return err
		}
		defer db.Close()
		result, err := indexer.Index(args[1], db)
		if err != nil {
			return err
		}
		return json.NewEncoder(output).Encode(result)
	case "status":
		db, err := openDB(args[1])
		if err != nil {
			return err
		}
		defer db.Close()
		repositories, err := workspace.Discover(args[1])
		if err != nil {
			return err
		}
		result := make([]statusRepository, 0, len(repositories))
		for _, repository := range repositories {
			record, err := db.RepositoryRecord(repository.Path)
			if err != nil {
				return err
			}
			result = append(result, statusRepository{Repository: repository, FileCount: record.FileCount, IndexedAt: record.IndexedAt})
		}
		return json.NewEncoder(output).Encode(result)
	case "search", "trace", "impact":
		if len(args) != 3 {
			return usage()
		}
		db, err := openDB(args[1])
		if err != nil {
			return err
		}
		defer db.Close()
		if args[0] == "search" {
			result, err := query.Search(db, args[2])
			if err != nil {
				return err
			}
			return json.NewEncoder(output).Encode(result)
		}
		if args[0] == "trace" {
			result, err := query.Trace(db, args[2], 6)
			if err != nil {
				return err
			}
			return json.NewEncoder(output).Encode(result)
		}
		result, err := query.Impact(db, args[2], 6)
		if err != nil {
			return err
		}
		return json.NewEncoder(output).Encode(result)
	default:
		return usage()
	}
}

type statusRepository struct {
	workspace.Repository
	FileCount int    `json:"file_count"`
	IndexedAt string `json:"indexed_at,omitempty"`
}

func openDB(root string) (*storage.DB, error) {
	workspaceDir, err := storage.WorkspaceDir(root)
	if err != nil {
		return nil, err
	}
	return storage.Open(workspaceDir)
}

func usage() error {
	return fmt.Errorf("usage: project-brain <discover|index|status> <workspace>; project-brain <search|trace|impact> <workspace> <target>")
}
