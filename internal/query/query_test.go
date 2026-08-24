package query

import (
	"testing"

	"github.com/CJhuochai/project-brain/internal/extract"
	"github.com/CJhuochai/project-brain/internal/storage"
)

func TestSearchReturnsEvidenceForRouteAndSymbol(t *testing.T) {
	db, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	result := extract.Result{Symbols: []extract.Symbol{{Name: "com.example.EntryController", Kind: "controller", Line: 3}}, Edges: []extract.Edge{{Source: "com.example.EntryController", Target: "/entry/submit", Kind: "route", Line: 4, Confidence: extract.Certain}}}
	if err := db.ReplaceEvidenceTx(tx, "repo", "EntryController.java", result); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	items, err := Search(db, "entry")
	if err != nil || len(items) < 2 {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	if items[0].File == "" || items[0].Line == 0 {
		t.Fatalf("missing source evidence: %#v", items[0])
	}
}
