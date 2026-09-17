package query

import "fmt"

type FlowStep struct {
	Stage    string   `json:"stage"`
	Evidence Evidence `json:"evidence"`
}

type Flow struct {
	Entry     Evidence   `json:"entry"`
	Steps     []FlowStep `json:"steps"`
	Crossings int        `json:"cross_repository_hops"`
}

type FlowResult struct {
	Target     string     `json:"target"`
	Flows      []Flow     `json:"flows"`
	Candidates []Evidence `json:"candidates,omitempty"`
	Gaps       []Evidence `json:"gaps,omitempty"`
	Coverage   Coverage   `json:"coverage"`
}

type ContextResult struct {
	Target     string            `json:"target"`
	Symbol     *Evidence         `json:"symbol,omitempty"`
	Candidates []Evidence        `json:"candidates,omitempty"`
	Callers    []Evidence        `json:"callers"`
	Callees    []Evidence        `json:"callees"`
	Related    []Evidence        `json:"related"`
	Flows      []Flow            `json:"flows"`
	Contracts  []ContractLink    `json:"contracts,omitempty"`
	Risks      []string          `json:"risks,omitempty"`
	Gaps       []Evidence        `json:"gaps,omitempty"`
	Counts     map[string]int    `json:"counts"`
	Expand     map[string]string `json:"expand,omitempty"`
	Coverage   Coverage          `json:"coverage"`
}

func stage(kind string) string {
	switch kind {
	case "controller", "controller_method", "route":
		return "entry"
	case "application", "application_method":
		return "application"
	case "repository", "repository_method":
		return "repository"
	case "mapper", "mapper_method", "mapper_statement", "maps_statement":
		return "mapper"
	case "queries_table", "migration_table":
		return "table"
	case "contract_http", "contract_dubbo", "contract_mq":
		return "cross_service"
	case "uses_config":
		return "configuration"
	}
	return "domain_or_service"
}

func (g *Graph) Flows(target, repo string, depth, limit int) (FlowResult, error) {
	trace, err := g.Trace(target, repo, depth, limit)
	r := FlowResult{Target: target, Candidates: trace.Candidates, Gaps: trace.Gaps, Coverage: trace.Coverage}
	if err != nil {
		return r, err
	}
	starts := g.starts(target, repo)
	if len(starts) != 1 {
		return r, nil
	}
	for _, p := range trace.Paths {
		f := Flow{Entry: starts[0], Steps: []FlowStep{{Stage: stage(starts[0].Kind), Evidence: starts[0]}}}
		for _, e := range p {
			kind := e.Kind
			if target, ok := g.byID[e.TargetID]; ok && !isContract(e.Kind) {
				kind = target.Kind
			}
			f.Steps = append(f.Steps, FlowStep{Stage: stage(kind), Evidence: e})
			if e.TargetRepository != "" && e.Repository != e.TargetRepository {
				f.Crossings++
			}
		}
		r.Flows = append(r.Flows, f)
	}
	return r, nil
}

func isContract(kind string) bool { return len(kind) >= 9 && kind[:9] == "contract_" }

func (g *Graph) Context(target, repo string, depth, limit int, detail bool) (ContextResult, error) {
	r := ContextResult{Target: target, Counts: map[string]int{}, Coverage: Coverage{Complete: true, Limit: limit}}
	if err := validateGraphOptions(depth, limit); err != nil {
		return r, err
	}
	starts := g.starts(target, repo)
	if len(starts) != 1 {
		r.Candidates = starts
		if len(starts) > 1 {
			r.Coverage.Gap("ambiguous_start", false)
		} else {
			r.Coverage.Gap("symbol_not_found", false)
		}
		return r, nil
	}
	s := starts[0]
	r.Symbol = &s
	for _, e := range g.in[s.ID] {
		r.Callers = append(r.Callers, e.Evidence)
	}
	for _, e := range g.out[s.ID] {
		if e.Kind == "calls" || isContract(e.Kind) {
			r.Callees = append(r.Callees, e.Evidence)
		} else {
			r.Related = append(r.Related, e.Evidence)
		}
	}
	r.Callers = dedupEvidence(r.Callers)
	r.Callees = dedupEvidence(r.Callees)
	r.Related = dedupEvidence(r.Related)
	flow, err := g.Flows(s.ID, s.Repository, depth, limit)
	if err != nil {
		return r, err
	}
	r.Flows = flow.Flows
	r.Gaps = flow.Gaps
	r.Coverage.Merge(flow.Coverage)
	for _, l := range g.Links {
		relevant := l.Consumer.SymbolID == s.ID
		for _, p := range l.Providers {
			relevant = relevant || p.SymbolID == s.ID
		}
		if relevant {
			r.Contracts = append(r.Contracts, l)
			if l.Status != "matched" {
				r.Coverage.Gap(l.Reason, false)
				r.Coverage.Unresolved++
			}
		}
	}
	r.Counts["callers"] = len(r.Callers)
	r.Counts["callees"] = len(r.Callees)
	r.Counts["related"] = len(r.Related)
	r.Counts["flows"] = len(r.Flows)
	r.Counts["contracts"] = len(r.Contracts)
	r.Counts["gaps"] = len(r.Gaps)
	// A single shared relation budget, rather than a limit per category.
	remaining := limit
	for _, items := range []*[]Evidence{&r.Callers, &r.Callees, &r.Related} {
		if len(*items) > remaining {
			*items = (*items)[:remaining]
			r.Coverage.Gap("context_relation_limit", true)
		}
		remaining -= len(*items)
	}
	if len(r.Contracts) > limit {
		r.Contracts = r.Contracts[:limit]
		r.Coverage.Gap("context_contract_limit", true)
	}
	if !detail {
		if len(r.Flows) > 3 {
			r.Flows = r.Flows[:3]
			r.Coverage.Gap("summary_flow_limit", true)
		}
		r.Expand = map[string]string{"get_symbol_context": s.ID, "trace_code_path": s.ID, "analyze_change_impact": s.ID, "list_contracts": s.Name}
		for i := range r.Flows {
			if len(r.Flows[i].Steps) > 4 {
				r.Flows[i].Steps = r.Flows[i].Steps[:4]
				r.Coverage.Gap("summary_step_limit", true)
			}
		}
	}
	if len(r.Gaps) > limit {
		r.Gaps = r.Gaps[:limit]
		r.Coverage.Gap("context_gap_limit", true)
	}
	r.Risks = append(r.Risks, r.Coverage.Reasons...)
	r.Coverage.Returned = limit - remaining
	if r.Counts["contracts"] > 0 {
		r.Risks = append(r.Risks, "Static contract match does not prove runtime routing or deployment compatibility.")
	}
	return r, nil
}

func ValidateRepository(g *Graph, repo string) error {
	if repo == "" {
		return nil
	}
	for _, s := range g.Symbols {
		if s.Repository == repo {
			return nil
		}
	}
	for _, c := range g.Contracts {
		if c.Repository == repo {
			return nil
		}
	}
	return fmt.Errorf("repository is not present in this snapshot: %s", repo)
}
