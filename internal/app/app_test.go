package app

import (
	"bytes"
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

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}
