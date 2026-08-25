package indexer

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/CJhuochai/project-brain/internal/extract"
	"github.com/CJhuochai/project-brain/internal/storage"
	"github.com/CJhuochai/project-brain/internal/workspace"
)

type Result struct {
	Repositories int `json:"repositories"`
	IndexedFiles int `json:"indexed_files"`
	ChangedFiles int `json:"changed_files"`
}

type RefreshResult struct {
	IndexState   string             `json:"index_state"`
	Repositories []RepositoryStatus `json:"repositories"`
	Result
}

type RepositoryStatus struct {
	workspace.Repository
	IndexState      string `json:"index_state"`
	StaleReason     string `json:"stale_reason,omitempty"`
	FileCount       int    `json:"file_count"`
	DiagnosticCount int    `json:"diagnostic_count"`
	IndexedAt       string `json:"indexed_at,omitempty"`
}

func Refresh(root string, db *storage.DB) (RefreshResult, error) {
	repositories, err := workspace.Discover(root)
	if err != nil {
		return RefreshResult{}, err
	}
	result := RefreshResult{IndexState: "baseline_unknown", Repositories: make([]RepositoryStatus, 0, len(repositories))}
	known := false
	for _, repository := range repositories {
		if repository.BaselineState != workspace.BaselineKnown {
			result.Repositories = append(result.Repositories, RepositoryStatus{Repository: repository, IndexState: "baseline_unknown", StaleReason: "未识别远程默认主分支"})
			continue
		}
		known = true
		record, err := db.RepositoryRecord(repository.Path)
		if err != nil {
			return RefreshResult{}, err
		}
		state := "up_to_date"
		if record.BaselineCommit != repository.BaselineCommit {
			indexed, changed, err := indexRepository(repository, db)
			if err != nil {
				return RefreshResult{}, err
			}
			record, err = db.RepositoryRecord(repository.Path)
			if err != nil {
				return RefreshResult{}, err
			}
			result.Result.Repositories++
			result.IndexedFiles += indexed
			result.ChangedFiles += changed
			result.IndexState = "refreshed"
			state = "refreshed"
		}
		result.Repositories = append(result.Repositories, RepositoryStatus{Repository: repository, IndexState: state, FileCount: record.FileCount, DiagnosticCount: record.DiagnosticCount, IndexedAt: record.IndexedAt})
	}
	if known && result.IndexState != "refreshed" {
		result.IndexState = "up_to_date"
	}
	return result, nil
}

// Status reads one completed snapshot without triggering an index write.
func Status(root string, db *storage.DB) (RefreshResult, error) {
	repositories, err := workspace.Discover(root)
	if err != nil {
		return RefreshResult{}, err
	}
	result := RefreshResult{IndexState: "baseline_unknown", Repositories: make([]RepositoryStatus, 0, len(repositories))}
	known := false
	for _, repository := range repositories {
		if repository.BaselineState != workspace.BaselineKnown {
			result.Repositories = append(result.Repositories, RepositoryStatus{Repository: repository, IndexState: "baseline_unknown", StaleReason: "未识别远程默认主分支"})
			continue
		}
		known = true
		record, err := db.RepositoryRecord(repository.Path)
		if err != nil {
			return RefreshResult{}, err
		}
		state := "up_to_date"
		if record.BaselineCommit != repository.BaselineCommit {
			state = "stale"
		}
		result.Repositories = append(result.Repositories, RepositoryStatus{Repository: repository, IndexState: state, FileCount: record.FileCount, DiagnosticCount: record.DiagnosticCount, IndexedAt: record.IndexedAt})
	}
	if known {
		result.IndexState = "up_to_date"
	}
	return result, nil
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
		indexed, changed, err := indexRepository(repository, db)
		if err != nil {
			return Result{}, err
		}
		result.IndexedFiles += indexed
		result.ChangedFiles += changed
	}
	return result, nil
}

func indexRepository(repository workspace.Repository, db *storage.DB) (int, int, error) {
	transaction, err := db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = transaction.Rollback() }()
	command := exec.Command("git", "-C", repository.Path, "archive", "--format=tar", repository.BaselineCommit)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return 0, 0, err
	}
	if err := command.Start(); err != nil {
		return 0, 0, err
	}
	reader := tar.NewReader(stdout)
	indexed, changed := 0, 0
	paths := map[string]bool{}
	for {
		header, nextErr := reader.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			_ = command.Wait()
			return 0, 0, fmt.Errorf("read archive %s: %w", repository.Name, nextErr)
		}
		if header.Typeflag != tar.TypeReg || !isSupported(header.Name) {
			continue
		}
		paths[header.Name] = true
		content, readErr := io.ReadAll(reader)
		if readErr != nil {
			_ = command.Wait()
			return 0, 0, readErr
		}
		fileChanged, upsertErr := db.UpsertFileTx(transaction, repository.Path, header.Name, content)
		if upsertErr != nil {
			_ = command.Wait()
			return 0, 0, upsertErr
		}
		indexed++
		if fileChanged {
			if err := db.ReplaceEvidenceTx(transaction, repository.Path, header.Name, extract.File(header.Name, content)); err != nil {
				_ = command.Wait()
				return 0, 0, err
			}
			changed++
		}
	}
	if _, err := io.Copy(io.Discard, stdout); err != nil {
		_ = command.Wait()
		return 0, 0, err
	}
	if err := command.Wait(); err != nil {
		return 0, 0, err
	}
	if err := db.DeleteFilesNotInTx(transaction, repository.Path, paths); err != nil {
		return 0, 0, err
	}
	if err := transaction.Commit(); err != nil {
		return 0, 0, err
	}
	if err := db.RecordIndex(repository.Path, repository.BaselineBranch, repository.BaselineCommit, string(repository.BaselineState)); err != nil {
		return 0, 0, err
	}
	return indexed, changed, nil
}

func readBaselineFile(repositoryPath, commit, path string) ([]byte, error) {
	return gitBytes(repositoryPath, "cat-file", "blob", commit+":"+path)
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
	for _, extension := range []string{".java", ".xml", ".yml", ".yaml", ".properties", ".sql", ".md", ".json", ".ts", ".tsx", ".js", ".vue", ".html"} {
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
