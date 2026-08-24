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
		report.Evidence = append(report.Evidence, items...)
		for _, item := range items {
			counts[item.Repository]++
			if item.Kind == "controller_method" || item.Kind == "route" || item.Kind == "mapper_statement" {
				report.EntryPoints = append(report.EntryPoints, item)
			}
			if item.Kind != "text" {
				report.ChangePoints = append(report.ChangePoints, item)
			}
		}
	}
	for repository, count := range counts {
		report.Repositories = append(report.Repositories, RepositoryCandidate{Repository: repository, Evidence: count})
	}
	for _, entry := range report.EntryPoints {
		if entry.Name == "" {
			continue
		}
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

func keywords(text string) []string {
	var result []string
	for _, token := range tokenPattern.FindAllString(text, -1) {
		if len(token) >= 3 {
			result = append(result, token)
		}
	}
	var han []rune
	flush := func() {
		for index := 0; index+1 < len(han); index++ {
			result = append(result, string(han[index:index+2]))
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
