package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Snapshot is a complete index database. Once Active is set through Control,
// its file is only opened read-only by query clients.
type Snapshot struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	Completed string `json:"completed"`
	Baseline  string `json:"baseline"`
}

// Control owns mutable workspace metadata. Source evidence never lives here.
type Control struct {
	*sql.DB
	workspaceDir string
}

func OpenControl(workspaceDir string) (*Control, error) {
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(workspaceDir, "control.sqlite"))
	if err != nil {
		return nil, err
	}
	control := &Control{DB: db, workspaceDir: workspaceDir}
	if _, err := control.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;`); err != nil {
		_ = control.Close()
		return nil, err
	}
	if err := control.initializeAndMigrate(); err != nil {
		_ = control.Close()
		return nil, err
	}
	return control, nil
}

func (control *Control) initializeAndMigrate() error {
	if _, err := control.Exec(`
CREATE TABLE IF NOT EXISTS snapshots (
  id TEXT PRIMARY KEY,
  path TEXT NOT NULL,
  completed TEXT NOT NULL,
  baseline TEXT NOT NULL,
  active INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS reports (
  id TEXT PRIMARY KEY,
  created_at TEXT NOT NULL,
  baseline_snapshot TEXT NOT NULL,
  input_digest TEXT NOT NULL,
  report_json TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS feedback (
  id TEXT PRIMARY KEY,
  report_id TEXT NOT NULL,
  subject_kind TEXT NOT NULL,
  subject_key TEXT NOT NULL,
  decision TEXT NOT NULL,
  note TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS rules (
  id INTEGER PRIMARY KEY,
  kind TEXT NOT NULL,
  pattern TEXT NOT NULL,
  target TEXT NOT NULL,
  confidence TEXT NOT NULL,
  note TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(kind, pattern, target)
);
CREATE TABLE IF NOT EXISTS control_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
`); err != nil {
		return err
	}
	var count int
	if err := control.QueryRow(`SELECT count(*) FROM snapshots WHERE active = 1`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return control.migrateLegacyIndex()
}

func (control *Control) migrateLegacyIndex() error {
	legacyPath := filepath.Join(control.workspaceDir, "index.sqlite")
	snapshot := Snapshot{ID: "snapshot-001", Path: filepath.Join(control.workspaceDir, "snapshot-001.sqlite"), Completed: time.Now().UTC().Format(time.RFC3339)}
	if _, err := os.Stat(legacyPath); errors.Is(err, os.ErrNotExist) {
		index, err := OpenSnapshot(snapshot.Path, true)
		if err != nil {
			return err
		}
		if err := index.Close(); err != nil {
			return err
		}
		return control.ActivateSnapshot(snapshot)
	} else if err != nil {
		return err
	}
	if err := os.Rename(legacyPath, snapshot.Path); err != nil {
		return fmt.Errorf("migrate v1.1 index: %w", err)
	}
	legacy, err := OpenSnapshot(snapshot.Path, false)
	if err != nil {
		return err
	}
	defer legacy.Close()
	transaction, err := control.Begin()
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if err := copyLegacyReports(transaction, legacy); err != nil {
		return err
	}
	if err := copyLegacyFeedback(transaction, legacy); err != nil {
		return err
	}
	if err := copyLegacyRules(transaction, legacy); err != nil {
		return err
	}
	if _, err := transaction.Exec(`INSERT INTO snapshots(id, path, completed, baseline, active) VALUES(?, ?, ?, ?, 1)`, snapshot.ID, snapshot.Path, snapshot.Completed, snapshot.Baseline); err != nil {
		return err
	}
	var revision int64
	if err := transaction.QueryRow(`SELECT count(*) FROM rules`).Scan(&revision); err != nil {
		return err
	}
	if _, err := transaction.Exec(`INSERT INTO control_meta(key, value) VALUES('rule_revision', ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, revision); err != nil {
		return err
	}
	return transaction.Commit()
}

func copyLegacyReports(transaction *sql.Tx, legacy *DB) error {
	rows, err := legacy.Query(`SELECT id, created_at, baseline_snapshot, input_digest, report_json FROM reports`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item Report
		if err := rows.Scan(&item.ID, &item.CreatedAt, &item.BaselineSnapshot, &item.InputDigest, &item.JSON); err != nil {
			return err
		}
		if _, err := transaction.Exec(`INSERT OR IGNORE INTO reports(id, created_at, baseline_snapshot, input_digest, report_json) VALUES(?, ?, ?, ?, ?)`, item.ID, item.CreatedAt, item.BaselineSnapshot, item.InputDigest, item.JSON); err != nil {
			return err
		}
	}
	return rows.Err()
}

func copyLegacyFeedback(transaction *sql.Tx, legacy *DB) error {
	rows, err := legacy.Query(`SELECT id, report_id, subject_kind, subject_key, decision, note, created_at FROM feedback`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item Feedback
		if err := rows.Scan(&item.ID, &item.ReportID, &item.SubjectKind, &item.SubjectKey, &item.Decision, &item.Note, &item.CreatedAt); err != nil {
			return err
		}
		if _, err := transaction.Exec(`INSERT OR IGNORE INTO feedback(id, report_id, subject_kind, subject_key, decision, note, created_at) VALUES(?, ?, ?, ?, ?, ?, ?)`, item.ID, item.ReportID, item.SubjectKind, item.SubjectKey, item.Decision, item.Note, item.CreatedAt); err != nil {
			return err
		}
	}
	return rows.Err()
}

func copyLegacyRules(transaction *sql.Tx, legacy *DB) error {
	rows, err := legacy.Query(`SELECT kind, pattern, target, confidence, note, created_at FROM rules`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item Rule
		var createdAt string
		if err := rows.Scan(&item.Kind, &item.Pattern, &item.Target, &item.Confidence, &item.Note, &createdAt); err != nil {
			return err
		}
		if _, err := transaction.Exec(`INSERT OR IGNORE INTO rules(kind, pattern, target, confidence, note, created_at) VALUES(?, ?, ?, ?, ?, ?)`, item.Kind, item.Pattern, item.Target, item.Confidence, item.Note, createdAt); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (control *Control) ActiveSnapshot() (Snapshot, error) {
	var snapshot Snapshot
	err := control.QueryRow(`SELECT id, path, completed, baseline FROM snapshots WHERE active = 1`).Scan(&snapshot.ID, &snapshot.Path, &snapshot.Completed, &snapshot.Baseline)
	return snapshot, err
}

func (control *Control) Snapshot(id string) (Snapshot, error) {
	var snapshot Snapshot
	err := control.QueryRow(`SELECT id, path, completed, baseline FROM snapshots WHERE id = ?`, id).Scan(&snapshot.ID, &snapshot.Path, &snapshot.Completed, &snapshot.Baseline)
	return snapshot, err
}

func (control *Control) ActivateSnapshot(snapshot Snapshot) error {
	if snapshot.Completed == "" {
		snapshot.Completed = time.Now().UTC().Format(time.RFC3339)
	}
	transaction, err := control.Begin()
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if _, err := transaction.Exec(`UPDATE snapshots SET active = 0 WHERE active = 1`); err != nil {
		return err
	}
	if _, err := transaction.Exec(`INSERT INTO snapshots(id, path, completed, baseline, active) VALUES(?, ?, ?, ?, 1) ON CONFLICT(id) DO UPDATE SET path=excluded.path, completed=excluded.completed, baseline=excluded.baseline, active=1`, snapshot.ID, snapshot.Path, snapshot.Completed, snapshot.Baseline); err != nil {
		return err
	}
	return transaction.Commit()
}

func (control *Control) SaveReport(report Report) error {
	if report.CreatedAt == "" {
		report.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := control.Exec(`INSERT INTO reports(id, created_at, baseline_snapshot, input_digest, report_json) VALUES(?, ?, ?, ?, ?)`, report.ID, report.CreatedAt, report.BaselineSnapshot, report.InputDigest, report.JSON)
	return err
}

func (control *Control) Report(id string) (Report, error) {
	var report Report
	err := control.QueryRow(`SELECT id, created_at, baseline_snapshot, input_digest, report_json FROM reports WHERE id = ?`, id).Scan(&report.ID, &report.CreatedAt, &report.BaselineSnapshot, &report.InputDigest, &report.JSON)
	return report, err
}

func (control *Control) Rules(kind string) ([]Rule, int64, error) {
	rows, err := control.Query(`SELECT kind, pattern, target, confidence, note FROM rules WHERE kind = ? ORDER BY id`, kind)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var rules []Rule
	for rows.Next() {
		var rule Rule
		if err := rows.Scan(&rule.Kind, &rule.Pattern, &rule.Target, &rule.Confidence, &rule.Note); err != nil {
			return nil, 0, err
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	revision, err := control.RuleRevision()
	return rules, revision, err
}

func (control *Control) RuleRevision() (int64, error) {
	var revision int64
	err := control.QueryRow(`SELECT CAST(value AS INTEGER) FROM control_meta WHERE key = 'rule_revision'`).Scan(&revision)
	return revision, err
}

func (control *Control) RecordFeedback(feedback Feedback) (int64, error) {
	if feedback.CreatedAt == "" {
		feedback.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	transaction, err := control.Begin()
	if err != nil {
		return 0, err
	}
	defer transaction.Rollback()
	if _, err := transaction.Exec(`INSERT INTO feedback(id, report_id, subject_kind, subject_key, decision, note, created_at) VALUES(?, ?, ?, ?, ?, ?, ?)`, feedback.ID, feedback.ReportID, feedback.SubjectKind, feedback.SubjectKey, feedback.Decision, feedback.Note, feedback.CreatedAt); err != nil {
		return 0, err
	}
	if _, err := transaction.Exec(`INSERT INTO rules(kind, pattern, target, confidence, note, created_at) VALUES(?, ?, ?, ?, ?, ?) ON CONFLICT(kind, pattern, target) DO UPDATE SET confidence=excluded.confidence, note=excluded.note, created_at=excluded.created_at`, feedback.SubjectKind, feedback.SubjectKey, feedback.Decision, "certain", feedback.Note, feedback.CreatedAt); err != nil {
		return 0, err
	}
	if _, err := transaction.Exec(`INSERT INTO control_meta(key, value) VALUES('rule_revision', '1') ON CONFLICT(key) DO UPDATE SET value = CAST(value AS INTEGER) + 1`); err != nil {
		return 0, err
	}
	var revision int64
	if err := transaction.QueryRow(`SELECT CAST(value AS INTEGER) FROM control_meta WHERE key = 'rule_revision'`).Scan(&revision); err != nil {
		return 0, err
	}
	if err := transaction.Commit(); err != nil {
		return 0, err
	}
	return revision, nil
}
