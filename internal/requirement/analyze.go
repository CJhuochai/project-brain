package requirement

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/CJhuochai/project-brain/internal/query"
	"github.com/CJhuochai/project-brain/internal/storage"
)

type RepositoryCandidate struct {
	Repository string  `json:"repository"`
	Evidence   int     `json:"evidence_count"`
	Score      float64 `json:"score"`
}

type Report struct {
	Coverage     query.Coverage        `json:"coverage"`
	Flows        []query.FlowResult    `json:"flows,omitempty"`
	Keywords     []string              `json:"keywords"`
	Evidence     []query.Evidence      `json:"evidence"`
	Repositories []RepositoryCandidate `json:"repositories"`
	EntryPoints  []query.Evidence      `json:"entry_points"`
	ChangePoints []query.Evidence      `json:"change_points"`
	Impacts      []query.TraceResult   `json:"impacts,omitempty"`
	Risks        []string              `json:"risks,omitempty"`
	Questions    []string              `json:"questions,omitempty"`
}

var tokenPattern = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_/-]*`)

const maxImpactTraces = 8
const maxEvidence = 200

func Analyze(db *storage.DB, text string) (Report, error) {
	report := Report{Coverage: query.Coverage{Complete: true, Limit: maxEvidence}}
	if strings.TrimSpace(text) == "" {
		return report, fmt.Errorf("requirement text must not be empty")
	}
	counts := map[string]int{}
	seen := map[string]bool{}
	all := map[string]query.Evidence{}
	var tokens []string
	tokenSeen := map[string]bool{}
	for _, token := range keywords(text) {
		key := strings.ToLower(token)
		if !tokenSeen[key] {
			tokenSeen[key] = true
			tokens = append(tokens, token)
		}
	}
	if len(tokens) > 64 {
		tokens = tokens[:64]
		report.Coverage.Gap("keyword_limit", true)
	}
	for _, token := range tokens {
		if seen[strings.ToLower(token)] {
			continue
		}
		seen[strings.ToLower(token)] = true
		report.Keywords = append(report.Keywords, token)
		search, err := query.SearchRanked(db, token, 1000)
		if err != nil {
			return report, err
		}
		report.Coverage.Merge(search.Coverage)
		for _, item := range search.Evidence {
			key := fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%s", item.Repository, item.File, item.Line, item.Name, item.Kind)
			if previous, ok := all[key]; ok {
				previous.Score += item.Score
				all[key] = previous
			} else {
				all[key] = item
			}
		}
	}
	for _, item := range all {
		report.Evidence = append(report.Evidence, item)
	}
	sort.Slice(report.Evidence, func(i, j int) bool {
		a, b := report.Evidence[i], report.Evidence[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		return fmt.Sprintf("%s/%s/%d/%s/%s", a.Repository, a.File, a.Line, a.Kind, a.Name) < fmt.Sprintf("%s/%s/%d/%s/%s", b.Repository, b.File, b.Line, b.Kind, b.Name)
	})
	report.Coverage.Matched = len(report.Evidence)
	if len(report.Evidence) > maxEvidence {
		report.Evidence = report.Evidence[:maxEvidence]
		report.Coverage.Gap("evidence_limit", true)
	}
	report.Coverage.Returned = len(report.Evidence)
	graph, err := query.LoadGraph(db)
	if err != nil {
		return report, err
	}
	for _, item := range report.Evidence {
		counts[item.Repository]++
		if isEntryPoint(item.Kind) {
			report.EntryPoints = append(report.EntryPoints, item)
		}
		if isChangePoint(item.Kind) {
			report.ChangePoints = append(report.ChangePoints, item)
		}
	}
	files := map[string]bool{}
	for _, item := range report.Evidence {
		if item.Kind != "text" {
			continue
		}
		key := item.Repository + "\x00" + item.File
		if files[key] {
			continue
		}
		files[key] = true
		related, err := query.FileEvidence(db, item.Repository, item.File)
		if err != nil {
			return report, err
		}
		for _, candidate := range related {
			if isEntryPoint(candidate.Kind) {
				report.EntryPoints = append(report.EntryPoints, candidate)
			}
			if isChangePoint(candidate.Kind) {
				report.ChangePoints = append(report.ChangePoints, candidate)
			}
		}
	}
	scores := map[string]float64{}
	fileScores := map[string]float64{}
	for _, item := range report.Evidence {
		key := item.Repository + "\x00" + item.File
		if item.Score > fileScores[key] {
			scores[item.Repository] += item.Score - fileScores[key]
			fileScores[key] = item.Score
		}
	}
	for repository, count := range counts {
		report.Repositories = append(report.Repositories, RepositoryCandidate{Repository: repository, Evidence: count, Score: scores[repository]})
	}
	entrySeen := map[string]bool{}
	var uniqueEntries []query.Evidence
	for _, e := range report.EntryPoints {
		key := fmt.Sprintf("%s/%s/%d/%s", e.Repository, e.File, e.Line, e.Name)
		if !entrySeen[key] {
			entrySeen[key] = true
			uniqueEntries = append(uniqueEntries, e)
		}
	}
	report.EntryPoints = uniqueEntries
	if len(report.EntryPoints) > maxEvidence {
		report.EntryPoints = report.EntryPoints[:maxEvidence]
		report.Coverage.Gap("entry_limit", true)
	}
	if len(report.ChangePoints) > maxEvidence {
		report.ChangePoints = report.ChangePoints[:maxEvidence]
		report.Coverage.Gap("change_point_limit", true)
	}
	traced := 0
	for _, entry := range report.EntryPoints {
		if entry.Name == "" || !isTraceTarget(entry.Kind) {
			continue
		}
		if traced == maxImpactTraces {
			report.Coverage.Gap("flow_entry_limit", true)
			break
		}
		traced++
		target := entry.Name
		for _, s := range graph.Resolve(entry.Name, entry.Repository) {
			if s.File == entry.File && s.Line == entry.Line {
				target = s.ID
				break
			}
		}
		trace, err := graph.Trace(target, entry.Repository, 6, 20)
		if err != nil {
			return report, err
		}
		if len(trace.Paths) > 0 {
			report.Impacts = append(report.Impacts, trace)
			flow, err := graph.Flows(target, entry.Repository, 6, 20)
			if err != nil {
				return report, err
			}
			report.Flows = append(report.Flows, flow)
		}
		report.Coverage.Merge(trace.Coverage)
	}
	if len(report.Evidence) == 0 {
		report.Questions = append(report.Questions, "没有找到可验证的代码证据，请确认业务词、路由或目标模块")
	}
	for _, item := range report.Evidence {
		if item.Kind == "queries_table" {
			report.Risks = append(report.Risks, "需求可能影响数据表，请确认迁移、回滚与历史数据兼容性")
			break
		}
	}
	if len(report.Repositories) > 1 {
		report.Risks = append(report.Risks, "需求涉及多个仓库，请确认接口契约和发布顺序")
	}
	report.Risks = append(report.Risks, report.Coverage.Reasons...)
	sort.Slice(report.Repositories, func(i, j int) bool {
		a, b := report.Repositories[i], report.Repositories[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		return a.Repository < b.Repository
	})
	return report, nil
}

func isEntryPoint(kind string) bool {
	return kind == "controller_method" || kind == "route" || kind == "mapper_statement"
}

func isChangePoint(kind string) bool {
	return isEntryPoint(kind) || kind == "service_method" || kind == "application_method" || kind == "calls" || kind == "queries_table"
}

func isTraceTarget(kind string) bool {
	return kind == "controller_method" || kind == "mapper_statement"
}

func keywords(text string) []string {
	var result []string
	for _, token := range tokenPattern.FindAllString(text, -1) {
		if len(token) >= 3 {
			result = append(result, token)
		}
	}
	var han []rune
	flush := func() {
		if len(han) == 2 {
			result = append(result, string(han))
		}
		for index := 0; index+2 < len(han); index++ {
			result = append(result, string(han[index:index+3]))
		}
		han = nil
	}
	for _, character := range text {
		if unicode.Is(unicode.Han, character) {
			han = append(han, character)
			continue
		}
		flush()
	}
	flush()
	return result
}
