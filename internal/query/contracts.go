package query

import (
	"fmt"
	"path"
	"strings"

	"github.com/CJhuochai/project-brain/internal/storage"
)

type ContractEvidence struct {
	ID         string `json:"id"`
	Repository string `json:"repository"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	Kind       string `json:"kind"`
	Role       string `json:"role"`
	Service    string `json:"service,omitempty"`
	Key        string `json:"key"`
	Symbol     string `json:"symbol"`
	Signature  string `json:"signature,omitempty"`
	SymbolID   string `json:"symbol_id,omitempty"`
	Confidence string `json:"confidence"`
	Reason     string `json:"reason,omitempty"`
	Broker     string `json:"broker,omitempty"`
}

type ContractLink struct {
	Consumer  ContractEvidence   `json:"consumer"`
	Providers []ContractEvidence `json:"providers,omitempty"`
	Status    string             `json:"status"`
	Reason    string             `json:"reason"`
}

type ContractResult struct {
	Contracts []ContractEvidence `json:"contracts"`
	Links     []ContractLink     `json:"links"`
	Gaps      []Evidence         `json:"gaps,omitempty"`
	Coverage  Coverage           `json:"coverage"`
}

func (g *Graph) loadContracts(db *storage.DB) error {
	rows, err := db.Query(`SELECT repository_id,path,line,kind,role,service,contract_key,symbol,signature,confidence,reason,broker FROM contracts ORDER BY repository_id,path,line,kind,contract_key`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var c ContractEvidence
		if err := rows.Scan(&c.Repository, &c.File, &c.Line, &c.Kind, &c.Role, &c.Service, &c.Key, &c.Symbol, &c.Signature, &c.Confidence, &c.Reason, &c.Broker); err != nil {
			return err
		}
		c.ID = fmt.Sprintf("contract:%s", storage.SymbolID(c.Repository, c.File, c.Kind+":"+c.Role+":"+c.Key, c.Signature))
		g.Contracts = append(g.Contracts, c)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range g.Contracts {
		c := &g.Contracts[i]
		matches := g.local(c.Repository, c.Symbol, c.Signature, nil)
		if len(matches) == 1 {
			c.SymbolID = matches[0].ID
		}
		if c.Kind == "http" && c.Role == "provider" && c.Service == "" {
			c.Service = g.configValue(*c, "service", "")
		}
		if c.Kind == "mq" && c.Broker != "" && !strings.Contains(c.Broker, ":") {
			c.Broker = g.configValue(*c, "broker", c.Broker)
		}
		if c.Kind == "mq" && !strings.Contains(c.Broker, ":") {
			c.Broker = ""
		}
	}
	providers := map[string][]ContractEvidence{}
	for _, p := range g.Contracts {
		if p.Role == "provider" {
			providers[p.Kind+"\x00"+p.Key] = append(providers[p.Kind+"\x00"+p.Key], p)
		}
	}
	for _, c := range g.Contracts {
		if c.Role != "consumer" {
			continue
		}
		link := ContractLink{Consumer: c, Status: "unresolved", Reason: "no_verified_provider"}
		if c.Confidence == "unresolved" || strings.Contains(c.Key, "${") || strings.Contains(c.Service, "${") {
			link.Reason = "dynamic_contract"
			g.Links = append(g.Links, link)
			continue
		}
		if c.Kind == "http" && (c.Service == "" || strings.HasPrefix(c.Key, "ANY:")) {
			link.Reason = "missing_service_or_http_method"
			g.Links = append(g.Links, link)
			continue
		}
		if c.Kind == "dubbo" && !strings.Contains(c.Key, ".") {
			link.Reason = "unqualified_interface"
			g.Links = append(g.Links, link)
			continue
		}
		if c.Kind == "mq" && c.Broker == "" {
			link.Reason = "unknown_broker"
			g.Links = append(g.Links, link)
			continue
		}
		for _, p := range providers[c.Kind+"\x00"+c.Key] {
			if p.Confidence == "unresolved" {
				continue
			}
			match := false
			switch c.Kind {
			case "http":
				match = c.Service == p.Service && p.Service != ""
			case "dubbo":
				match = true
			case "mq":
				match = c.Broker == p.Broker && p.Broker != ""
			}
			if match {
				link.Providers = append(link.Providers, p)
			}
		}
		if len(link.Providers) == 1 && c.SymbolID != "" && link.Providers[0].SymbolID != "" {
			link.Status = "matched"
			link.Reason = "exact_static_contract"
			p := link.Providers[0]
			e := graphEdge{Evidence: Evidence{Repository: c.Repository, File: c.File, Line: c.Line, Name: p.Symbol, Source: c.Symbol, Target: p.Symbol, SourceID: c.SymbolID, TargetID: p.SymbolID, TargetRepository: p.Repository, Kind: "contract_" + c.Kind, Confidence: "probable", Reason: link.Reason}}
			if c.Kind == "mq" {
				e = graphEdge{Evidence: Evidence{Repository: p.Repository, File: p.File, Line: p.Line, Name: c.Symbol, Source: p.Symbol, Target: c.Symbol, SourceID: p.SymbolID, TargetID: c.SymbolID, TargetRepository: c.Repository, Kind: "contract_mq", Confidence: "probable", Reason: link.Reason}}
			}
			g.addEdge(e)
		} else if len(link.Providers) > 1 {
			link.Status = "ambiguous"
			link.Reason = "multiple_providers"
		} else if len(link.Providers) == 1 {
			link.Reason = "unresolved_contract_symbol"
		}
		g.Links = append(g.Links, link)
	}
	// Bind RPC method calls only through an unambiguous qualified contract.
	for sourceID, edges := range g.out {
		for i, e := range edges {
			if e.Kind != "calls" {
				continue
			}
			var targets []Evidence
			for _, link := range g.Links {
				if link.Status != "matched" || link.Consumer.Kind != "dubbo" || link.Consumer.Repository != e.Repository || !strings.HasPrefix(e.Source, link.Consumer.Symbol+".") || !strings.HasPrefix(e.Target, link.Consumer.Key+".") {
					continue
				}
				p := link.Providers[0]
				method := strings.TrimPrefix(e.Target, link.Consumer.Key)
				targets = append(targets, g.local(p.Repository, p.Symbol+method, "", e.arity)...)
			}
			targets = dedupEvidence(targets)
			if len(targets) == 1 {
				if e.TargetID != "" {
					incoming := g.in[e.TargetID]
					for j := len(incoming) - 1; j >= 0; j-- {
						if incoming[j].SourceID == e.SourceID && incoming[j].Target == e.Target && incoming[j].Line == e.Line {
							incoming = append(incoming[:j], incoming[j+1:]...)
						}
					}
					g.in[e.TargetID] = incoming
				}
				e.TargetID = targets[0].ID
				e.Target = targets[0].Name
				e.Name = targets[0].Name
				e.TargetRepository = targets[0].Repository
				e.Reason = "exact_dubbo_contract"
				e.Confidence = "probable"
				g.out[sourceID][i] = e
				g.in[e.TargetID] = append(g.in[e.TargetID], e)
			}
		}
	}
	return nil
}

// Config is associated only with its own Maven-style module path. Multiple
// profiles/values remain ambiguous rather than picking an arbitrary profile.
func (g *Graph) configValue(c ContractEvidence, kind, protocol string) string {
	best := -1
	values := map[string]bool{}
	for _, cfg := range g.Contracts {
		if cfg.Repository != c.Repository || cfg.Kind != kind {
			continue
		}
		if protocol != "" && cfg.Key != protocol && !strings.HasPrefix(cfg.Broker, protocol+":") {
			continue
		}
		prefix := modulePrefix(cfg.File)
		if prefix != "" && !strings.HasPrefix(c.File, prefix+"/") {
			continue
		}
		if len(prefix) < best {
			continue
		}
		if len(prefix) > best {
			best = len(prefix)
			values = map[string]bool{}
		}
		value := cfg.Service
		if kind == "broker" {
			value = cfg.Broker
		}
		if value != "" && !strings.Contains(value, "${") {
			values[value] = true
		}
	}
	if len(values) == 1 {
		for v := range values {
			return v
		}
	}
	return ""
}

func modulePrefix(file string) string {
	file = strings.ReplaceAll(file, "\\", "/")
	if i := strings.Index(file, "/src/"); i >= 0 {
		return file[:i]
	}
	if strings.HasPrefix(file, "src/") {
		return ""
	}
	dir := path.Dir(file)
	if dir == "." {
		return ""
	}
	return dir
}

func (g *Graph) ContractReport(repo, filter string, limit int) (ContractResult, error) {
	r := ContractResult{Coverage: Coverage{Complete: true, Limit: limit}}
	if limit < 1 || limit > 1000 {
		return r, fmt.Errorf("limit must be between 1 and 1000")
	}
	ids := map[string]bool{}
	files := map[string]bool{}
	for _, c := range g.Contracts {
		if c.Kind == "service" || c.Kind == "broker" {
			continue
		}
		if repo != "" && c.Repository != repo {
			continue
		}
		if filter != "" && !strings.Contains(strings.ToLower(c.Key+" "+c.Symbol+" "+c.Service), strings.ToLower(filter)) {
			continue
		}
		r.Coverage.Matched++
		files[c.Repository+"\x00"+c.File] = true
		if len(r.Contracts) >= limit {
			r.Coverage.Gap("contract_limit", true)
			continue
		}
		r.Contracts = append(r.Contracts, c)
		ids[c.ID] = true
	}
	for _, l := range g.Links {
		if !ids[l.Consumer.ID] {
			continue
		}
		r.Links = append(r.Links, l)
		if l.Status != "matched" {
			r.Coverage.Unresolved++
			r.Coverage.Gap(l.Reason, false)
		}
	}
	for key, diagnostics := range g.diagnostics {
		for _, d := range diagnostics {
			if repo != "" && d.Repository != repo {
				continue
			}
			if filter != "" && !files[key] && !strings.Contains(strings.ToLower(d.Reason), strings.ToLower(filter)) {
				continue
			}
			r.Coverage.Diagnostics++
			r.Coverage.Gap("parser_diagnostics", false)
			r.Gaps = append(r.Gaps, d)
		}
	}
	sortEvidence(r.Gaps)
	if len(r.Gaps) > limit {
		r.Gaps = r.Gaps[:limit]
		r.Coverage.Gap("contract_gap_limit", true)
	}
	r.Coverage.Returned = len(r.Contracts)
	return r, nil
}
