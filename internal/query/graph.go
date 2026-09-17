package query

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/CJhuochai/project-brain/internal/storage"
)

type graphEdge struct {
	Evidence
	arity           *int
	sourceSignature string
}

// Graph is scoped to one immutable snapshot and one operation; no mutable cache.
type Graph struct {
	Symbols     []Evidence
	byID        map[string]Evidence
	byName      map[string][]Evidence
	byShort     map[string][]Evidence
	out         map[string][]graphEdge
	in          map[string][]graphEdge
	diagnostics map[string][]Evidence
	Contracts   []ContractEvidence
	Links       []ContractLink
}

func LoadGraph(db *storage.DB) (*Graph, error) {
	g := &Graph{byID: map[string]Evidence{}, byName: map[string][]Evidence{}, byShort: map[string][]Evidence{}, out: map[string][]graphEdge{}, in: map[string][]graphEdge{}, diagnostics: map[string][]Evidence{}}
	rows, err := db.Query(`SELECT repository_id,path,line,name,kind,uid,signature,end_line FROM symbols ORDER BY repository_id,path,line,name,signature`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		s := Evidence{Confidence: "certain"}
		if err := rows.Scan(&s.Repository, &s.File, &s.Line, &s.Name, &s.Kind, &s.ID, &s.Signature, &s.EndLine); err != nil {
			rows.Close()
			return nil, err
		}
		if s.ID == "" {
			s.ID = storage.SymbolID(s.Repository, s.File, s.Name, s.Signature)
		}
		g.Symbols = append(g.Symbols, s)
		g.byID[s.ID] = s
		g.byName[s.Repository+"\x00"+s.Name] = append(g.byName[s.Repository+"\x00"+s.Name], s)
		g.byShort[s.Repository+"\x00"+shortName(s.Name)] = append(g.byShort[s.Repository+"\x00"+shortName(s.Name)], s)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	rows, err = db.Query(`SELECT repository_id,path,line,source,target,kind,confidence,source_signature,target_arity FROM edges ORDER BY repository_id,path,line,kind,target`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		e := graphEdge{}
		var arity sql.NullInt64
		if err := rows.Scan(&e.Repository, &e.File, &e.Line, &e.Source, &e.Target, &e.Kind, &e.Confidence, &e.sourceSignature, &arity); err != nil {
			rows.Close()
			return nil, err
		}
		if !traversable(e.Kind) {
			continue
		}
		if arity.Valid {
			v := int(arity.Int64)
			e.arity = &v
		}
		e.Name = e.Target
		sources := g.local(e.Repository, e.Source, e.sourceSignature, nil)
		// Legacy source names can be shared by overloads. Attribute only when range proves ownership.
		if len(sources) > 1 {
			var owned []Evidence
			for _, s := range sources {
				if s.File == e.File && s.Line <= e.Line && s.EndLine >= e.Line {
					owned = append(owned, s)
				}
			}
			sources = owned
		}
		if len(sources) != 1 {
			g.diagnostics[e.Repository+"\x00"+e.File] = append(g.diagnostics[e.Repository+"\x00"+e.File], Evidence{Repository: e.Repository, File: e.File, Line: e.Line, Kind: "diagnostic", Confidence: "unresolved", Reason: "unresolved_or_ambiguous_source: " + e.Source})
			continue
		}
		e.SourceID = sources[0].ID
		if e.Kind == "queries_table" || e.Kind == "route" || e.Kind == "uses_config" || e.Kind == "migration_table" {
			e.Reason = "terminal_evidence"
		} else {
			targets := g.local(e.Repository, e.Target, "", e.arity)
			if len(targets) == 1 {
				e.TargetID = targets[0].ID
				e.TargetRepository = targets[0].Repository
			} else if len(targets) > 1 {
				e.Reason = "ambiguous_target"
			} else {
				e.Reason = "unresolved_target"
			}
		}
		g.addEdge(e)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	rows, err = db.Query(`SELECT repository_id,path,line,message,confidence FROM diagnostics ORDER BY repository_id,path,line`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		d := Evidence{Kind: "diagnostic"}
		if err := rows.Scan(&d.Repository, &d.File, &d.Line, &d.Reason, &d.Confidence); err != nil {
			rows.Close()
			return nil, err
		}
		g.diagnostics[d.Repository+"\x00"+d.File] = append(g.diagnostics[d.Repository+"\x00"+d.File], d)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if err := g.loadContracts(db); err != nil {
		return nil, err
	}
	return g, nil
}

func traversable(kind string) bool {
	switch kind {
	case "calls", "uses", "implements", "maps_statement", "queries_table", "route", "uses_config", "depends_on_module":
		return true
	}
	return false
}

func (g *Graph) addEdge(e graphEdge) {
	g.out[e.SourceID] = append(g.out[e.SourceID], e)
	if e.TargetID != "" {
		g.in[e.TargetID] = append(g.in[e.TargetID], e)
	}
}

func (g *Graph) local(repo, name, signature string, arity *int) []Evidence {
	items := g.byName[repo+"\x00"+name]
	if len(items) == 0 && !strings.Contains(name, ".") {
		items = g.byShort[repo+"\x00"+name]
	}
	if len(items) == 0 && strings.Contains(name, ".") && !strings.Contains(strings.SplitN(name, ".", 2)[0], "/") {
		// Only relative Type.method suffixes, never drop a package from a qualified target.
		if strings.Count(name, ".") == 1 {
			for _, s := range g.Symbols {
				if s.Repository == repo && strings.HasSuffix(s.Name, "."+name) {
					items = append(items, s)
				}
			}
		}
	}
	var result []Evidence
	for _, s := range items {
		if signature != "" && s.Signature != signature {
			continue
		}
		if arity != nil && s.Signature != "" && signatureArity(s.Signature) != *arity {
			continue
		}
		result = append(result, s)
	}
	// MyBatis XML is executable evidence for a mapper declaration of the same name.
	if len(result) > 1 {
		var statements []Evidence
		for _, s := range result {
			if s.Kind == "mapper_statement" {
				statements = append(statements, s)
			}
		}
		if len(statements) == 1 {
			return statements
		}
	}
	return result
}

func signatureArity(s string) int {
	start, end := strings.Index(s, "("), strings.LastIndex(s, ")")
	if start < 0 || end < start {
		return -1
	}
	body := strings.TrimSpace(s[start+1 : end])
	if body == "" {
		return 0
	}
	depth, n := 0, 1
	for _, r := range body {
		switch r {
		case '<', '[':
			depth++
		case '>', ']':
			depth--
		case ',':
			if depth == 0 {
				n++
			}
		}
	}
	return n
}

func (g *Graph) Resolve(target, repo string) []Evidence {
	if s, ok := g.byID[target]; ok {
		if repo == "" || repo == s.Repository {
			return []Evidence{s}
		}
		return nil
	}
	name, signature := target, ""
	if i := strings.Index(target, "("); i >= 0 {
		name = target[:i]
		signature = target[i:]
	}
	var exact, fallback []Evidence
	for _, s := range g.Symbols {
		if repo != "" && s.Repository != repo {
			continue
		}
		if signature != "" && s.Signature != signature && s.Signature != name+signature && s.Signature != shortName(name)+signature {
			continue
		}
		if s.Name == name {
			exact = append(exact, s)
		} else if strings.HasSuffix(s.Name, "."+name) {
			fallback = append(fallback, s)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return fallback
}

func validateGraphOptions(depth, limit int) error {
	if depth < 1 || depth > 32 {
		return fmt.Errorf("max_depth must be between 1 and 32")
	}
	if limit < 1 || limit > 200 {
		return fmt.Errorf("limit must be between 1 and 200")
	}
	return nil
}

func evidenceKey(e Evidence) string {
	return fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%s\x00%s\x00%s", e.Repository, e.File, e.Line, e.Name, e.Kind, e.SourceID, e.TargetID)
}

func dedupEvidence(items []Evidence) []Evidence {
	seen := map[string]bool{}
	var out []Evidence
	for _, e := range items {
		k := evidenceKey(e)
		if !seen[k] {
			seen[k] = true
			out = append(out, e)
		}
	}
	return out
}

func sortEvidence(items []Evidence) {
	sort.SliceStable(items, func(i, j int) bool { return evidenceKey(items[i]) < evidenceKey(items[j]) })
}
