package coordinator

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/CJhuochai/project-brain/internal/query"
	"github.com/CJhuochai/project-brain/internal/storage"
)

// Break caught: giving each client its own coordinator endpoint would restore
// the multiple-writer shape that caused the original SQLite lock failures.
func TestIndependentClientsUseOneCoordinatorAndSnapshot(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", t.TempDir())
	server, err := Start(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })

	responses := make(chan Response, 3)
	errors := make(chan error, 3)
	var group sync.WaitGroup
	for range 3 {
		group.Add(1)
		go func() {
			defer group.Done()
			client, err := Connect(root)
			if err != nil {
				errors <- err
				return
			}
			response, err := client.Call(context.Background(), Request{Operation: "health"})
			if err != nil {
				errors <- err
				return
			}
			responses <- response
		}()
	}
	group.Wait()
	close(errors)
	close(responses)
	for err := range errors {
		t.Fatal(err)
	}
	var snapshot string
	for response := range responses {
		if response.Meta.SnapshotID == "" {
			t.Fatalf("missing snapshot metadata: %#v", response)
		}
		if snapshot == "" {
			snapshot = response.Meta.SnapshotID
		}
		if response.Meta.SnapshotID != snapshot || response.Meta.RefreshTaskCount != 1 {
			t.Fatalf("response=%#v snapshot=%q", response, snapshot)
		}
	}
}

// Break caught: concurrent latest callers must wait on one staging refresh;
// otherwise separate refreshes again race for a workspace writer.
func TestLatestRequestsCoalesceToOneRefreshedSnapshot(t *testing.T) {
	root := coordinatorRepository(t)
	t.Setenv("LOCALAPPDATA", t.TempDir())
	server, err := Start(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	client, err := Connect(root)
	if err != nil {
		t.Fatal(err)
	}
	responses := make(chan Response, 3)
	errors := make(chan error, 3)
	for range 3 {
		go func() {
			response, err := client.Call(context.Background(), Request{Operation: "find_business_context", Arguments: map[string]any{"text": "Initial"}, Freshness: "latest"})
			if err != nil {
				errors <- err
				return
			}
			responses <- response
		}()
	}
	var snapshot string
	for range 3 {
		select {
		case err := <-errors:
			t.Fatal(err)
		case response := <-responses:
			if response.Meta.SnapshotID == "snapshot-001" || response.Meta.RefreshTaskCount != 1 {
				t.Fatalf("response=%#v", response)
			}
			if snapshot == "" {
				snapshot = response.Meta.SnapshotID
			}
			if response.Meta.SnapshotID != snapshot {
				t.Fatalf("response used a second snapshot: %#v", response)
			}
			var items query.SearchResult
			if err := json.Unmarshal(response.Result, &items); err != nil || len(items.Evidence) == 0 {
				t.Fatalf("items=%#v err=%v", items, err)
			}
		}
	}
}

func TestStableReturnsCompleteActiveSnapshotWhileRefreshRuns(t *testing.T) {
	root := coordinatorRepository(t)
	t.Setenv("LOCALAPPDATA", t.TempDir())
	server, err := Start(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	client, err := Connect(root)
	if err != nil {
		t.Fatal(err)
	}
	stable, err := client.Call(context.Background(), Request{Operation: "find_business_context", Arguments: map[string]any{"text": "Initial"}, Freshness: "stable"})
	if err != nil {
		t.Fatal(err)
	}
	if stable.Meta.SnapshotID != "snapshot-001" || stable.Meta.Freshness != "refreshing" {
		t.Fatalf("stable=%#v", stable)
	}
	latest, err := client.Call(context.Background(), Request{Operation: "find_business_context", Arguments: map[string]any{"text": "Initial"}, Freshness: "latest"})
	if err != nil {
		t.Fatal(err)
	}
	if latest.Meta.SnapshotID == "snapshot-001" || latest.Meta.Freshness != "up_to_date" {
		t.Fatalf("latest=%#v", latest)
	}
}

func coordinatorRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	repository := filepath.Join(root, "service")
	runCoordinatorGit(t, root, "init", "-b", "main", repository)
	runCoordinatorGit(t, repository, "config", "user.email", "test@example.com")
	runCoordinatorGit(t, repository, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repository, "Initial.java"), []byte("class Initial {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCoordinatorGit(t, repository, "add", ".")
	runCoordinatorGit(t, repository, "commit", "-m", "initial")
	commit := runCoordinatorGit(t, repository, "rev-parse", "HEAD")
	runCoordinatorGit(t, repository, "remote", "add", "origin", "https://example.invalid/service.git")
	runCoordinatorGit(t, repository, "update-ref", "refs/remotes/origin/main", commit)
	runCoordinatorGit(t, repository, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	return root
}

func runCoordinatorGit(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, arguments...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
	return strings.TrimSpace(string(output))
}

// Break caught: a caller should reuse the healthy workspace coordinator
// instead of attempting another writer start for every MCP process.
func TestEnsureReusesHealthyCoordinator(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", t.TempDir())
	server, err := Start(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	client, err := Ensure(root, "")
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Call(context.Background(), Request{Operation: "health"})
	if err != nil || response.Meta.SnapshotID == "" {
		t.Fatalf("response=%#v err=%v", response, err)
	}
}

func TestStartRemovesInterruptedStagingFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", t.TempDir())
	workspaceDir, err := storage.WorkspaceDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	staging := filepath.Join(workspaceDir, "snapshot-003.sqlite.staging")
	if err := os.WriteFile(staging, []byte("interrupted"), 0o600); err != nil {
		t.Fatal(err)
	}
	server, err := Start(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	if _, err := os.Stat(staging); !os.IsNotExist(err) {
		t.Fatalf("staging remains: %v", err)
	}
}
