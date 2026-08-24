package mcp

import (
	"bytes"
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
