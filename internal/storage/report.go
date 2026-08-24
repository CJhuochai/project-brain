package storage

import "time"

type Report struct {
	ID               string
	CreatedAt        string
	BaselineSnapshot string
	InputDigest      string
	JSON             string
}

type Feedback struct {
	ID          string
	ReportID    string
	SubjectKind string
	SubjectKey  string
	Decision    string
	Note        string
	CreatedAt   string
}

type Rule struct {
	Kind       string
	Pattern    string
	Target     string
	Confidence string
	Note       string
}

func (db *DB) SaveReport(report Report) error {
	if report.CreatedAt == "" {
		report.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := db.Exec(`INSERT INTO reports(id, created_at, baseline_snapshot, input_digest, report_json) VALUES(?, ?, ?, ?, ?)`, report.ID, report.CreatedAt, report.BaselineSnapshot, report.InputDigest, report.JSON)
	return err
}

func (db *DB) Report(id string) (Report, error) {
	var report Report
	err := db.QueryRow(`SELECT id, created_at, baseline_snapshot, input_digest, report_json FROM reports WHERE id = ?`, id).Scan(&report.ID, &report.CreatedAt, &report.BaselineSnapshot, &report.InputDigest, &report.JSON)
	return report, err
}

func (db *DB) RecordFeedback(feedback Feedback) error {
	if feedback.CreatedAt == "" {
		feedback.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	transaction, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()
	if _, err = transaction.Exec(`INSERT INTO feedback(id, report_id, subject_kind, subject_key, decision, note, created_at) VALUES(?, ?, ?, ?, ?, ?, ?)`, feedback.ID, feedback.ReportID, feedback.SubjectKind, feedback.SubjectKey, feedback.Decision, feedback.Note, feedback.CreatedAt); err != nil {
		return err
	}
	if _, err = transaction.Exec(`INSERT INTO rules(kind, pattern, target, confidence, note, created_at) VALUES(?, ?, ?, ?, ?, ?) ON CONFLICT(kind, pattern, target) DO UPDATE SET confidence=excluded.confidence, note=excluded.note, created_at=excluded.created_at`, feedback.SubjectKind, feedback.SubjectKey, feedback.Decision, "certain", feedback.Note, feedback.CreatedAt); err != nil {
		return err
	}
	return transaction.Commit()
}

func (db *DB) Rules(kind string) ([]Rule, error) {
	rows, err := db.Query(`SELECT kind, pattern, target, confidence, note FROM rules WHERE kind = ? ORDER BY id`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []Rule
	for rows.Next() {
		var rule Rule
		if err := rows.Scan(&rule.Kind, &rule.Pattern, &rule.Target, &rule.Confidence, &rule.Note); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}
