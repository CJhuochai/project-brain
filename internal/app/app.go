package app

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/CJhuochai/project-brain/internal/indexer"
	"github.com/CJhuochai/project-brain/internal/storage"
	"github.com/CJhuochai/project-brain/internal/workspace"
)

func Run(args []string, output io.Writer) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: project-brain <discover|index> <workspace>")
	}
	switch args[0] {
	case "discover":
		repositories, err := workspace.Discover(args[1])
		if err != nil {
			return err
		}
		return json.NewEncoder(output).Encode(repositories)
	case "index":
		workspaceDir, err := storage.WorkspaceDir(args[1])
		if err != nil {
			return err
		}
		db, err := storage.Open(workspaceDir)
		if err != nil {
			return err
		}
		defer db.Close()
		result, err := indexer.Index(args[1], db)
		if err != nil {
			return err
		}
		return json.NewEncoder(output).Encode(result)
	default:
		return fmt.Errorf("usage: project-brain <discover|index> <workspace>")
	}
}
