package service

import (
	"fmt"

	"github.com/CJhuochai/project-brain/internal/change"
	"github.com/CJhuochai/project-brain/internal/input"
	"github.com/CJhuochai/project-brain/internal/query"
	"github.com/CJhuochai/project-brain/internal/report"
	"github.com/CJhuochai/project-brain/internal/requirement"
	"github.com/CJhuochai/project-brain/internal/storage"
	"github.com/google/uuid"
)

type Evidence = query.Evidence
type Provenance = report.Provenance

type FeedbackResult struct {
	storage.Feedback
	RuleRevision int64 `json:"rule_revision"`
}

// Execute performs one public operation against the caller-selected snapshot.
// The mutable report and feedback state is always kept in control.
func Execute(root string, snapshot *storage.DB, control *storage.Control, operation string, arguments map[string]any, provenance Provenance) (any, error) {
	switch operation {
	case "get_analysis_report":
		return control.Report(stringArgument(arguments, "id"))
	case "record_analysis_feedback":
		feedback := storage.Feedback{ID: uuid.NewString(), ReportID: stringArgument(arguments, "report_id"), SubjectKind: stringArgument(arguments, "subject_kind"), SubjectKey: stringArgument(arguments, "subject_key"), Decision: stringArgument(arguments, "decision"), Note: stringArgument(arguments, "note")}
		if feedback.ReportID == "" || feedback.SubjectKind == "" || feedback.SubjectKey == "" || feedback.Decision == "" {
			return nil, fmt.Errorf("feedback fields are required")
		}
		revision, err := control.RecordFeedback(feedback)
		if err != nil {
			return nil, err
		}
		return FeedbackResult{Feedback: feedback, RuleRevision: revision}, nil
	}
	if snapshot == nil {
		return nil, fmt.Errorf("snapshot is required for %s", operation)
	}
	text := stringArgument(arguments, "text")
	switch operation {
	case "find_business_context", "get_evidence":
		return query.Search(snapshot, text)
	case "trace_code_path":
		return query.Trace(snapshot, text, 6)
	case "analyze_change_impact":
		return query.Impact(snapshot, text, 6)
	case "analyze_change":
		return change.Analyze(root, snapshot, stringArgument(arguments, "range"))
	case "analyze_requirement":
		return requirement.Analyze(snapshot, text)
	case "analyze_inputs":
		paths, err := stringSlice(arguments["paths"])
		if err != nil {
			return nil, err
		}
		source, err := input.Parse(text, paths, stringArgument(arguments, "source_ref"))
		if err != nil {
			return nil, err
		}
		return report.AnalyzeRequirementAt(snapshot, control, source, provenance)
	default:
		return nil, fmt.Errorf("unknown tool: %s", operation)
	}
}

func stringArgument(arguments map[string]any, key string) string {
	value, _ := arguments[key].(string)
	return value
}

func stringSlice(value any) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("paths must be an array of strings")
	}
	paths := make([]string, 0, len(items))
	for _, item := range items {
		path, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("paths must be an array of strings")
		}
		paths = append(paths, path)
	}
	return paths, nil
}
