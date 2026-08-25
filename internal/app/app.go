package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/CJhuochai/project-brain/internal/coordinator"
	"github.com/CJhuochai/project-brain/internal/workspace"
)

func Run(args []string, output io.Writer) error {
	if len(args) < 2 {
		return usage()
	}
	if args[0] == "discover" {
		repositories, err := workspace.Discover(args[1])
		if err != nil {
			return err
		}
		return json.NewEncoder(output).Encode(repositories)
	}
	operation, arguments, freshness, err := command(args)
	if err != nil {
		return err
	}
	client, err := coordinator.Ensure(args[1], os.Args[0])
	if err != nil {
		return err
	}
	response, err := client.Call(context.Background(), coordinator.Request{Operation: operation, Arguments: arguments, Freshness: freshness})
	if err != nil {
		return err
	}
	var result any
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(result)
}

func command(args []string) (string, map[string]any, string, error) {
	switch args[0] {
	case "index", "refresh", "status":
		return "workspace_status", map[string]any{}, "latest", nil
	case "search", "trace", "impact":
		if len(args) != 3 {
			return "", nil, "", usage()
		}
		operations := map[string]string{"search": "find_business_context", "trace": "trace_code_path", "impact": "analyze_change_impact"}
		return operations[args[0]], map[string]any{"text": args[2]}, "stable", nil
	case "analyze":
		if len(args) < 3 {
			return "", nil, "", usage()
		}
		paths := make([]any, 0, len(args)-2)
		for _, path := range args[2:] {
			paths = append(paths, path)
		}
		return "analyze_inputs", map[string]any{"paths": paths}, "latest", nil
	case "report":
		if len(args) != 3 {
			return "", nil, "", usage()
		}
		return "get_analysis_report", map[string]any{"id": args[2]}, "stable", nil
	case "feedback":
		if len(args) != 7 {
			return "", nil, "", usage()
		}
		return "record_analysis_feedback", map[string]any{"report_id": args[2], "subject_kind": args[3], "subject_key": args[4], "decision": args[5], "note": args[6]}, "stable", nil
	default:
		return "", nil, "", usage()
	}
}

func usage() error {
	return fmt.Errorf("usage: project-brain <discover|index|refresh|status> <workspace>; project-brain <search|trace|impact|report> <workspace> <target>; project-brain analyze <workspace> <local-input...>; project-brain feedback <workspace> <report-id> <kind> <key> <decision> <note>")
}
