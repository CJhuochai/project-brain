package requirement

import (
	"regexp"
	"sort"

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
}

var tokenPattern = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_/-]*`)

func Analyze(db *storage.DB, text string) (Report, error) {
	report := Report{}
	counts := map[string]int{}
	seen := map[string]bool{}
	for _, token := range tokenPattern.FindAllString(text, -1) {
		if seen[token] || len(token) < 3 {
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
		}
	}
	for repository, count := range counts {
		report.Repositories = append(report.Repositories, RepositoryCandidate{Repository: repository, Evidence: count})
	}
	sort.Slice(report.Repositories, func(i, j int) bool { return report.Repositories[i].Evidence > report.Repositories[j].Evidence })
	return report, nil
}
