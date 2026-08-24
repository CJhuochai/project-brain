package change

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/CJhuochai/project-brain/internal/workspace"
)

type File struct {
	Repository string `json:"repository"`
	Path       string `json:"path"`
}

func Files(root, revision string) ([]File, error) {
	if !strings.Contains(revision, "..") {
		return nil, fmt.Errorf("变更范围必须是本地 Git commit 或 base..target")
	}
	repositories, err := workspace.Discover(root)
	if err != nil {
		return nil, err
	}
	var files []File
	for _, repository := range repositories {
		if repository.BaselineState != workspace.BaselineKnown {
			continue
		}
		output, diffErr := exec.Command("git", "-C", repository.Path, "diff", "--name-only", revision).Output()
		if diffErr != nil {
			continue
		}
		for _, path := range strings.Fields(string(output)) {
			files = append(files, File{Repository: repository.Name, Path: path})
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("未在工作区本地 Git 对象中找到变更范围：%s", revision)
	}
	return files, nil
}
