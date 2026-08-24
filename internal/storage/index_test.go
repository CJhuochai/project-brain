package storage

import "testing"

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
