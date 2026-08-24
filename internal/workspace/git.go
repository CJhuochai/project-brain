package workspace

import (
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
)

func discoverRepository(path string) (Repository, error) {
	repository := Repository{Name: filepath.Base(path), Path: path, BaselineState: BaselineUnknown}
	if remote, err := gitOutput(path, "remote", "get-url", "origin"); err == nil {
		repository.RemoteURL = sanitizeRemoteURL(remote)
	}
	ref, err := gitOutput(path, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD")
	if err != nil {
		return repository, nil
	}
	commit, err := gitOutput(path, "rev-parse", "--verify", ref)
	if err != nil {
		return repository, nil
	}
	repository.BaselineBranch = strings.TrimPrefix(ref, "refs/remotes/origin/")
	repository.BaselineCommit = commit
	repository.BaselineState = BaselineKnown
	return repository, nil
}

func sanitizeRemoteURL(remote string) string {
	parsed, err := url.Parse(remote)
	if err != nil || parsed.User == nil {
		return remote
	}
	parsed.User = nil
	return parsed.String()
}

func gitOutput(path string, args ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", path}, args...)...)
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
