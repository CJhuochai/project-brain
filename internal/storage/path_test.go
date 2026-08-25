package storage

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestWorkspaceDirUsesLocalAppDataAndStablePathHash(t *testing.T) {
	t.Setenv("LOCALAPPDATA", `C:\\Users\\tester\\AppData\\Local`)
	wantDataDir := filepath.Join(`C:\\Users\\tester\\AppData\\Local`, "ProjectBrain")
	if runtime.GOOS != "windows" {
		t.Setenv("XDG_DATA_HOME", "/tmp/project-brain-test")
		wantDataDir = filepath.Join("/tmp/project-brain-test", "project-brain")
	} else {
		t.Setenv("XDG_DATA_HOME", "")
	}

	first, err := WorkspaceDir(`E:\\ExampleWorkspace`)
	if err != nil {
		t.Fatal(err)
	}
	second, err := WorkspaceDir(`e:\\exampleworkspace\\`)
	if err != nil {
		t.Fatal(err)
	}

	wantPrefix := filepath.Join(wantDataDir, "workspaces")
	if filepath.Dir(first) != wantPrefix {
		t.Fatalf("workspace dir = %q, want parent %q", first, wantPrefix)
	}
	if first != second {
		t.Fatalf("equivalent workspace paths produced different directories: %q and %q", first, second)
	}
}
