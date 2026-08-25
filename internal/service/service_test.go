package service

import (
	"path/filepath"
	"testing"

	"github.com/CJhuochai/project-brain/internal/storage"
)

// Break caught: routing a source lookup to control.sqlite rather than the
// selected snapshot would either fail or read data from a changing database.
func TestExecuteSearchUsesSelectedSnapshot(t *testing.T) {
	directory := t.TempDir()
	legacy, err := storage.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.UpsertFile("service", "Main.java", []byte("class Main {}")); err != nil {
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
	selected, err := control.ActiveSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	index, err := storage.OpenSnapshot(filepath.Clean(selected.Path), false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = index.Close() })
	result, err := Execute(directory, index, control, "find_business_context", map[string]any{"text": "Main"}, Provenance{SnapshotID: selected.ID})
	if err != nil {
		t.Fatal(err)
	}
	items, ok := result.([]Evidence)
	if !ok || len(items) != 1 || items[0].File != "Main.java" {
		t.Fatalf("result=%#v", result)
	}
}

// Break caught: feedback must be serialized into control state and expose the
// revision committed with the feedback, not a stale pre-write revision.
func TestExecuteFeedbackReturnsCommittedRuleRevision(t *testing.T) {
	control, err := storage.OpenControl(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close() })
	result, err := Execute("", nil, control, "record_analysis_feedback", map[string]any{
		"report_id": "r1", "subject_kind": "repository", "subject_key": "student", "decision": "confirmed", "note": "verified",
	}, Provenance{})
	if err != nil {
		t.Fatal(err)
	}
	feedback, ok := result.(FeedbackResult)
	if !ok || feedback.RuleRevision != 1 {
		t.Fatalf("feedback=%#v", result)
	}
}
