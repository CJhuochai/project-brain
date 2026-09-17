package query

import (
	"fmt"
	"strings"
	"testing"

	"github.com/CJhuochai/project-brain/internal/extract"
	"github.com/CJhuochai/project-brain/internal/storage"
)

func TestNormalizedExactSurvivesCandidateLimit(t *testing.T) {
	db := searchTestDB(t)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for i := 0; i < 1001; i++ {
		if err := db.ReplaceEvidenceTx(tx, "repo", fmt.Sprintf("A%04d.java", i), extract.Result{Symbols: []extract.Symbol{{Name: fmt.Sprintf("p.Save%04dThing", i), Kind: "class", Line: 1}}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.ReplaceEvidenceTx(tx, "repo", "Z.java", extract.Result{Symbols: []extract.Symbol{{Name: "p.SaveThing", Kind: "class", Line: 1}}}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	r, err := SearchRanked(db, "save_thing", 1)
	if err != nil || len(r.Evidence) != 1 || r.Evidence[0].Name != "p.SaveThing" {
		t.Fatalf("normalized=%#v err=%v", r, err)
	}
}

func TestSearchRankedPrefersExactNormalizesAndDeduplicates(t *testing.T) {
	db := searchTestDB(t)
	putSearchFile(t, db, "repo-a", "OrderService.java", "OrderService handles 学生报名 and a literal 100% value.", extract.Result{Symbols: []extract.Symbol{{Name: "OrderService", Kind: "service", Line: 1}, {Name: "StudentEntryController", Kind: "controller", Line: 2}}})
	putSearchFile(t, db, "repo-a", "Notes.txt", "A literal file match is indexed twice by the candidate sources.", extract.Result{})

	exact, err := SearchRanked(db, "OrderService", 10)
	if err != nil || len(exact.Evidence) == 0 || exact.Evidence[0].Name != "OrderService" || exact.Evidence[0].Score != 1000 {
		t.Fatalf("exact=%#v err=%v", exact, err)
	}
	camel, err := SearchRanked(db, "student_entry", 10)
	if err != nil || len(camel.Evidence) == 0 || camel.Evidence[0].Name != "StudentEntryController" {
		t.Fatalf("camel=%#v err=%v", camel, err)
	}
	chinese, err := SearchRanked(db, "学生报名", 10)
	if err != nil || len(chinese.Evidence) == 0 || chinese.Evidence[0].File != "OrderService.java" {
		t.Fatalf("chinese=%#v err=%v", chinese, err)
	}
	dedup, err := SearchRanked(db, "candidate sources", 10)
	if err != nil || len(dedup.Evidence) != 1 || !hasSource(dedup.Evidence[0], "file_fts") || !hasSource(dedup.Evidence[0], "file_substring") {
		t.Fatalf("dedup=%#v err=%v", dedup, err)
	}
}

func TestSearchRankedLimitsScopeAndLiteralPunctuation(t *testing.T) {
	db := searchTestDB(t)
	for _, name := range []string{"OrderController", "OrderService", "OrderMapper"} {
		putSearchFile(t, db, "repo-a", name+".java", "ratio=100%", extract.Result{Symbols: []extract.Symbol{{Name: name, Kind: "service", Line: 1}}})
	}
	putSearchFile(t, db, "repo-b", "OrderService.java", "other repository", extract.Result{Symbols: []extract.Symbol{{Name: "OrderService", Kind: "service", Line: 1}}})
	limited, err := SearchRanked(db, "Order", 1)
	if err != nil || len(limited.Evidence) != 1 || !limited.Coverage.Truncated || limited.Coverage.Matched < 3 {
		t.Fatalf("limited=%#v err=%v", limited, err)
	}
	scoped, err := SearchRankedScoped(db, "OrderService", "repo-a", 10)
	if err != nil || len(scoped.Evidence) == 0 || scoped.Evidence[0].Repository != "repo-a" {
		t.Fatalf("scoped=%#v err=%v", scoped, err)
	}
	if _, err := SearchRankedScoped(db, "Order", "missing", 10); err == nil {
		t.Fatal("missing repository scope must fail")
	}
	punctuation, err := SearchRanked(db, "%", 10)
	if err != nil || len(punctuation.Evidence) != 3 {
		t.Fatalf("punctuation=%#v err=%v", punctuation, err)
	}
	for _, limit := range []int{0, 1001} {
		if _, err := SearchRanked(db, "Order", limit); err == nil {
			t.Fatalf("limit %d must fail", limit)
		}
	}
	if _, err := SearchRanked(db, "   ", 1); err == nil {
		t.Fatal("empty text must fail")
	}
}

func TestSearchRankedReturnsFTSErrorsInsteadOfZeroHits(t *testing.T) {
	db := searchTestDB(t)
	putSearchFile(t, db, "repo", "Empty.java", "known content", extract.Result{})
	zero, err := SearchRanked(db, "absent", 10)
	if err != nil || len(zero.Evidence) != 0 {
		t.Fatalf("zero=%#v err=%v", zero, err)
	}
	if _, err := db.Exec(`DROP TABLE file_fts`); err != nil {
		t.Fatal(err)
	}
	if _, err := SearchRanked(db, "absent", 10); err == nil {
		t.Fatal("missing FTS table must be an error")
	}
}

func TestSearchRankedKeepsLateExactAndFTSRelevance(t *testing.T) {
	db := searchTestDB(t)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < searchCandidateLimit+1; index++ {
		if err := db.ReplaceEvidenceTx(tx, "a", fmt.Sprintf("broad%04d.java", index), extract.Result{Symbols: []extract.Symbol{{Name: "OrderServiceHelper", Kind: "service", Line: 1}}}); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
	}
	if err := db.ReplaceEvidenceTx(tx, "z", "Exact.java", extract.Result{Symbols: []extract.Symbol{{Name: "OrderService", Kind: "service", Line: 1}}}); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	exact, err := SearchRanked(db, "OrderService", 1)
	if err != nil || len(exact.Evidence) != 1 || exact.Evidence[0].Name != "OrderService" || exact.Evidence[0].ID == "" {
		t.Fatalf("exact=%#v err=%v", exact, err)
	}
	putSearchFile(t, db, "fts", "best.txt", "alpha beta alpha beta alpha beta", extract.Result{})
	putSearchFile(t, db, "fts", "other.txt", "alpha beta", extract.Result{})
	fts, err := SearchRanked(db, "alpha beta", 10)
	if err != nil || len(fts.Evidence) < 2 || fts.Evidence[0].File != "best.txt" {
		t.Fatalf("fts=%#v err=%v", fts, err)
	}
}

func searchTestDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func putSearchFile(t *testing.T, db *storage.DB, repository, path, content string, result extract.Result) {
	t.Helper()
	if _, err := db.UpsertFile(repository, path, []byte(content)); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceEvidenceTx(tx, repository, path, result); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func hasSource(item Evidence, source string) bool {
	return strings.Contains(strings.Join(item.MatchSources, ","), source)
}
