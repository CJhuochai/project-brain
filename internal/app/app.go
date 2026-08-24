package app

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/CJhuochai/project-brain/internal/indexer"
	"github.com/CJhuochai/project-brain/internal/input"
	"github.com/CJhuochai/project-brain/internal/query"
	"github.com/CJhuochai/project-brain/internal/report"
	"github.com/CJhuochai/project-brain/internal/storage"
	"github.com/CJhuochai/project-brain/internal/workspace"
	"github.com/google/uuid"
)

func Run(args []string, output io.Writer) error {
	if len(args) < 2 {
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
	case "refresh":
		db, err := openDB(args[1])
		if err != nil {
			return err
		}
		defer db.Close()
		result, err := indexer.Refresh(args[1], db)
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
			result = append(result, statusRepository{Repository: repository, FileCount: record.FileCount, DiagnosticCount: record.DiagnosticCount, IndexedAt: record.IndexedAt})
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
	case "analyze":
		if len(args) < 3 {
			return usage()
		}
		db, err := openDB(args[1])
		if err != nil {
			return err
		}
		defer db.Close()
		if _, err := indexer.Refresh(args[1], db); err != nil {
			return err
		}
		source, err := input.Parse("", args[2:], "")
		if err != nil {
			return err
		}
		result, err := report.AnalyzeRequirement(db, source)
		if err != nil {
			return err
		}
		return json.NewEncoder(output).Encode(result)
	case "report":
		if len(args) != 3 {
			return usage()
		}
		db, err := openDB(args[1])
		if err != nil {
			return err
		}
		defer db.Close()
		result, err := db.Report(args[2])
		if err != nil {
			return err
		}
		return json.NewEncoder(output).Encode(result)
	case "feedback":
		if len(args) != 7 {
			return usage()
		}
		db, err := openDB(args[1])
		if err != nil {
			return err
		}
		defer db.Close()
		item := storage.Feedback{ID: uuid.NewString(), ReportID: args[2], SubjectKind: args[3], SubjectKey: args[4], Decision: args[5], Note: args[6]}
		if err := db.RecordFeedback(item); err != nil {
			return err
		}
		return json.NewEncoder(output).Encode(item)
	default:
		return usage()
	}
}

type statusRepository struct {
	workspace.Repository
	FileCount       int    `json:"file_count"`
	DiagnosticCount int    `json:"diagnostic_count"`
	IndexedAt       string `json:"indexed_at,omitempty"`
}

func openDB(root string) (*storage.DB, error) {
	workspaceDir, err := storage.WorkspaceDir(root)
	if err != nil {
		return nil, err
	}
	return storage.Open(workspaceDir)
}

func usage() error {
	return fmt.Errorf("usage: project-brain <discover|index|refresh|status> <workspace>; project-brain <search|trace|impact|report> <workspace> <target>; project-brain analyze <workspace> <local-input...>; project-brain feedback <workspace> <report-id> <kind> <key> <decision> <note>")
}
