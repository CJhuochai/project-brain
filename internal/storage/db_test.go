package storage

import (
	"path/filepath"
	"testing"
)

func TestOpenRebuildsPreviousExtractorSchema(t *testing.T) {
	directory := t.TempDir()
	db, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO files(repository_id, path, content_hash, content) VALUES('repo', 'Old.java', 'hash', 'old evidence'); PRAGMA user_version = 6`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(filepath.Clean(directory))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 7 {
		t.Fatalf("version=%d err=%v", version, err)
	}
	var files int
	if err := db.QueryRow(`SELECT count(*) FROM files`).Scan(&files); err != nil || files != 0 {
		t.Fatalf("files=%d err=%v", files, err)
	}
}
