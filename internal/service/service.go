package service

import (
	"fmt"
	"math"
	"strings"

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
	for _, key := range []string{"text", "repository", "view", "source_ref", "range", "id", "report_id", "subject_kind", "subject_key", "decision", "note"} {
		if value, ok := arguments[key]; ok {
			if _, valid := value.(string); !valid {
				return nil, fmt.Errorf("%s must be a string", key)
			}
		}
	}
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
	if operation != "analyze_inputs" && operation != "analyze_change" && strings.TrimSpace(text) == "" && operation != "list_contracts" {
		return nil, fmt.Errorf("text must not be empty")
	}
	repo := stringArgument(arguments, "repository")
	maxLimit := 200
	if operation == "find_business_context" || operation == "get_evidence" {
		maxLimit = 1000
	}
	limit, err := integerArgument(arguments, "limit", 20, 1, maxLimit)
	if err != nil {
		return nil, err
	}
	depth, err := integerArgument(arguments, "max_depth", 6, 1, 32)
	if err != nil {
		return nil, err
	}
	switch operation {
	case "find_business_context", "get_evidence":
		searchLimit, err := integerArgument(arguments, "limit", 50, 1, 1000)
		if err != nil {
			return nil, err
		}
		return query.SearchRankedScoped(snapshot, text, repo, searchLimit)
	case "trace_code_path", "analyze_change_impact", "get_symbol_context", "list_contracts", "trace_business_flow":
		graph, err := query.LoadGraph(snapshot)
		if err != nil {
			return nil, err
		}
		if err := query.ValidateRepository(graph, repo); err != nil {
			return nil, err
		}
		switch operation {
		case "trace_code_path":
			return graph.Trace(text, repo, depth, limit)
		case "analyze_change_impact":
			return graph.Impact(text, repo, depth, limit)
		case "trace_business_flow":
			return graph.Flows(text, repo, depth, limit)
		case "list_contracts":
			return graph.ContractReport(repo, text, limit)
		default:
			view := stringArgument(arguments, "view")
			if view != "" && view != "summary" && view != "detail" {
				return nil, fmt.Errorf("view must be summary or detail")
			}
			return graph.Context(text, repo, depth, limit, view == "detail")
		}
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

func integerArgument(arguments map[string]any, key string, fallback, min, max int) (int, error) {
	value, ok := arguments[key]
	if !ok {
		return fallback, nil
	}
	var n float64
	switch v := value.(type) {
	case int:
		n = float64(v)
	case float64:
		n = v
	default:
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	if math.IsNaN(n) || math.IsInf(n, 0) || math.Trunc(n) != n || n < float64(min) || n > float64(max) {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", key, min, max)
	}
	return int(n), nil
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
