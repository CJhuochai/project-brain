package query

import (
	"github.com/CJhuochai/project-brain/internal/storage"
	"strings"
)

type TraceResult struct {
	Target     string       `json:"target"`
	Candidates []Evidence   `json:"candidates,omitempty"`
	Paths      [][]Evidence `json:"paths,omitempty"`
	Gaps       []Evidence   `json:"gaps,omitempty"`
	Coverage   Coverage     `json:"coverage"`
}

type ImpactResult struct {
	Target     string     `json:"target"`
	Candidates []Evidence `json:"candidates,omitempty"`
	Relations  []Evidence `json:"relations,omitempty"`
	Gaps       []Evidence `json:"gaps,omitempty"`
	Coverage   Coverage   `json:"coverage"`
}

const maxTracePaths = 20

func Trace(db *storage.DB, target string, maxDepth int) (TraceResult, error) {
	g, err := LoadGraph(db)
	if err != nil {
		return TraceResult{}, err
	}
	return g.Trace(target, "", maxDepth, maxTracePaths)
}
func Impact(db *storage.DB, target string, maxDepth int) (ImpactResult, error) {
	g, err := LoadGraph(db)
	if err != nil {
		return ImpactResult{}, err
	}
	return g.Impact(target, "", maxDepth, maxTracePaths)
}

func (g *Graph) starts(target, repo string) []Evidence {
	starts := g.Resolve(target, repo)
	if len(starts) == 0 && strings.HasPrefix(target, "/") {
		seen := map[string]bool{}
		for id, edges := range g.out {
			for _, e := range edges {
				if e.Kind == "route" && e.Target == target && (repo == "" || repo == e.Repository) && !seen[id] {
					seen[id] = true
					starts = append(starts, g.byID[id])
				}
			}
		}
		sortEvidence(starts)
	}
	return starts
}

func (g *Graph) Trace(target, repo string, depth, limit int) (TraceResult, error) {
	r := TraceResult{Target: target, Coverage: Coverage{Complete: true, Limit: limit}}
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
	r.Paths, r.Gaps, r.Coverage = g.walk(starts[0], depth, limit, false)
	return r, nil
}

func (g *Graph) Impact(target, repo string, depth, limit int) (ImpactResult, error) {
	r := ImpactResult{Target: target, Coverage: Coverage{Complete: true, Limit: limit}}
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
	paths, gaps, coverage := g.walk(starts[0], depth, limit, true)
	for _, p := range paths {
		r.Relations = append(r.Relations, p...)
	}
	r.Relations = dedupEvidence(r.Relations)
	r.Gaps = gaps
	r.Coverage = coverage
	r.Coverage.Returned = len(r.Relations)
	for _, edges := range g.out {
		for _, e := range edges {
			if e.TargetID == "" && (e.Reason == "ambiguous_target" || e.Reason == "unresolved_target") && (e.Target == starts[0].Name || e.Repository == starts[0].Repository && e.Target == shortName(starts[0].Name)) {
				r.Gaps = append(r.Gaps, e.Evidence)
				r.Coverage.Unresolved++
				r.Coverage.Gap("unresolved_incoming", false)
			}
		}
	}
	r.Gaps = dedupEvidence(r.Gaps)
	return r, nil
}

func (g *Graph) walk(start Evidence, maxDepth, limit int, reverse bool) ([][]Evidence, []Evidence, Coverage) {
	var paths [][]Evidence
	var gaps []Evidence
	c := Coverage{Complete: true, Limit: limit}
	diagnosed := map[string]bool{}
	gapSeen := map[string]bool{}
	addGap := func(e Evidence) {
		k := evidenceKey(e)
		if !gapSeen[k] {
			gapSeen[k] = true
			gaps = append(gaps, e)
			c.Unresolved++
			c.Gap(e.Reason, false)
		}
	}
	var visit func(Evidence, int, []Evidence, map[string]bool)
	appendPath := func(p []Evidence) {
		if len(p) == 0 {
			return
		}
		if len(paths) >= limit {
			c.Gap("path_limit", true)
			return
		}
		paths = append(paths, append([]Evidence(nil), p...))
	}
	visit = func(current Evidence, depth int, path []Evidence, seen map[string]bool) {
		for _, link := range g.Links {
			if link.Consumer.SymbolID == current.ID && link.Status != "matched" {
				addGap(Evidence{Repository: current.Repository, File: link.Consumer.File, Line: link.Consumer.Line, Name: link.Consumer.Key, Kind: "contract_" + link.Consumer.Kind, Reason: link.Reason, Confidence: "unresolved"})
			}
		}
		key := current.Repository + "\x00" + current.File
		if !diagnosed[key] {
			diagnosed[key] = true
			for _, d := range g.diagnostics[key] {
				gaps = append(gaps, d)
				c.Diagnostics++
				c.Gap("parser_diagnostics", false)
			}
		}
		edges := g.out[current.ID]
		if reverse {
			edges = g.in[current.ID]
		}
		if len(edges) == 0 {
			appendPath(path)
			return
		}
		if depth >= maxDepth {
			c.Gap("depth_limit", true)
			appendPath(path)
			return
		}
		for _, e := range edges {
			if len(paths) >= limit {
				c.Gap("path_limit", true)
				return
			}
			nextID := e.TargetID
			if reverse {
				nextID = e.SourceID
			}
			nextPath := append(append([]Evidence(nil), path...), e.Evidence)
			if nextID == "" {
				if e.Reason != "terminal_evidence" {
					addGap(e.Evidence)
				}
				appendPath(nextPath)
				continue
			}
			if seen[nextID] {
				c.Gap("cycle_detected", false)
				appendPath(nextPath)
				continue
			}
			seen[nextID] = true
			visit(g.byID[nextID], depth+1, nextPath, seen)
			delete(seen, nextID)
		}
	}
	visit(start, 0, nil, map[string]bool{start.ID: true})
	c.Returned = len(paths)
	return paths, dedupEvidence(gaps), c
}

func shortName(name string) string {
	if index := strings.LastIndex(name, "."); index >= 0 {
		return name[index+1:]
	}
	return name
}
