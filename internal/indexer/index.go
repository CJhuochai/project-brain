package indexer

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/CJhuochai/project-brain/internal/storage"
	"github.com/CJhuochai/project-brain/internal/workspace"
)

type Result struct {
	Repositories int `json:"repositories"`
	IndexedFiles int `json:"indexed_files"`
	ChangedFiles int `json:"changed_files"`
}

func Index(root string, db *storage.DB) (Result, error) {
	repositories, err := workspace.Discover(root)
	if err != nil {
		return Result{}, err
	}
	result := Result{}
	for _, repository := range repositories {
		if repository.BaselineState != workspace.BaselineKnown {
			continue
		}
		result.Repositories++
		paths, err := baselinePaths(repository)
		if err != nil {
			return Result{}, err
		}
		for _, path := range paths {
			content, err := gitBytes(repository.Path, "show", repository.BaselineCommit+":"+path)
			if err != nil {
				return Result{}, err
			}
			changed, err := db.UpsertFile(repository.Path, path, content)
			if err != nil {
				return Result{}, err
			}
			result.IndexedFiles++
			if changed {
				result.ChangedFiles++
			}
		}
	}
	return result, nil
}

func baselinePaths(repository workspace.Repository) ([]string, error) {
	data, err := gitBytes(repository.Path, "ls-tree", "-r", "--name-only", "-z", repository.BaselineCommit)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, path := range bytes.Split(data, []byte{0}) {
		if len(path) == 0 || !isSupported(string(path)) {
			continue
		}
		paths = append(paths, string(path))
	}
	return paths, nil
}

func isSupported(path string) bool {
	name := strings.ToLower(filepath.Base(path))
	if name == "pom.xml" {
		return true
	}
	for _, extension := range []string{".java", ".xml", ".yml", ".yaml", ".properties", ".sql", ".md", ".json"} {
		if strings.HasSuffix(name, extension) {
			return true
		}
	}
	return false
}

func gitBytes(path string, args ...string) ([]byte, error) {
	command := exec.Command("git", append([]string{"-C", path}, args...)...)
	return command.Output()
}
