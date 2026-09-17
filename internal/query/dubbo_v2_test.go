package query

import (
	"github.com/CJhuochai/project-brain/internal/extract"
	"testing"
)

func TestContractReportDisclosesSkippedParserEvidence(t *testing.T) {
	db := v2DB(t)
	v2Put(t, db, "client", "Client.java", extract.Result{Diagnostics: []extract.Diagnostic{{Message: "动态 HTTP 合约未能安全解析", Line: 1, Confidence: extract.Unresolved}}})
	g, err := LoadGraph(db)
	if err != nil {
		t.Fatal(err)
	}
	r, err := g.ContractReport("client", "", 1)
	if err != nil || r.Coverage.Complete || r.Coverage.Diagnostics != 1 || len(r.Gaps) != 1 {
		t.Fatalf("report=%#v err=%v", r, err)
	}
}

func TestDubboCallBridgesLocalInterfaceStub(t *testing.T) {
	db := v2DB(t)
	for _, c := range []struct{ repo, file, source string }{
		{"client", "Client.java", "package p;\nimport api.OrderApi;\nclass Client {\n @DubboReference\n private OrderApi api;\n void run() { api.submit(); }\n}"},
		{"client", "OrderApi.java", "package api;\ninterface OrderApi {\n void submit();\n}"},
		{"client", "Local.java", "package p;\nimport api.OrderApi;\nclass Local {\n private OrderApi api;\n void run() { api.submit(); }\n}"},
		{"server", "Provider.java", "package p;\nimport api.OrderApi;\n@DubboService\nclass Provider implements OrderApi {\n void submit() {}\n}"},
	} {
		v2Put(t, db, c.repo, c.file, extract.File(c.file, []byte(c.source)))
	}
	g, err := LoadGraph(db)
	if err != nil {
		t.Fatal(err)
	}
	r, err := g.Trace("p.Client.run", "client", 6, 20)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, path := range r.Paths {
		for _, e := range path {
			if e.TargetRepository == "server" && e.Target == "p.Provider.submit" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("RPC stub blocked provider: %#v links=%#v", r, g.Links)
	}
	local, err := g.Trace("p.Local.run", "client", 6, 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range local.Paths {
		for _, e := range path {
			if e.TargetRepository == "server" {
				t.Fatalf("unannotated class inherited RPC contract: %#v", local)
			}
		}
	}
}
