package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"

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
	metadata := map[string]any{"snapshot_id": response.Meta.SnapshotID, "rule_revision": response.Meta.RuleRevision, "freshness": response.Meta.Freshness, "active_baseline": response.Meta.ActiveBaseline}
	if response.Meta.RefreshTarget != "" {
		metadata["refresh_target"] = response.Meta.RefreshTarget
	}
	if object, ok := result.(map[string]any); ok {
		for key, value := range metadata {
			object[key] = value
		}
		return json.NewEncoder(output).Encode(object)
	}
	metadata["result"] = result
	return json.NewEncoder(output).Encode(metadata)
}

func command(args []string) (string, map[string]any, string, error) {
	switch args[0] {
	case "index", "refresh", "status":
		return "workspace_status", map[string]any{}, "latest", nil
	case "search", "trace", "impact", "context", "contracts", "flow":
		if len(args) < 3 {
			return "", nil, "", usage()
		}
		operations := map[string]string{"search": "find_business_context", "trace": "trace_code_path", "impact": "analyze_change_impact", "context": "get_symbol_context", "contracts": "list_contracts", "flow": "trace_business_flow"}
		arguments := map[string]any{"text": args[2]}
		freshness := "stable"
		for i := 3; i < len(args); i++ {
			flag := args[i]
			if flag == "--detail" {
				if args[0] != "context" {
					return "", nil, "", fmt.Errorf("--detail is only valid for context")
				}
				arguments["view"] = "detail"
				continue
			}
			if flag == "--latest" {
				freshness = "latest"
				continue
			}
			if i+1 >= len(args) {
				return "", nil, "", usage()
			}
			i++
			switch flag {
			case "--repository":
				arguments["repository"] = args[i]
			case "--limit", "--max-depth":
				n, err := strconv.Atoi(args[i])
				if err != nil {
					return "", nil, "", fmt.Errorf("%s must be an integer", flag)
				}
				key := "limit"
				if flag == "--max-depth" {
					key = "max_depth"
				}
				arguments[key] = n
			default:
				return "", nil, "", fmt.Errorf("unknown option: %s", flag)
			}
		}
		return operations[args[0]], arguments, freshness, nil
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
	return fmt.Errorf("usage: project-brain <discover|index|refresh|status> <workspace>; project-brain <search|trace|impact|context|contracts|flow> <workspace> <target> [--repository <indexed-path>] [--limit <n>] [--max-depth <n>] [--latest] [--detail (context only)]; project-brain report <workspace> <id>; project-brain analyze <workspace> <local-input...>; project-brain feedback <workspace> <report-id> <kind> <key> <decision> <note>")
}
