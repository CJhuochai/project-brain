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
	SnapshotID          string                            `json:"snapshot_id,omitempty"`
	SnapshotCompleted   string                            `json:"snapshot_completed,omitempty"`
	RuleRevision        int64                             `json:"rule_revision"`
	InputFacts          []input.Fact                      `json:"input_facts"`
	Modules             []requirement.RepositoryCandidate `json:"modules"`
	RecommendedBranches []string                          `json:"recommended_branches"`
	TestPoints          []string                          `json:"test_points"`
	Feedback            []storage.Rule                    `json:"feedback,omitempty"`
	InputDiagnostics    []input.Diagnostic                `json:"input_diagnostics,omitempty"`
	requirement.Report
}

type Provenance struct {
	SnapshotID        string
	SnapshotCompleted string
	Baselines         string
	RuleRevision      int64
}

func AnalyzeRequirement(db *storage.DB, source input.Result) (AnalysisReport, error) {
	rules, err := db.Rules("repository")
	if err != nil {
		return AnalysisReport{}, err
	}
	report, err := analyze(db, source, rules, Provenance{})
	if err != nil {
		return AnalysisReport{}, err
	}
	if err := db.SaveReport(storage.Report{ID: report.ID, CreatedAt: report.CreatedAt, BaselineSnapshot: report.BaselineSnapshot, InputDigest: reportDigest(source), JSON: reportJSON(report)}); err != nil {
		return AnalysisReport{}, err
	}
	return report, nil
}

// AnalyzeRequirementAt reads one immutable index snapshot and records the
// resulting report in the mutable control database.
func AnalyzeRequirementAt(db *storage.DB, control *storage.Control, source input.Result, provenance Provenance) (AnalysisReport, error) {
	rules, err := control.RulesAtRevision("repository", provenance.RuleRevision)
	if err != nil {
		return AnalysisReport{}, err
	}
	report, err := analyze(db, source, rules, provenance)
	if err != nil {
		return AnalysisReport{}, err
	}
	if err := control.SaveReport(storage.Report{ID: report.ID, CreatedAt: report.CreatedAt, BaselineSnapshot: report.BaselineSnapshot, InputDigest: reportDigest(source), JSON: reportJSON(report)}); err != nil {
		return AnalysisReport{}, err
	}
	return report, nil
}

func analyze(db *storage.DB, source input.Result, rules []storage.Rule, provenance Provenance) (AnalysisReport, error) {
	values := make([]string, 0, len(source.Facts))
	for _, fact := range source.Facts {
		values = append(values, fact.Value)
	}
	base, err := requirement.Analyze(db, strings.Join(values, " "))
	if err != nil {
		return AnalysisReport{}, err
	}
	snapshots, err := db.BaselineSnapshots()
	if err != nil {
		return AnalysisReport{}, err
	}
	baseline := strings.Join(snapshots, ",")
	if provenance.Baselines != "" {
		baseline = provenance.Baselines
	}
	report := AnalysisReport{ID: uuid.NewString(), CreatedAt: time.Now().UTC().Format(time.RFC3339), BaselineSnapshot: baseline, SnapshotID: provenance.SnapshotID, SnapshotCompleted: provenance.SnapshotCompleted, RuleRevision: provenance.RuleRevision, InputFacts: source.Facts, InputDiagnostics: source.Diagnostics, Modules: base.Repositories, Report: base}
	if len(report.Modules) > 0 {
		report.RecommendedBranches = append(report.RecommendedBranches, "feature/"+report.ID[:8])
	}
	for _, module := range report.Modules {
		report.TestPoints = append(report.TestPoints, "验证 "+module.Repository+" 的主流程与异常路径")
	}
	for _, diagnostic := range source.Diagnostics {
		report.Risks = append(report.Risks, diagnostic.Message)
	}
	if len(report.TestPoints) == 0 {
		report.TestPoints = append(report.TestPoints, "确认需求输入是否能定位到已索引代码")
	}
	for _, rule := range rules {
		for _, module := range report.Modules {
			if module.Repository == rule.Pattern {
				report.Feedback = append(report.Feedback, rule)
			}
		}
	}
	return report, nil
}

func reportDigest(source input.Result) string {
	values := make([]string, 0, len(source.Facts))
	for _, fact := range source.Facts {
		values = append(values, fact.Value)
	}
	digest := sha256.Sum256([]byte(strings.Join(values, "\n")))
	return hex.EncodeToString(digest[:])
}

func reportJSON(report AnalysisReport) string {
	encoded, _ := json.Marshal(report)
	return string(encoded)
}
