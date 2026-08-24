package app

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunDiscoverWritesRepositoryJSON(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "service")
	runGit(t, root, "init", "-b", "main", repo)

	var output bytes.Buffer
	if err := Run([]string{"discover", root}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"name":"service"`) || !strings.Contains(output.String(), `"baseline_state":"baseline_unknown"`) {
		t.Fatalf("discover output = %s", output.String())
	}
}

func TestRunIndexWritesBaselineIndexSummary(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "service")
	runGit(t, root, "init", "-b", "main", repo)
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "Main.java"), []byte("class Main {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "initial")
	commit := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "HEAD"))
	runGit(t, repo, "remote", "add", "origin", "https://example.invalid/service.git")
	runGit(t, repo, "update-ref", "refs/remotes/origin/main", commit)
	runGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	t.Setenv("LOCALAPPDATA", t.TempDir())

	var output bytes.Buffer
	if err := Run([]string{"index", root}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"indexed_files":1`) {
		t.Fatalf("index output = %s", output.String())
	}
}

func TestRunStatusWritesIndexedFileCount(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "service")
	runGit(t, root, "init", "-b", "main", repo)
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "Main.java"), []byte("class Main {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "initial")
	commit := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "HEAD"))
	runGit(t, repo, "remote", "add", "origin", "https://example.invalid/service.git")
	runGit(t, repo, "update-ref", "refs/remotes/origin/main", commit)
	runGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	t.Setenv("LOCALAPPDATA", t.TempDir())
	if err := Run([]string{"index", root}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := Run([]string{"status", root}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"file_count":1`) {
		t.Fatalf("status output = %s", output.String())
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}
