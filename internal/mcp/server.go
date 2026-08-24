package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"

	"github.com/CJhuochai/project-brain/internal/query"
	"github.com/CJhuochai/project-brain/internal/requirement"
	"github.com/CJhuochai/project-brain/internal/storage"
	"github.com/CJhuochai/project-brain/internal/workspace"
)

type request struct {
	ID     any    `json:"id"`
	Method string `json:"method"`
	Params struct {
		Name      string            `json:"name"`
		Arguments map[string]string `json:"arguments"`
	} `json:"params"`
}

func Serve(input io.Reader, output io.Writer, root string) error {
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		var request request
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			return err
		}
		response := map[string]any{"jsonrpc": "2.0", "id": request.ID}
		result, err := handle(root, request)
		if err != nil {
			response["error"] = map[string]any{"code": -32602, "message": err.Error()}
		} else {
			response["result"] = result
		}
		if err := json.NewEncoder(output).Encode(response); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func handle(root string, request request) (any, error) {
	switch request.Method {
	case "initialize":
		return map[string]any{"protocolVersion": "2025-03-26", "serverInfo": map[string]string{"name": "project-brain", "version": "v1"}, "capabilities": map[string]any{"tools": map[string]any{}}}, nil
	case "tools/list":
		return map[string]any{"tools": tools()}, nil
	case "tools/call":
		return call(root, request.Params.Name, request.Params.Arguments)
	default:
		return nil, fmt.Errorf("unknown method: %s", request.Method)
	}
}

func tools() []map[string]any {
	return []map[string]any{
		{"name": "workspace_status", "description": "读取本地工作区索引状态"},
		{"name": "find_business_context", "description": "按关键词查找源码证据"},
		{"name": "trace_code_path", "description": "追踪下游代码关系"},
		{"name": "analyze_change_impact", "description": "分析上游影响"},
		{"name": "analyze_requirement", "description": "从需求文本列出候选仓库"},
		{"name": "get_evidence", "description": "读取指定关键词的来源证据"},
	}
}

func call(root, name string, arguments map[string]string) (any, error) {
	if name == "workspace_status" {
		repositories, err := workspace.Discover(root)
		if err != nil {
			return nil, err
		}
		return repositories, nil
	}
	workspaceDir, err := storage.WorkspaceDir(root)
	if err != nil {
		return nil, err
	}
	db, err := storage.Open(workspaceDir)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	text := arguments["text"]
	switch name {
	case "find_business_context", "get_evidence":
		return query.Search(db, text)
	case "trace_code_path":
		return query.Trace(db, text, 6)
	case "analyze_change_impact":
		return query.Impact(db, text, 6)
	case "analyze_requirement":
		return requirement.Analyze(db, text)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}
