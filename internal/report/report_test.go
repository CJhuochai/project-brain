package report

import (
	"path/filepath"
	"strings"
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
	if err := db.RecordIndex("entry-service", "release", "abc", "baseline_known"); err != nil {
		t.Fatal(err)
	}
	first, err := AnalyzeRequirement(db, input.Result{Facts: []input.Fact{{Kind: "business_term", Value: "entry"}}})
	if err != nil || first.ID == "" || first.BaselineSnapshot == "" || len(first.RecommendedBranches) != 1 || len(first.TestPoints) == 0 {
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

// Break caught: persisting a report without its selected snapshot and rule
// revision makes a later feedback write change the meaning of old analysis.
func TestAnalysisReportPersistsChosenSnapshotAndRuleRevision(t *testing.T) {
	directory := t.TempDir()
	legacy, err := storage.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.UpsertFile("student", "AdmitService.java", []byte("class AdmitService {}")); err != nil {
		t.Fatal(err)
	}
	if err := legacy.RecordIndex("student", "release", "abc", "baseline_known"); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	control, err := storage.OpenControl(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close() })
	snapshot, err := control.ActiveSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	index, err := storage.OpenSnapshot(filepath.Clean(snapshot.Path), false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = index.Close() })
	result, err := AnalyzeRequirementAt(index, control, input.Result{Facts: []input.Fact{{Kind: "business_term", Value: "Admit"}}}, Provenance{SnapshotID: snapshot.ID, SnapshotCompleted: snapshot.Completed, Baselines: "student@abc", RuleRevision: 0})
	if err != nil {
		t.Fatal(err)
	}
	if result.SnapshotID != snapshot.ID || result.RuleRevision != 0 {
		t.Fatalf("report=%#v", result)
	}
	stored, err := control.Report(result.ID)
	if err != nil || !strings.Contains(stored.JSON, `"snapshot_id":"`+snapshot.ID+`"`) {
		t.Fatalf("stored=%#v err=%v", stored, err)
	}
}
