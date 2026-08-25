package indexer

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/CJhuochai/project-brain/internal/storage"
)

// This opt-in test measures one real-baseline incremental refresh without
// changing a business repository or the active Project Brain snapshot.
func TestRealWorkspaceIncrementalRefresh(t *testing.T) {
	root := os.Getenv("PROJECT_BRAIN_REAL_WORKSPACE")
	if root == "" {
		t.Skip("set PROJECT_BRAIN_REAL_WORKSPACE to run the local benchmark")
	}
	workspaceDir, err := storage.WorkspaceDir(root)
	if err != nil {
		t.Fatal(err)
	}
	control, err := storage.OpenControlRead(workspaceDir)
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	active, err := control.ActiveSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	probe := filepath.Join(t.TempDir(), "probe.sqlite")
	copySnapshot(t, active.Path, probe)
	db, err := storage.OpenSnapshot(probe, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM repositories WHERE repository_id = ?`, filepath.Join(root, "example-9160ee5f41")); err != nil {
		t.Fatal(err)
	}
	result, err := Refresh(root, db)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if result.Result.Repositories < 1 || result.IndexState != "refreshed" {
		t.Fatalf("expected at least one local probe refresh, got %#v", result)
	}
	t.Logf("incremental refresh: repositories=%d indexed_files=%d changed_files=%d", result.Result.Repositories, result.IndexedFiles, result.ChangedFiles)
}

func copySnapshot(t *testing.T, source, target string) {
	t.Helper()
	in, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.Create(target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}
