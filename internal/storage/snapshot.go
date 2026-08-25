package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const snapshotReserveBytes uint64 = 128 * 1024 * 1024

var freeSpace = diskFreeSpace

// PrepareStaging copies the active immutable snapshot to a private file. The
// returned cleanup is safe to call after either activation or an index error.
func PrepareStaging(control *Control) (Snapshot, func(), error) {
	active, err := control.ActiveSnapshot()
	if err != nil {
		return Snapshot{}, nil, err
	}
	if err := requireFreeSpace(control.workspaceDir, active.Path); err != nil {
		return Snapshot{}, nil, err
	}
	id, err := nextSnapshotID(control)
	if err != nil {
		return Snapshot{}, nil, err
	}
	staging := Snapshot{ID: id, Path: filepath.Join(control.workspaceDir, id+".sqlite.staging"), Baseline: active.Baseline}
	if err := copyFile(active.Path, staging.Path); err != nil {
		return Snapshot{}, nil, err
	}
	return staging, func() { _ = os.Remove(staging.Path) }, nil
}

// ActivateStaging checkpoints its writable WAL, renames it to its immutable
// filename, and changes the active pointer in Control's short transaction.
func ActivateStaging(control *Control, staging Snapshot) (Snapshot, error) {
	if !strings.HasSuffix(staging.Path, ".sqlite.staging") {
		return Snapshot{}, fmt.Errorf("invalid staging snapshot path: %s", staging.Path)
	}
	stagingDB, err := OpenSnapshot(staging.Path, true)
	if err != nil {
		return Snapshot{}, err
	}
	if _, err := stagingDB.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		_ = stagingDB.Close()
		return Snapshot{}, err
	}
	if err := stagingDB.Close(); err != nil {
		return Snapshot{}, err
	}
	if err := validateSnapshot(staging.Path); err != nil {
		return Snapshot{}, err
	}
	completedPath := strings.TrimSuffix(staging.Path, ".staging")
	if err := os.Rename(staging.Path, completedPath); err != nil {
		return Snapshot{}, err
	}
	staging.Path = completedPath
	staging.Completed = time.Now().UTC().Format(time.RFC3339)
	if err := control.ActivateSnapshot(staging); err != nil {
		return Snapshot{}, err
	}
	_ = CleanupSnapshots(control)
	return staging, nil
}

func CleanupSnapshots(control *Control) error {
	rows, err := control.Query(`SELECT id, path FROM snapshots ORDER BY completed DESC, id DESC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type candidate struct{ id, path string }
	var snapshots []candidate
	for rows.Next() {
		var snapshot candidate
		if err := rows.Scan(&snapshot.id, &snapshot.path); err != nil {
			return err
		}
		snapshots = append(snapshots, snapshot)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, snapshot := range snapshots[2:] {
		if err := os.Remove(snapshot.path); err != nil && !os.IsNotExist(err) {
			continue // a Windows reader can still hold the previous file
		}
		if _, err := control.Exec(`DELETE FROM snapshots WHERE id = ?`, snapshot.id); err != nil {
			return err
		}
	}
	return nil
}

func RemoveStaging(workspaceDir string) error {
	entries, err := os.ReadDir(workspaceDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.HasPrefix(entry.Name(), "snapshot-") && strings.HasSuffix(entry.Name(), ".sqlite.staging") {
			if err := os.Remove(filepath.Join(workspaceDir, entry.Name())); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}

func nextSnapshotID(control *Control) (string, error) {
	rows, err := control.Query(`SELECT id FROM snapshots`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	max := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", err
		}
		value, err := strconv.Atoi(strings.TrimPrefix(id, "snapshot-"))
		if err == nil && value > max {
			max = value
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return fmt.Sprintf("snapshot-%03d", max+1), nil
}

func requireFreeSpace(directory, activePath string) error {
	info, err := os.Stat(activePath)
	if err != nil {
		return err
	}
	required := uint64(info.Size())*2 + snapshotReserveBytes
	available, err := freeSpace(directory)
	if err != nil {
		return err
	}
	if available < required {
		return fmt.Errorf("insufficient disk space: need %d bytes, have %d bytes", required, available)
	}
	return nil
}

func copyFile(source, target string) (err error) {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	temporary := target + ".copying"
	output, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		_ = output.Close()
		if err != nil {
			_ = os.Remove(temporary)
		}
	}()
	if _, err = io.Copy(output, input); err != nil {
		return err
	}
	if err = output.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, target)
}

func validateSnapshot(path string) error {
	db, err := OpenSnapshot(path, false)
	if err != nil {
		return err
	}
	defer db.Close()
	var count int
	return db.QueryRow(`SELECT count(*) FROM files`).Scan(&count)
}

func snapshotPaths(workspaceDir string) ([]string, error) {
	entries, err := os.ReadDir(workspaceDir)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.HasPrefix(entry.Name(), "snapshot-") && strings.HasSuffix(entry.Name(), ".sqlite") {
			paths = append(paths, filepath.Join(workspaceDir, entry.Name()))
		}
	}
	sort.Strings(paths)
	return paths, nil
}
