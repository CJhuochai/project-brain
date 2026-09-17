package query

import (
	"fmt"
	"slices"
	"testing"

	"github.com/CJhuochai/project-brain/internal/extract"
	"github.com/CJhuochai/project-brain/internal/storage"
)

func v2DB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
func v2Put(t *testing.T, db *storage.DB, repo, file string, r extract.Result) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := db.ReplaceEvidenceTx(tx, repo, file, r); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}
func v2Symbol(name, signature string) extract.Symbol {
	return extract.Symbol{Name: name, Signature: signature, Line: 1, EndLine: 20, Kind: "service_method"}
}

func TestGraphIdentityScopeOverloadsAndIncoming(t *testing.T) {
	db := v2DB(t)
	one, two := 1, 2
	v2Put(t, db, "a", "Root.java", extract.Result{Symbols: []extract.Symbol{v2Symbol("p.Root.run", "p.Root.run()")}, Edges: []extract.Edge{{Source: "p.Root.run", SourceSignature: "p.Root.run()", Target: "p.Service.save", TargetArity: &one, Kind: "calls", Line: 2, Confidence: extract.Certain}}})
	v2Put(t, db, "a", "Service.java", extract.Result{Symbols: []extract.Symbol{v2Symbol("p.Service.save", "p.Service.save(int)"), v2Symbol("p.Service.save", "p.Service.save(int,int)")}})
	v2Put(t, db, "b", "Service.java", extract.Result{Symbols: []extract.Symbol{v2Symbol("p.Service.save", "p.Service.save(int)")}})
	g, err := LoadGraph(db)
	if err != nil {
		t.Fatal(err)
	}
	r, err := g.Trace("p.Root.run", "a", 6, 20)
	if err != nil || !r.Coverage.Complete || len(r.Paths) != 1 || r.Paths[0][0].TargetID == "" || r.Paths[0][0].TargetRepository != "a" {
		t.Fatalf("trace=%#v err=%v", r, err)
	}
	impact, err := g.Impact("p.Service.save(int)", "a", 6, 20)
	if err != nil || len(impact.Relations) != 1 {
		t.Fatalf("impact=%#v err=%v", impact, err)
	}
	ambiguous, _ := g.Trace("p.Service.save", "", 6, 20)
	if len(ambiguous.Candidates) != 3 || ambiguous.Coverage.Complete {
		t.Fatalf("ambiguity=%#v", ambiguous)
	}
	if signatureArity("save(java.util.Map<A,B>,int)") != two {
		t.Fatal("generic commas counted as parameters")
	}
	// Same arity with unknown argument types cannot pick an overload.
	v2Put(t, db, "a", "Service.java", extract.Result{Symbols: []extract.Symbol{v2Symbol("p.Service.save", "p.Service.save(int)"), v2Symbol("p.Service.save", "p.Service.save(String)")}})
	g, err = LoadGraph(db)
	if err != nil {
		t.Fatal(err)
	}
	r, _ = g.Trace("p.Root.run", "a", 6, 20)
	if r.Coverage.Complete || r.Coverage.Unresolved != 1 || r.Paths[0][0].TargetID != "" {
		t.Fatalf("same-arity=%#v", r)
	}
}

func TestGraphNeverImplicitlyCrossesRepository(t *testing.T) {
	db := v2DB(t)
	v2Put(t, db, "a", "Root.java", extract.Result{Symbols: []extract.Symbol{v2Symbol("p.Root.run", "")}, Edges: []extract.Edge{{Source: "p.Root.run", Target: "p.Other.run", Kind: "calls", Line: 2, Confidence: extract.Certain}}})
	v2Put(t, db, "b", "Other.java", extract.Result{Symbols: []extract.Symbol{v2Symbol("p.Other.run", "")}})
	g, err := LoadGraph(db)
	if err != nil {
		t.Fatal(err)
	}
	r, _ := g.Trace("p.Root.run", "a", 6, 20)
	if r.Coverage.Complete || r.Paths[0][0].TargetID != "" {
		t.Fatalf("cross-repo=%#v", r)
	}
}

func TestGraphCoverageBranchDepthCycleAndDiagnostics(t *testing.T) {
	db := v2DB(t)
	result := extract.Result{Symbols: []extract.Symbol{{Name: "Root", Line: 1, Kind: "service"}}, Diagnostics: []extract.Diagnostic{{Message: "dynamic", Line: 1, Confidence: extract.Unresolved}}}
	for i := 0; i < 21; i++ {
		name := fmt.Sprintf("Leaf%d", i)
		result.Symbols = append(result.Symbols, extract.Symbol{Name: name, Line: i + 2, Kind: "service"})
		result.Edges = append(result.Edges, extract.Edge{Source: "Root", Target: name, Kind: "calls", Line: i + 2, Confidence: extract.Certain})
	}
	v2Put(t, db, "a", "Root.java", result)
	g, _ := LoadGraph(db)
	r, _ := g.Trace("Root", "a", 1, 20)
	if len(r.Paths) != 20 || !r.Coverage.Truncated || !slices.Contains(r.Coverage.Reasons, "path_limit") || r.Coverage.Diagnostics != 1 {
		t.Fatalf("branches=%#v", r)
	}
	result.Edges = []extract.Edge{{Source: "Root", Target: "Leaf0", Kind: "calls", Line: 2, Confidence: extract.Certain}, {Source: "Leaf0", Target: "Root", Kind: "calls", Line: 3, Confidence: extract.Certain}}
	result.Diagnostics = nil
	v2Put(t, db, "a", "Root.java", result)
	g, _ = LoadGraph(db)
	r, _ = g.Trace("Root", "a", 1, 20)
	if !slices.Contains(r.Coverage.Reasons, "depth_limit") {
		t.Fatalf("depth=%#v", r)
	}
	r, _ = g.Trace("Root", "a", 6, 20)
	if !slices.Contains(r.Coverage.Reasons, "cycle_detected") {
		t.Fatalf("cycle=%#v", r)
	}
}

