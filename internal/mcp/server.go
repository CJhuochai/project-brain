package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/CJhuochai/project-brain/internal/coordinator"
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
	empty := readSchema(map[string]any{})
	text := readSchema(map[string]any{"text": map[string]string{"type": "string", "description": "需求、业务词、路由或符号"}})
	changeRange := readSchema(map[string]any{"range": map[string]string{"type": "string", "description": "本地 Git commit 或 base..target"}})
	inputs := readSchema(map[string]any{"text": map[string]string{"type": "string"}, "paths": map[string]any{"type": "array", "items": map[string]string{"type": "string"}}, "source_ref": map[string]string{"type": "string"}})
	reportID := readSchema(map[string]any{"id": map[string]string{"type": "string"}})
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

func readSchema(properties map[string]any) map[string]any {
	properties["freshness"] = map[string]any{"type": "string", "enum": []string{"stable", "latest"}, "description": "默认 stable；latest 等待合并刷新"}
	properties["snapshot_id"] = map[string]string{"type": "string", "description": "固定读取已完成快照"}
	return map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
}

func call(root, name string, arguments map[string]any) (any, error) {
	if !knownTool(name) {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	client, err := coordinator.Ensure(root, os.Args[0])
	if err != nil {
		return nil, err
	}
	response, err := client.Call(context.Background(), coordinator.Request{Operation: name, Arguments: arguments, Freshness: freshnessArgument(arguments), SnapshotID: stringArgument(arguments, "snapshot_id")})
	if err != nil {
		return nil, err
	}
	var result any
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return nil, err
	}
	metadata := map[string]any{"snapshot_id": response.Meta.SnapshotID, "rule_revision": response.Meta.RuleRevision, "freshness": response.Meta.Freshness, "active_baseline": response.Meta.ActiveBaseline}
	if response.Meta.RefreshTarget != "" {
		metadata["refresh_target"] = response.Meta.RefreshTarget
	}
	if object, ok := result.(map[string]any); ok {
		for key, value := range metadata {
			object[key] = value
		}
		return object, nil
	}
	metadata["result"] = result
	return metadata, nil
}

func knownTool(name string) bool {
	for _, tool := range tools() {
		if tool["name"] == name {
			return true
		}
	}
	return false
}

func freshnessArgument(arguments map[string]any) string {
	value := stringArgument(arguments, "freshness")
	if value == "latest" {
		return value
	}
	return "stable"
}

func stringArgument(arguments map[string]any, key string) string {
	value, _ := arguments[key].(string)
	return value
}
