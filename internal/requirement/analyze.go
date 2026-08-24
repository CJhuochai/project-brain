package requirement

import (
	"regexp"
	"sort"
	"unicode"

	"github.com/CJhuochai/project-brain/internal/query"
	"github.com/CJhuochai/project-brain/internal/storage"
)

type RepositoryCandidate struct {
	Repository string `json:"repository"`
	Evidence   int    `json:"evidence_count"`
}

type Report struct {
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
	report := Report{}
	counts := map[string]int{}
	seen := map[string]bool{}
	for _, token := range keywords(text) {
		if seen[token] {
			continue
		}
		seen[token] = true
		report.Keywords = append(report.Keywords, token)
		items, err := query.Search(db, token)
		if err != nil {
			return report, err
		}
		if remaining := maxEvidence - len(report.Evidence); remaining > 0 {
			if len(items) > remaining {
				items = items[:remaining]
			}
			report.Evidence = append(report.Evidence, items...)
		} else {
			items = nil
		}
		for _, item := range items {
			counts[item.Repository]++
			if isEntryPoint(item.Kind) {
				report.EntryPoints = append(report.EntryPoints, item)
			}
			if isChangePoint(item.Kind) {
				report.ChangePoints = append(report.ChangePoints, item)
			}
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
	for repository, count := range counts {
		report.Repositories = append(report.Repositories, RepositoryCandidate{Repository: repository, Evidence: count})
	}
	traced := 0
	for _, entry := range report.EntryPoints {
		if entry.Name == "" || !isTraceTarget(entry.Kind) {
			continue
		}
		if traced == maxImpactTraces {
			break
		}
		traced++
		trace, err := query.Trace(db, entry.Name, 6)
		if err != nil {
			return report, err
		}
		if len(trace.Paths) > 0 {
			report.Impacts = append(report.Impacts, trace)
		}
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
	sort.Slice(report.Repositories, func(i, j int) bool { return report.Repositories[i].Evidence > report.Repositories[j].Evidence })
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
