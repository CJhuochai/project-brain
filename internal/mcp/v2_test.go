package mcp

import "testing"

func TestV2RejectsMalformedSnapshotAndFreshness(t *testing.T) {
	for _, args := range []map[string]any{{"freshness": 42}, {"freshness": "old"}, {"snapshot_id": 42}} {
		if _, err := call("", "get_symbol_context", args); err == nil {
			t.Fatalf("accepted %#v", args)
		}
	}
}

func TestV2ToolsExposeReadOnlyScopedContracts(t *testing.T) {
	for _, name := range []string{"get_symbol_context", "list_contracts", "trace_business_flow"} {
		found := false
		for _, tool := range tools() {
			if tool["name"] != name {
				continue
			}
			found = true
			if !tool["annotations"].(map[string]bool)["readOnlyHint"] {
				t.Fatalf("%s mutating", name)
			}
			props := tool["inputSchema"].(map[string]any)["properties"].(map[string]any)
			for _, key := range []string{"repository", "limit", "max_depth", "freshness", "snapshot_id"} {
				if props[key] == nil {
					t.Fatalf("%s missing %s", name, key)
				}
			}
		}
		if !found {
			t.Fatalf("missing %s", name)
		}
	}
}
