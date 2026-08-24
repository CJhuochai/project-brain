package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func DataDir() (string, error) {
	if runtime.GOOS == "windows" {
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			return filepath.Join(localAppData, "ProjectBrain"), nil
		}
	}
	if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
		return filepath.Join(dataHome, "project-brain"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "ProjectBrain"), nil
	}
	return filepath.Join(home, ".local", "share", "project-brain"), nil
}

func WorkspaceDir(root string) (string, error) {
	dataDir, err := DataDir()
	if err != nil {
		return "", err
	}
	normalized := strings.ToLower(strings.TrimRight(filepath.Clean(root), `\\/`))
	digest := sha256.Sum256([]byte(normalized))
	return filepath.Join(dataDir, "workspaces", hex.EncodeToString(digest[:8])), nil
}
