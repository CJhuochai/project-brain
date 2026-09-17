package storage

import "testing"

func TestReportsAndFeedbackPersistLocally(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	report := Report{ID: "report-1", BaselineSnapshot: "repo-a@abc", InputDigest: "digest", JSON: `{"id":"report-1"}`}
	if err := db.SaveReport(report); err != nil {
		t.Fatal(err)
	}
	got, err := db.Report("report-1")
	if err != nil || got.JSON != report.JSON || got.BaselineSnapshot != report.BaselineSnapshot {
		t.Fatalf("report=%#v err=%v", got, err)
	}

	if err := db.RecordFeedback(Feedback{ID: "feedback-1", ReportID: report.ID, SubjectKind: "change_point", SubjectKey: "repo-a:EntryService", Decision: "rejected", Note: "false positive"}); err != nil {
		t.Fatal(err)
	}
	rules, err := db.Rules("change_point")
	if err != nil || len(rules) != 1 || rules[0].Pattern != "repo-a:EntryService" || rules[0].Target != "rejected" {
		t.Fatalf("rules=%#v err=%v", rules, err)
	}
}

func TestSchemaNineKeepsVersionSevenFilesAndReports(t *testing.T) {
	directory := t.TempDir()
	db, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO files(repository_id, path, content_hash, content) VALUES('repo', 'Entry.java', 'hash', 'evidence');
DROP TABLE contracts; DROP INDEX symbols_uid; DROP INDEX symbols_repo_name;
ALTER TABLE symbols DROP COLUMN uid; ALTER TABLE symbols DROP COLUMN signature; ALTER TABLE symbols DROP COLUMN end_line;
ALTER TABLE edges DROP COLUMN source_signature; ALTER TABLE edges DROP COLUMN target_arity;
PRAGMA user_version = 7`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var files, version int
	if err := db.QueryRow(`SELECT count(*) FROM files`).Scan(&files); err != nil || files != 1 {
		t.Fatalf("files=%d err=%v", files, err)
	}
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != SchemaVersion {
		t.Fatalf("version=%d err=%v", version, err)
	}
}
