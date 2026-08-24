package storage

import (
	"path/filepath"
	"testing"
)

func TestWorkspaceDirUsesLocalAppDataAndStablePathHash(t *testing.T) {
	t.Setenv("LOCALAPPDATA", `C:\\Users\\tester\\AppData\\Local`)
	t.Setenv("XDG_DATA_HOME", "")

	first, err := WorkspaceDir(`E:\\BasisProject`)
	if err != nil {
		t.Fatal(err)
	}
	second, err := WorkspaceDir(`e:\\basisproject\\`)
	if err != nil {
		t.Fatal(err)
	}

	wantPrefix := filepath.Join(`C:\\Users\\tester\\AppData\\Local`, "ProjectBrain", "workspaces")
	if filepath.Dir(first) != wantPrefix {
		t.Fatalf("workspace dir = %q, want parent %q", first, wantPrefix)
	}
	if first != second {
		t.Fatalf("equivalent workspace paths produced different directories: %q and %q", first, second)
	}
}
