package mcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestServeListsToolsAndRejectsUnknownTool(t *testing.T) {
	input := strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\"}\n{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/list\"}\n{\"jsonrpc\":\"2.0\",\"id\":3,\"method\":\"tools/call\",\"params\":{\"name\":\"missing\"}}\n")
	var output bytes.Buffer
	if err := Serve(input, &output, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "find_business_context") || !strings.Contains(text, "unknown tool") {
		t.Fatalf("output=%s", text)
	}
}

func TestServeIgnoresInitializedNotificationAndPublishesInputSchemas(t *testing.T) {
	input := strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{}}\n{\"jsonrpc\":\"2.0\",\"method\":\"notifications/initialized\",\"params\":{}}\n{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/list\",\"params\":{}}\n")
	var output bytes.Buffer
	if err := Serve(input, &output, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	lines := strings.Fields(strings.TrimSpace(output.String()))
	if len(lines) != 2 {
		t.Fatalf("notification must not receive a response: %s", output.String())
	}
	var response struct {
		Result struct {
			Tools []struct {
				Name        string         `json:"name"`
				InputSchema map[string]any `json:"inputSchema"`
				Annotations map[string]any `json:"annotations"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &response); err != nil {
		t.Fatal(err)
	}
	for _, tool := range response.Result.Tools {
		if tool.InputSchema["type"] != "object" || tool.Annotations["readOnlyHint"] != true {
			t.Fatalf("tool %s missing MCP contract: %#v", tool.Name, tool)
		}
	}
}
