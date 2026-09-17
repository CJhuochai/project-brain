package service

import (
	"github.com/CJhuochai/project-brain/internal/extract"
	"github.com/CJhuochai/project-brain/internal/query"
	"github.com/CJhuochai/project-brain/internal/storage"
	"testing"
)

func TestV2PublicGraphOperationsAndValidation(t *testing.T) {
	db, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, _ := db.Begin()
	r := extract.Result{Symbols: []extract.Symbol{{Name: "p.Controller.run", Kind: "controller_method", Line: 1, EndLine: 3, Signature: "p.Controller.run()"}}, Edges: []extract.Edge{{Source: "p.Controller.run", SourceSignature: "p.Controller.run()", Target: "/x", Kind: "route", Line: 1, Confidence: extract.Certain}}}
	if err := db.ReplaceEvidenceTx(tx, "repo", "Controller.java", r); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	for _, op := range []string{"trace_code_path", "analyze_change_impact", "get_symbol_context", "list_contracts", "trace_business_flow"} {
		result, err := Execute("", db, nil, op, map[string]any{"text": "p.Controller.run", "repository": "repo"}, Provenance{})
		if err != nil || result == nil {
			t.Fatalf("%s result=%#v err=%v", op, result, err)
		}
	}
	result, err := Execute("", db, nil, "get_symbol_context", map[string]any{"text": "/x", "view": "detail"}, Provenance{})
	if err != nil {
		t.Fatal(err)
	}
	context := result.(query.ContextResult)
	if context.Symbol == nil || len(context.Flows) != 1 || context.Expand != nil {
		t.Fatalf("context=%#v", context)
	}
	for _, args := range []map[string]any{{"text": "p.Controller.run", "limit": 1.5}, {"text": "p.Controller.run", "limit": 0}, {"text": "p.Controller.run", "max_depth": 33}, {"text": "p.Controller.run", "repository": "missing"}, {"text": "p.Controller.run", "repository": 42}, {"text": "p.Controller.run", "view": "invalid"}, {"text": " "}} {
		if _, err := Execute("", db, nil, "get_symbol_context", args, Provenance{}); err == nil {
			t.Fatalf("accepted invalid args: %#v", args)
		}
	}
}
