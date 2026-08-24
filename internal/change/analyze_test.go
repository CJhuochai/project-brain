package change

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilesReadsLocalCommitRangeWithoutWorkingTree(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "entry-service")
	runGit(t, root, "init", "-b", "main", repo)
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "EntryController.java"), []byte("class EntryController {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "first")
	base := runGit(t, repo, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(repo, "EntryController.java"), []byte("class EntryController { void submit() {} }"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "second")
	target := runGit(t, repo, "rev-parse", "HEAD")
	runGit(t, repo, "remote", "add", "origin", "https://example.invalid/entry.git")
	runGit(t, repo, "update-ref", "refs/remotes/origin/main", target)
	runGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")

	files, err := Files(root, base+".."+target)
	if err != nil || len(files) != 1 || files[0].Repository != "entry-service" || files[0].Path != "EntryController.java" {
		t.Fatalf("files=%#v err=%v", files, err)
	}
}

func TestFilesReadsSingleLocalCommit(t *testing.T) {
	root, _, commit := changeFixture(t)
	files, err := Files(root, commit)
	if err != nil || len(files) != 1 || files[0].Repository != "entry-service" || files[0].Path != "EntryController.java" {
		t.Fatalf("files=%#v err=%v", files, err)
	}
}

func changeFixture(t *testing.T) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	repo := filepath.Join(root, "entry-service")
	runGit(t, root, "init", "-b", "main", repo)
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "EntryController.java"), []byte("class EntryController {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "first")
	commit := runGit(t, repo, "rev-parse", "HEAD")
	runGit(t, repo, "remote", "add", "origin", "https://example.invalid/entry.git")
	runGit(t, repo, "update-ref", "refs/remotes/origin/main", commit)
	runGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	return root, repo, commit
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	output, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}
