package storage

import (
	"os"
	"path/filepath"
	"testing"
)

// Break caught: deleting the v1.1 migration would leave existing reports,
// rules, and indexed files unavailable after v1.2 starts.
func TestOpenControlMigratesV11ReportsRulesAndIndex(t *testing.T) {
	directory := t.TempDir()
	legacy, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`INSERT INTO files(repository_id, path, content_hash, content) VALUES('service', 'Main.java', 'hash', 'class Main {}')`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.SaveReport(Report{ID: "r1", BaselineSnapshot: "service@a", InputDigest: "digest", JSON: `{"id":"r1"}`}); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`INSERT INTO rules(kind, pattern, target, confidence, note, created_at) VALUES('repository', 'service', 'confirmed', 'certain', 'ok', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	control, err := OpenControl(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close() })
	snapshot, err := control.ActiveSnapshot()
	if err != nil || snapshot.ID != "snapshot-001" {
		t.Fatalf("snapshot=%#v err=%v", snapshot, err)
	}
	if _, err := os.Stat(filepath.Join(directory, "index.sqlite")); !os.IsNotExist(err) {
		t.Fatalf("legacy index still exists: %v", err)
	}
	index, err := OpenSnapshot(snapshot.Path, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = index.Close() })
	var files int
	if err := index.QueryRow(`SELECT count(*) FROM files WHERE path = 'Main.java'`).Scan(&files); err != nil || files != 1 {
		t.Fatalf("files=%d err=%v", files, err)
	}
	if _, err := control.Report("r1"); err != nil {
		t.Fatal(err)
	}
	rules, revision, err := control.Rules("repository")
	if err != nil || len(rules) != 1 || revision != 1 {
		t.Fatalf("rules=%#v revision=%d err=%v", rules, revision, err)
	}

	again, err := OpenControl(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	rules, revision, err = again.Rules("repository")
	if err != nil || len(rules) != 1 || revision != 1 {
		t.Fatalf("repeated migration rules=%#v revision=%d err=%v", rules, revision, err)
	}
}

// Break caught: accidentally treating an immutable active snapshot as a
// writable index would reintroduce partial views during refresh.
func TestOpenSnapshotReadOnlyRejectsWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot-001.sqlite")
	writable, err := OpenSnapshot(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writable.Exec(`CREATE TABLE marker(value TEXT)`); err != nil {
		t.Fatal(err)
	}
	if err := writable.Close(); err != nil {
		t.Fatal(err)
	}
	readOnly, err := OpenSnapshot(path, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = readOnly.Close() })
	if _, err := readOnly.Exec(`INSERT INTO marker(value) VALUES('no')`); err == nil {
		t.Fatal("read-only snapshot accepted a write")
	}
}
