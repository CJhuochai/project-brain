package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverFindsRepositoriesAndReadsOnlyLocalRemoteHead(t *testing.T) {
	root := t.TempDir()
	known := createRepository(t, root, "known", true)
	createRepository(t, root, "unknown", false)
	if err := os.MkdirAll(filepath.Join(root, "known", "target", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}

	repos, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 {
		t.Fatalf("repository count = %d, want 2", len(repos))
	}

	byName := map[string]Repository{}
	for _, repo := range repos {
		byName[repo.Name] = repo
	}
	if got := byName["known"]; got.Path != known || got.RemoteURL != "https://example.invalid/known.git" || got.BaselineBranch != "main" || got.BaselineState != BaselineKnown || got.BaselineCommit == "" {
		t.Fatalf("known repository = %#v", got)
	}
	if got := byName["unknown"]; got.BaselineState != BaselineUnknown || got.BaselineBranch != "" {
		t.Fatalf("unknown repository = %#v", got)
	}
}

func createRepository(t *testing.T, root, name string, withBaseline bool) string {
	t.Helper()
	path := filepath.Join(root, name)
	runGit(t, root, "init", "-b", "main", path)
	runGit(t, path, "config", "user.email", "test@example.com")
	runGit(t, path, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(path, "README.md"), []byte(name), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, path, "add", "README.md")
	runGit(t, path, "commit", "-m", "initial")
	if withBaseline {
		runGit(t, path, "remote", "add", "origin", "https://user:password@example.invalid/"+name+".git")
		commit := runGit(t, path, "rev-parse", "HEAD")
		runGit(t, path, "update-ref", "refs/remotes/origin/main", commit)
		runGit(t, path, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	}
	return path
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}
