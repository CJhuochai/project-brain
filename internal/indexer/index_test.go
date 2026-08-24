package indexer

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CJhuochai/project-brain/internal/storage"
)

func TestIndexUsesBaselineCommitInsteadOfWorkingTree(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "service")
	runGit(t, root, "init", "-b", "main", repo)
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	if err := os.MkdirAll(filepath.Join(repo, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(repo, "src", "EntryController.java")
	if err := os.WriteFile(file, []byte("class BaselineEntryController {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "baseline")
	commit := runGit(t, repo, "rev-parse", "HEAD")
	runGit(t, repo, "remote", "add", "origin", "https://example.invalid/service.git")
	runGit(t, repo, "update-ref", "refs/remotes/origin/main", commit)
	runGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	if err := os.WriteFile(file, []byte("class WorkingTreeOnly {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	db, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	result, err := Index(root, db)
	if err != nil {
		t.Fatal(err)
	}
	if result.IndexedFiles != 1 || result.Repositories != 1 {
		t.Fatalf("result = %#v", result)
	}
	if paths, _ := db.SearchFiles("BaselineEntryController"); len(paths) != 1 {
		t.Fatalf("baseline content was not indexed: %#v", paths)
	}
	if paths, _ := db.SearchFiles("WorkingTreeOnly"); len(paths) != 0 {
		t.Fatalf("working tree content leaked into index: %#v", paths)
	}
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
