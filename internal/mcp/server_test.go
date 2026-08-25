package mcp

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CJhuochai/project-brain/internal/coordinator"
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

func TestWorkspaceStatusRefreshesChangedBaseline(t *testing.T) {
	root := createBaselineRepository(t)
	t.Setenv("LOCALAPPDATA", t.TempDir())
	text := workspaceStatus(t, root)
	if !strings.Contains(text, `"index_state":"refreshed"`) {
		t.Fatalf("status=%s", text)
	}
}

func TestWorkspaceStatusSkipsUnchangedBaseline(t *testing.T) {
	root := createBaselineRepository(t)
	t.Setenv("LOCALAPPDATA", t.TempDir())
	workspaceStatus(t, root)
	text := workspaceStatus(t, root)
	if !strings.Contains(text, `"index_state":"up_to_date"`) {
		t.Fatalf("status=%s", text)
	}
}

func TestWorkspaceStatusReportsPerRepositoryFreshness(t *testing.T) {
	root := createBaselineRepository(t)
	t.Setenv("LOCALAPPDATA", t.TempDir())
	text := workspaceStatus(t, root)
	var status struct {
		Repositories []struct {
			BaselineCommit  string `json:"baseline_commit"`
			DiagnosticCount int    `json:"diagnostic_count"`
			FileCount       int    `json:"file_count"`
			IndexedAt       string `json:"indexed_at"`
			IndexState      string `json:"index_state"`
		} `json:"repositories"`
	}
	if err := json.Unmarshal([]byte(text), &status); err != nil {
		t.Fatal(err)
	}
	if len(status.Repositories) != 1 {
		t.Fatalf("repositories=%#v", status.Repositories)
	}
	repository := status.Repositories[0]
	if repository.IndexState != "refreshed" || repository.FileCount != 1 || repository.BaselineCommit == "" || repository.IndexedAt == "" || repository.DiagnosticCount != 0 {
		t.Fatalf("repository status=%#v", repository)
	}
}

func workspaceStatus(t *testing.T, root string) string {
	t.Helper()
	server, err := coordinator.Start(root)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	var output bytes.Buffer
	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"workspace_status","arguments":{"freshness":"latest"}}}` + "\n")
	if err := Serve(input, &output, root); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response.Result.Content[0].Text
}

func createBaselineRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	repo := filepath.Join(root, "service")
	runGit(t, root, "init", "-b", "main", repo)
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "Main.java"), []byte("class Main {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "initial")
	commit := runGit(t, repo, "rev-parse", "HEAD")
	runGit(t, repo, "remote", "add", "origin", "https://example.invalid/service.git")
	runGit(t, repo, "update-ref", "refs/remotes/origin/main", commit)
	runGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	return root
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func TestServeIgnoresInitializedNotificationAndPublishesInputSchemas(t *testing.T) {
	input := strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{}}\n{\"jsonrpc\":\"2.0\",\"method\":\"notifications/initialized\",\"params\":{}}\n{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/list\",\"params\":{}}\n")
	var output bytes.Buffer
	if err := Serve(input, &output, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
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
		readOnly := tool.Name != "record_analysis_feedback"
		if tool.InputSchema["type"] != "object" || tool.Annotations["readOnlyHint"] != readOnly {
			t.Fatalf("tool %s missing MCP contract: %#v", tool.Name, tool)
		}
	}
}

func TestToolsExposeReportLoopContracts(t *testing.T) {
	listed := tools()
	for _, want := range []struct {
		name     string
		readOnly bool
	}{{"analyze_inputs", true}, {"get_analysis_report", true}, {"record_analysis_feedback", false}} {
		found := false
		for _, tool := range listed {
			if tool["name"] == want.name && tool["annotations"].(map[string]bool)["readOnlyHint"] == want.readOnly {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing tool %s: %#v", want.name, listed)
		}
	}
}

// Break caught: omitting freshness/snapshot selection would let an MCP caller
// unknowingly read a different index view than another concurrent session.
func TestReadToolsExposeSnapshotSelection(t *testing.T) {
	for _, tool := range tools() {
		if tool["name"] == "record_analysis_feedback" {
			continue
		}
		properties := tool["inputSchema"].(map[string]any)["properties"].(map[string]any)
		if _, ok := properties["freshness"]; !ok {
			t.Fatalf("%s missing freshness: %#v", tool["name"], properties)
		}
		if _, ok := properties["snapshot_id"]; !ok {
			t.Fatalf("%s missing snapshot_id: %#v", tool["name"], properties)
		}
	}
}
