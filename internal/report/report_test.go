package report

import (
	"testing"

	"github.com/CJhuochai/project-brain/internal/input"
	"github.com/CJhuochai/project-brain/internal/storage"
)

func TestAnalysisReportPersistsAndAppliesRule(t *testing.T) {
	db, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.UpsertFile("entry-service", "EntryService.java", []byte("class EntryService {}")); err != nil {
		t.Fatal(err)
	}
	first, err := AnalyzeRequirement(db, input.Result{Facts: []input.Fact{{Kind: "business_term", Value: "entry"}}})
	if err != nil || first.ID == "" || len(first.TestPoints) == 0 {
		t.Fatalf("report=%#v err=%v", first, err)
	}
	if err := db.RecordFeedback(storage.Feedback{ID: "f1", ReportID: first.ID, SubjectKind: "repository", SubjectKey: "entry-service", Decision: "rule", Note: "confirmed"}); err != nil {
		t.Fatal(err)
	}
	second, err := AnalyzeRequirement(db, input.Result{Facts: []input.Fact{{Kind: "business_term", Value: "entry"}}})
	if err != nil || len(second.Feedback) == 0 {
		t.Fatalf("report=%#v err=%v", second, err)
	}
	if _, err := db.Report(second.ID); err != nil {
		t.Fatal(err)
	}
}
