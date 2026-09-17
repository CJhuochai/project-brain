package coordinator

import (
	"crypto/sha256"
	"database/sql"
	"github.com/CJhuochai/project-brain/internal/storage"
	"os"
	"testing"
)

// Genuine v1 shape, unchanged Git baseline: schema change alone must refresh.
func TestV2UpgradeStagesReindexAndPreservesReport(t *testing.T) {
	root := coordinatorRepository(t)
	t.Setenv("LOCALAPPDATA", t.TempDir())
	server, err := Start(root)
	if err != nil {
		t.Fatal(err)
	}
	response := server.execute(Request{Operation: "workspace_status", Freshness: "latest"})
	if response.Error != "" {
		t.Fatal(response.Error)
	}
	active, err := server.control.ActiveSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err := server.control.SaveReport(storage.Report{ID: "v1-report", JSON: `{"id":"v1-report","evidence":[]}`}); err != nil {
		t.Fatal(err)
	}
	server.Close()
	db, err := sql.Open("sqlite", active.Path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`DROP TABLE contracts; DROP INDEX symbols_uid; DROP INDEX symbols_repo_name;
ALTER TABLE symbols DROP COLUMN uid; ALTER TABLE symbols DROP COLUMN signature; ALTER TABLE symbols DROP COLUMN end_line;
ALTER TABLE edges DROP COLUMN source_signature; ALTER TABLE edges DROP COLUMN target_arity; PRAGMA user_version=8; PRAGMA wal_checkpoint(TRUNCATE)`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	before, err := os.ReadFile(active.Path)
	if err != nil {
		t.Fatal(err)
	}
	server, err = Start(root)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	response = server.execute(Request{Operation: "find_business_context", Arguments: map[string]any{"text": "Initial"}, Freshness: "stable"})
	if response.Error != "" || response.Meta.SnapshotID == active.ID {
		t.Fatalf("upgrade=%#v", response)
	}
	after, err := os.ReadFile(active.Path)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("active legacy snapshot mutated")
	}
	saved, err := server.control.Report("v1-report")
	if err != nil || saved.JSON != `{"id":"v1-report","evidence":[]}` {
		t.Fatalf("saved=%#v err=%v", saved, err)
	}
	pinned := server.execute(Request{Operation: "get_symbol_context", Arguments: map[string]any{"text": "Initial"}, SnapshotID: active.ID})
	if pinned.Error == "" {
		t.Fatal("old pinned snapshot silently accepted")
	}
}

func TestV2FailedUpgradeKeepsActiveSnapshot(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", t.TempDir())
	server, err := Start(root)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	active, _ := server.control.ActiveSnapshot()
	db, err := sql.Open("sqlite", active.Path)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a corrupt v1 fixture: missing table makes staged migration fail.
	if _, err := db.Exec(`DROP TABLE edges; PRAGMA user_version=8; PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	before, _ := os.ReadFile(active.Path)
	response := server.execute(Request{Operation: "get_symbol_context", Arguments: map[string]any{"text": "Root"}, Freshness: "latest"})
	if response.Error == "" {
		t.Fatal("failed migration not surfaced")
	}
	selected, _ := server.control.ActiveSnapshot()
	after, _ := os.ReadFile(active.Path)
	if selected.ID != active.ID || sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("failed migration replaced or changed active snapshot")
	}
}
