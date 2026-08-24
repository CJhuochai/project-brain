package change

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/CJhuochai/project-brain/internal/query"
	"github.com/CJhuochai/project-brain/internal/storage"
	"github.com/CJhuochai/project-brain/internal/workspace"
)

type File struct {
	Repository   string `json:"repository"`
	Path         string `json:"path"`
	RepositoryID string `json:"-"`
}

type Report struct {
	Range    string           `json:"range"`
	Files    []File           `json:"files"`
	Evidence []query.Evidence `json:"evidence"`
}

func Files(root, revision string) ([]File, error) {
	repositories, err := workspace.Discover(root)
	if err != nil {
		return nil, err
	}
	var files []File
	for _, repository := range repositories {
		if repository.BaselineState != workspace.BaselineKnown {
			continue
		}
		args := []string{"-C", repository.Path, "show", "--format=", "--name-only", revision}
		if strings.Contains(revision, "..") {
			args = []string{"-C", repository.Path, "diff", "--name-only", revision}
		}
		output, diffErr := exec.Command("git", args...).Output()
		if diffErr != nil {
			continue
		}
		for _, path := range strings.Fields(string(output)) {
			files = append(files, File{Repository: repository.Name, RepositoryID: repository.Path, Path: path})
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("未在工作区本地 Git 对象中找到变更范围：%s", revision)
	}
	return files, nil
}

func Analyze(root string, db *storage.DB, revision string) (Report, error) {
	files, err := Files(root, revision)
	if err != nil {
		return Report{}, err
	}
	report := Report{Range: revision, Files: files}
	for _, file := range files {
		rows, queryErr := db.Query(`SELECT repository_id, path, line, name, kind, 'certain' FROM symbols WHERE repository_id = ? AND path = ?
UNION ALL SELECT repository_id, path, line, target, kind, confidence FROM edges WHERE repository_id = ? AND path = ?
ORDER BY path, line`, file.RepositoryID, file.Path, file.RepositoryID, file.Path)
		if queryErr != nil {
			return Report{}, queryErr
		}
		for rows.Next() {
			var item query.Evidence
			if err := rows.Scan(&item.Repository, &item.File, &item.Line, &item.Name, &item.Kind, &item.Confidence); err != nil {
				_ = rows.Close()
				return Report{}, err
			}
			report.Evidence = append(report.Evidence, item)
		}
		if err := rows.Close(); err != nil {
			return Report{}, err
		}
	}
	return report, nil
}
