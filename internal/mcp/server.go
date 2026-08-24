package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/CJhuochai/project-brain/internal/change"
	"github.com/CJhuochai/project-brain/internal/indexer"
	"github.com/CJhuochai/project-brain/internal/input"
	"github.com/CJhuochai/project-brain/internal/query"
	"github.com/CJhuochai/project-brain/internal/report"
	"github.com/CJhuochai/project-brain/internal/requirement"
	"github.com/CJhuochai/project-brain/internal/storage"
	"github.com/google/uuid"
)

type request struct {
	ID     any    `json:"id"`
	Method string `json:"method"`
	Params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	} `json:"params"`
}

func Serve(input io.Reader, output io.Writer, root string) error {
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		var request request
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			return err
		}
		if strings.HasPrefix(request.Method, "notifications/") {
			continue
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
		result, err := call(root, request.Params.Name, request.Params.Arguments)
		if err != nil {
			return nil, err
		}
		text, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}
		return map[string]any{"content": []map[string]string{{"type": "text", "text": string(text)}}}, nil
	default:
		return nil, fmt.Errorf("unknown method: %s", request.Method)
	}
}

func tools() []map[string]any {
	empty := map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
	text := map[string]any{"type": "object", "properties": map[string]any{"text": map[string]string{"type": "string", "description": "需求、业务词、路由或符号"}}, "required": []string{"text"}, "additionalProperties": false}
	changeRange := map[string]any{"type": "object", "properties": map[string]any{"range": map[string]string{"type": "string", "description": "本地 Git commit 或 base..target"}}, "required": []string{"range"}, "additionalProperties": false}
	inputs := map[string]any{"type": "object", "properties": map[string]any{"text": map[string]string{"type": "string"}, "paths": map[string]any{"type": "array", "items": map[string]string{"type": "string"}}, "source_ref": map[string]string{"type": "string"}}, "additionalProperties": false}
	reportID := map[string]any{"type": "object", "properties": map[string]any{"id": map[string]string{"type": "string"}}, "required": []string{"id"}, "additionalProperties": false}
	feedback := map[string]any{"type": "object", "properties": map[string]any{"report_id": map[string]string{"type": "string"}, "subject_kind": map[string]string{"type": "string"}, "subject_key": map[string]string{"type": "string"}, "decision": map[string]any{"type": "string", "enum": []string{"confirmed", "rejected", "rule"}}, "note": map[string]string{"type": "string"}}, "required": []string{"report_id", "subject_kind", "subject_key", "decision", "note"}, "additionalProperties": false}
	readOnly := map[string]bool{"readOnlyHint": true}
	return []map[string]any{
		{"name": "workspace_status", "description": "读取本地工作区索引状态", "inputSchema": empty, "annotations": readOnly},
		{"name": "find_business_context", "description": "按关键词查找源码证据", "inputSchema": text, "annotations": readOnly},
		{"name": "trace_code_path", "description": "追踪下游代码关系", "inputSchema": text, "annotations": readOnly},
		{"name": "analyze_change_impact", "description": "分析上游影响", "inputSchema": text, "annotations": readOnly},
		{"name": "analyze_change", "description": "根据本地 Git 提交或范围定位变更文件及索引证据", "inputSchema": changeRange, "annotations": readOnly},
		{"name": "analyze_requirement", "description": "收到需求文档、原型或二次开发需求时，先用此工具定位候选项目和业务上下文", "inputSchema": text, "annotations": readOnly},
		{"name": "analyze_inputs", "description": "解析本地需求文档和交互原型，生成可追溯跨服务分析报告", "inputSchema": inputs, "annotations": readOnly},
		{"name": "get_analysis_report", "description": "按 ID 读取已保存的本地分析报告", "inputSchema": reportID, "annotations": readOnly},
		{"name": "record_analysis_feedback", "description": "将人工确认、拒绝或规则反馈回写到本地知识库", "inputSchema": feedback, "annotations": map[string]bool{"readOnlyHint": false}},
		{"name": "get_evidence", "description": "读取指定关键词的来源证据", "inputSchema": text, "annotations": readOnly},
	}
}

func call(root, name string, arguments map[string]any) (any, error) {
	workspaceDir, err := storage.WorkspaceDir(root)
	if err != nil {
		return nil, err
	}
	db, err := storage.Open(workspaceDir)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var refresh any
	if name != "get_analysis_report" && name != "record_analysis_feedback" {
		refresh, err = indexer.Refresh(root, db)
		if err != nil {
			return nil, err
		}
	}
	if name == "workspace_status" {
		return refresh, nil
	}
	text, _ := arguments["text"].(string)
	switch name {
	case "find_business_context", "get_evidence":
		return query.Search(db, text)
	case "trace_code_path":
		return query.Trace(db, text, 6)
	case "analyze_change_impact":
		return query.Impact(db, text, 6)
	case "analyze_change":
		revision, _ := arguments["range"].(string)
		return change.Analyze(root, db, revision)
	case "analyze_requirement":
		return requirement.Analyze(db, text)
	case "analyze_inputs":
		paths, err := stringSlice(arguments["paths"])
		if err != nil {
			return nil, err
		}
		source, err := input.Parse(text, paths, stringArgument(arguments, "source_ref"))
		if err != nil {
			return nil, err
		}
		return report.AnalyzeRequirement(db, source)
	case "get_analysis_report":
		return db.Report(stringArgument(arguments, "id"))
	case "record_analysis_feedback":
		item := storage.Feedback{ID: uuid.NewString(), ReportID: stringArgument(arguments, "report_id"), SubjectKind: stringArgument(arguments, "subject_kind"), SubjectKey: stringArgument(arguments, "subject_key"), Decision: stringArgument(arguments, "decision"), Note: stringArgument(arguments, "note")}
		if item.ReportID == "" || item.SubjectKind == "" || item.SubjectKey == "" || item.Decision == "" {
			return nil, fmt.Errorf("feedback fields are required")
		}
		if err := db.RecordFeedback(item); err != nil {
			return nil, err
		}
		return item, nil
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
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
