package app

import "testing"

func TestV2CommandOptions(t *testing.T) {
	for name, want := range map[string]string{"context": "get_symbol_context", "contracts": "list_contracts", "flow": "trace_business_flow"} {
		op, args, freshness, err := command([]string{name, "workspace", "p.Controller.run", "--repository", "repo", "--limit", "4", "--max-depth", "2", "--latest"})
		if err != nil || op != want || args["repository"] != "repo" || args["limit"] != 4 || args["max_depth"] != 2 || freshness != "latest" {
			t.Fatalf("op=%s args=%#v freshness=%s err=%v", op, args, freshness, err)
		}
	}
	_, args, _, err := command([]string{"context", "workspace", "p.Controller.run", "--detail"})
	if err != nil || args["view"] != "detail" {
		t.Fatalf("detail=%#v err=%v", args, err)
	}
	for _, args := range [][]string{{"flow", "workspace", "x", "--detail"}, {"context", "workspace", "x", "--unknown", "a"}, {"context", "workspace", "x", "--limit", "bad"}, {"context", "workspace", "x", "--repository"}} {
		if _, _, _, err := command(args); err == nil {
			t.Fatalf("accepted invalid args: %#v", args)
		}
	}
}