func TestContractsServiceVerbBrokerAndAmbiguity(t *testing.T) {
	db := v2DB(t)
	consumer := extract.Contract{Kind: "http", Role: "consumer", Service: "students", Key: "POST:/entry", Symbol: "Client.submit", Line: 1, Confidence: extract.Certain}
	v2Put(t, db, "client", "Client.java", extract.Result{Symbols: []extract.Symbol{v2Symbol("Client.submit", "")}, Contracts: []extract.Contract{consumer}})
	for _, p := range []struct{ repo, service, key string }{{"right", "students", "POST:/entry"}, {"wrong-service", "finance", "POST:/entry"}, {"wrong-verb", "students", "GET:/entry"}} {
		v2Put(t, db, p.repo, "Controller.java", extract.Result{Symbols: []extract.Symbol{v2Symbol("Controller.submit", "")}, Contracts: []extract.Contract{{Kind: "http", Role: "provider", Service: p.service, Key: p.key, Symbol: "Controller.submit", Line: 1, Confidence: extract.Certain}}})
	}
	g, err := LoadGraph(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Links) != 1 || g.Links[0].Status != "matched" || g.Links[0].Providers[0].Repository != "right" {
		t.Fatalf("links=%#v", g.Links)
	}
	flow, _ := g.Flows("Client.submit", "client", 6, 20)
	if len(flow.Flows) != 1 || flow.Flows[0].Crossings != 1 || flow.Flows[0].Steps[1].Stage != "cross_service" {
		t.Fatalf("flow=%#v", flow)
	}
	v2Put(t, db, "duplicate", "Controller.java", extract.Result{Symbols: []extract.Symbol{v2Symbol("Controller.submit", "")}, Contracts: []extract.Contract{{Kind: "http", Role: "provider", Service: "students", Key: "POST:/entry", Symbol: "Controller.submit", Line: 1, Confidence: extract.Certain}}})
	g, _ = LoadGraph(db)
	if g.Links[0].Status != "ambiguous" {
		t.Fatalf("duplicate=%#v", g.Links)
	}
	for _, p := range []struct{ repo, role, broker string }{{"listener", "consumer", "kafka:cluster-a"}, {"publisher", "provider", "kafka:cluster-b"}} {
		v2Put(t, db, p.repo, "MQ.java", extract.Result{Symbols: []extract.Symbol{v2Symbol("MQ.run", "")}, Contracts: []extract.Contract{{Kind: "mq", Role: p.role, Broker: p.broker, Key: "entry.created", Symbol: "MQ.run", Line: 1, Confidence: extract.Certain}}})
	}
	g, _ = LoadGraph(db)
	for _, l := range g.Links {
		if l.Consumer.Kind == "mq" && l.Status == "matched" {
			t.Fatal("different brokers linked")
		}
	}
	v2Put(t, db, "publisher", "MQ.java", extract.Result{Symbols: []extract.Symbol{v2Symbol("MQ.run", "")}, Contracts: []extract.Contract{{Kind: "mq", Role: "provider", Broker: "kafka:cluster-a", Key: "entry.created", Symbol: "MQ.run", Line: 1, Confidence: extract.Certain}}})
	g, _ = LoadGraph(db)
	mqFlow, _ := g.Flows("MQ.run", "publisher", 6, 20)
	if len(mqFlow.Flows) != 1 || mqFlow.Flows[0].Crossings != 1 {
		t.Fatalf("MQ flow=%#v", mqFlow)
	}
}

func TestContractConfigAssociationAndContextLimits(t *testing.T) {
	db := v2DB(t)
	v2Put(t, db, "server", "student/src/main/resources/application.yml", extract.Result{Contracts: []extract.Contract{{Kind: "service", Role: "provider", Service: "students", Key: "students", Line: 1, Confidence: extract.Certain}}})
	v2Put(t, db, "server", "finance/src/main/resources/application.yml", extract.Result{Contracts: []extract.Contract{{Kind: "service", Role: "provider", Service: "finance", Key: "finance", Line: 1, Confidence: extract.Certain}}})
	v2Put(t, db, "server", "student/src/main/java/Controller.java", extract.Result{Symbols: []extract.Symbol{v2Symbol("Controller.run", "")}, Contracts: []extract.Contract{{Kind: "http", Role: "provider", Key: "GET:/x", Symbol: "Controller.run", Line: 1, Confidence: extract.Certain}}})
	g, err := LoadGraph(db)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range g.Contracts {
		if c.Kind == "http" && c.Service != "students" {
			t.Fatalf("module config=%#v", c)
		}
	}
	context, err := g.Context("Controller.run", "server", 6, 1, false)
	if err != nil || context.Symbol == nil || context.Symbol.ID == "" || context.Expand["get_symbol_context"] == "" {
		t.Fatalf("context=%#v err=%v", context, err)
	}
	if _, err := g.Context("Controller.run", "server", 0, 1, false); err == nil {
		t.Fatal("invalid depth accepted")
	}
}
