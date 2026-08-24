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

func TestTraceAndImpactFollowStoredEvidence(t *testing.T) {
	db, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	result := extract.Result{
		Symbols: []extract.Symbol{{Name: "com.example.EntryController", Kind: "controller", Line: 1}, {Name: "com.example.EntryService", Kind: "service", Line: 1}, {Name: "com.example.EntryMapper", Kind: "mapper", Line: 1}},
		Edges:   []extract.Edge{{Source: "com.example.EntryController", Target: "/entry", Kind: "route", Line: 2, Confidence: extract.Certain}, {Source: "com.example.EntryController", Target: "EntryService", Kind: "uses", Line: 3, Confidence: extract.Probable}, {Source: "com.example.EntryService", Target: "EntryMapper", Kind: "uses", Line: 2, Confidence: extract.Probable}},
	}
	if err := db.ReplaceEvidenceTx(tx, "repo", "Entry.java", result); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	trace, err := Trace(db, "EntryController", 3)
	if err != nil || longest(trace.Paths) != 2 {
		t.Fatalf("trace=%#v err=%v", trace, err)
	}
	impact, err := Impact(db, "com.example.EntryMapper", 3)
	if err != nil || len(impact.Relations) != 2 {
		t.Fatalf("impact=%#v err=%v", impact, err)
	}
	routeTrace, err := Trace(db, "/entry", 3)
	if err != nil || longest(routeTrace.Paths) != 2 {
		t.Fatalf("route trace=%#v err=%v", routeTrace, err)
	}
}

func TestTraceFollowsControllerMethodToMyBatisTable(t *testing.T) {
	db, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"EntryController.java": []byte(`package com.example.entry;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RestController;
import com.example.application.EntryApplication;
@RestController
class EntryController {
  private final EntryApplication application;
  @PostMapping("/entry/submit")
  public void submit() { application.submit(); }
}`),
		"EntryApplication.java": []byte(`package com.example.application;
import org.springframework.stereotype.Service;
import com.example.infrastructure.EntryMapper;
@Service
class EntryApplication {
  private final EntryMapper entryMapper;
  public void submit() { entryMapper.insert(); }
}`),
		"EntryMapper.xml": []byte("<mapper namespace=\"com.example.infrastructure.EntryMapper\"><insert id=\"insert\">INSERT INTO student_entry(id) VALUES (1)</insert></mapper>"),
	}
	for path, content := range files {
		if err := db.ReplaceEvidenceTx(tx, "repo", path, extract.File(path, content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	trace, err := Trace(db, "/entry/submit", 4)
	if err != nil || longest(trace.Paths) != 3 {
		t.Fatalf("trace=%#v err=%v", trace, err)
	}
	for _, path := range trace.Paths {
		if len(path) == 3 {
			last := path[2]
			if last.Target == "student_entry" && last.Kind == "queries_table" {
				return
			}
		}
	}
	t.Fatalf("table path missing: %#v", trace.Paths)
}

func longest(paths [][]Evidence) int {
	result := 0
	for _, path := range paths {
		if len(path) > result {
			result = len(path)
		}
	}
	return result
}
