package report

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/CJhuochai/project-brain/internal/input"
	"github.com/CJhuochai/project-brain/internal/requirement"
	"github.com/CJhuochai/project-brain/internal/storage"
)

type AnalysisReport struct {
	ID                  string                            `json:"id"`
	CreatedAt           string                            `json:"created_at"`
	BaselineSnapshot    string                            `json:"baseline_snapshot"`
	InputFacts          []input.Fact                      `json:"input_facts"`
	Modules             []requirement.RepositoryCandidate `json:"modules"`
	RecommendedBranches []string                          `json:"recommended_branches"`
	TestPoints          []string                          `json:"test_points"`
	Feedback            []storage.Rule                    `json:"feedback,omitempty"`
	requirement.Report
}

func AnalyzeRequirement(db *storage.DB, source input.Result) (AnalysisReport, error) {
	values := make([]string, 0, len(source.Facts))
	for _, fact := range source.Facts {
		values = append(values, fact.Value)
	}
	base, err := requirement.Analyze(db, strings.Join(values, " "))
	if err != nil {
		return AnalysisReport{}, err
	}
	report := AnalysisReport{ID: uuid.NewString(), CreatedAt: time.Now().UTC().Format(time.RFC3339), InputFacts: source.Facts, Modules: base.Repositories, Report: base}
	for _, module := range report.Modules {
		report.RecommendedBranches = append(report.RecommendedBranches, "feature/"+report.ID[:8])
		report.TestPoints = append(report.TestPoints, "验证 "+module.Repository+" 的主流程与异常路径")
	}
	if len(report.TestPoints) == 0 {
		report.TestPoints = append(report.TestPoints, "确认需求输入是否能定位到已索引代码")
	}
	rules, err := db.Rules("repository")
	if err != nil {
		return AnalysisReport{}, err
	}
	for _, rule := range rules {
		for _, module := range report.Modules {
			if module.Repository == rule.Pattern {
				report.Feedback = append(report.Feedback, rule)
			}
		}
	}
	digest := sha256.Sum256([]byte(strings.Join(values, "\n")))
	encoded, err := json.Marshal(report)
	if err != nil {
		return AnalysisReport{}, err
	}
	if err := db.SaveReport(storage.Report{ID: report.ID, CreatedAt: report.CreatedAt, BaselineSnapshot: report.BaselineSnapshot, InputDigest: hex.EncodeToString(digest[:]), JSON: string(encoded)}); err != nil {
		return AnalysisReport{}, err
	}
	return report, nil
}
