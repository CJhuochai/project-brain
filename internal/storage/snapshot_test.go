package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Break caught: mutating the active database instead of a copy would make an
// in-flight query observe a mixture of old and new index content.
func TestActivateStagingKeepsOldReaderOnOldSnapshot(t *testing.T) {
	control, active := controlWithIndexedSnapshot(t, "class Before {}")
	oldReader, err := OpenSnapshot(active.Path, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = oldReader.Close() })

	staging, discard, err := PrepareStaging(control)
	if err != nil {
		t.Fatal(err)
	}
	defer discard()
	writer, err := OpenSnapshot(staging.Path, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec(`UPDATE files SET content = 'class After {}'`); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := ActivateStaging(control, staging); err != nil {
		t.Fatal(err)
	}

	var oldContent string
	if err := oldReader.QueryRow(`SELECT content FROM files`).Scan(&oldContent); err != nil || oldContent != "class Before {}" {
		t.Fatalf("old=%q err=%v", oldContent, err)
	}
	current, err := control.ActiveSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	newReader, err := OpenSnapshot(current.Path, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = newReader.Close() })
	var newContent string
	if err := newReader.QueryRow(`SELECT content FROM files`).Scan(&newContent); err != nil || newContent != "class After {}" {
		t.Fatalf("new=%q err=%v", newContent, err)
	}
}

// Break caught: starting a staging copy without sufficient room could consume
// the active database's disk and make every existing query fail.
func TestPrepareStagingPreservesActiveSnapshotWhenSpaceIsInsufficient(t *testing.T) {
	control, active := controlWithIndexedSnapshot(t, "class Main {}")
	original := freeSpace
	freeSpace = func(string) (uint64, error) {
		return uint64(fileSize(t, active.Path))*2 + 128*1024*1024 - 1, nil
	}
	t.Cleanup(func() { freeSpace = original })
	if _, _, err := PrepareStaging(control); err == nil || !strings.Contains(err.Error(), "insufficient disk") {
		t.Fatalf("err=%v", err)
	}
	got, err := control.ActiveSnapshot()
	if err != nil || got.ID != active.ID {
		t.Fatalf("active=%#v err=%v", got, err)
	}
}

func controlWithIndexedSnapshot(t *testing.T, content string) (*Control, Snapshot) {
	t.Helper()
	directory := t.TempDir()
	control, err := OpenControl(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close() })
	path := filepath.Join(directory, "fixture.sqlite")
	index, err := OpenSnapshot(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := index.Exec(`INSERT INTO files(repository_id, path, content_hash, content) VALUES('service', 'Main.java', 'hash', ?)`, content); err != nil {
		t.Fatal(err)
	}
	if err := index.Close(); err != nil {
		t.Fatal(err)
	}
	active := Snapshot{ID: "snapshot-002", Path: filepath.Join(directory, "snapshot-002.sqlite")}
	if err := os.Rename(path, active.Path); err != nil {
		t.Fatal(err)
	}
	if err := control.ActivateSnapshot(active); err != nil {
		t.Fatal(err)
	}
	return control, active
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}
