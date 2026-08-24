package storage

import (
	"testing"

	"github.com/CJhuochai/project-brain/internal/extract"
)

func TestUpsertFileSkipsUnchangedContentAndIndexesSearchText(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	changed, err := db.UpsertFile("repo-1", "src/StudentEntryApplication.java", []byte("class StudentEntryApplication {}"))
	if err != nil || !changed {
		t.Fatalf("first upsert changed=%v err=%v, want true nil", changed, err)
	}
	changed, err = db.UpsertFile("repo-1", "src/StudentEntryApplication.java", []byte("class StudentEntryApplication {}"))
	if err != nil || changed {
		t.Fatalf("same content upsert changed=%v err=%v, want false nil", changed, err)
	}

	paths, err := db.SearchFiles("StudentEntryApplication")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "src/StudentEntryApplication.java" {
		t.Fatalf("search paths = %#v", paths)
	}
	changed, err = db.UpsertFile("repo-2", "src/StudentEntryApplication.java", []byte("class StudentEntryApplication {}"))
	if err != nil || !changed {
		t.Fatalf("second repository upsert changed=%v err=%v, want true nil", changed, err)
	}
	paths, err = db.SearchFiles("StudentEntryApplication")
	if err != nil || len(paths) != 2 {
		t.Fatalf("same path from two repositories must remain searchable: paths=%#v err=%v", paths, err)
	}
}

func TestReplaceEvidenceTxStoresSourceLine(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	result := extract.Result{Symbols: []extract.Symbol{{Name: "com.example.EntryController", Kind: "controller", Line: 8}}, Edges: []extract.Edge{{Source: "com.example.EntryController", Target: "/entry", Kind: "route", Line: 9, Confidence: extract.Certain}}}
	if err := db.ReplaceEvidenceTx(tx, "repo", "EntryController.java", result); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var line int
	if err := db.QueryRow(`SELECT line FROM edges WHERE target = '/entry'`).Scan(&line); err != nil || line != 9 {
		t.Fatalf("line=%d err=%v", line, err)
	}
}

func TestUpsertFileTxKeepsMultipleFilesSearchableAfterCommit(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertFileTx(tx, "repo", "A.java", []byte("class Alpha {}")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertFileTx(tx, "repo", "B.java", []byte("class Beta {}")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if paths, err := db.SearchFiles("Alpha OR Beta"); err != nil || len(paths) != 2 {
		t.Fatalf("paths=%#v err=%v", paths, err)
	}
}

func TestRecordIndexReturnsFileCountAndTime(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.UpsertFile("repo", "A.java", []byte("class A {}")); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordIndex("repo", "main", "abc", "baseline_known"); err != nil {
		t.Fatal(err)
	}
	record, err := db.RepositoryRecord("repo")
	if err != nil || record.FileCount != 1 || record.IndexedAt == "" {
		t.Fatalf("record=%#v err=%v", record, err)
	}
}
