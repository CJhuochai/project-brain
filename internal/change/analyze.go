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
	Range     string           `json:"range"`
	Files     []File           `json:"files"`
	Evidence  []query.Evidence `json:"evidence"`
	Risks     []string         `json:"risks,omitempty"`
	Questions []string         `json:"questions,omitempty"`
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
		if !strings.Contains(revision, "..") && strings.Contains(strings.ReplaceAll(revision, "\\", "/"), "/") {
			path := strings.TrimPrefix(strings.ReplaceAll(revision, "\\", "/"), repository.Name+"/")
			if err := exec.Command("git", "-C", repository.Path, "cat-file", "-e", repository.BaselineCommit+":"+path).Run(); err == nil {
				files = append(files, File{Repository: repository.Name, RepositoryID: repository.Path, Path: path})
			}
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
	if len(report.Evidence) == 0 {
		report.Questions = append(report.Questions, "变更文件尚无索引证据，请先刷新对应仓库基线索引")
	}
	repositories := map[string]bool{}
	for _, file := range files {
		repositories[file.Repository] = true
	}
	if len(repositories) > 1 {
		report.Risks = append(report.Risks, "变更跨多个仓库，需要确认版本发布顺序与接口兼容性")
	}
	for _, evidence := range report.Evidence {
		if evidence.Kind == "queries_table" {
			report.Risks = append(report.Risks, "变更涉及数据表，请确认迁移、回滚与历史数据兼容性")
			break
		}
	}
	return report, nil
}
