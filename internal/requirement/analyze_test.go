package requirement

import (
	"testing"

	"github.com/CJhuochai/project-brain/internal/extract"
	"github.com/CJhuochai/project-brain/internal/storage"
)

func TestAnalyzeReturnsRepositoryCandidatesFromRequirementWords(t *testing.T) {
	db, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.UpsertFile("entry-service", "EntryController.java", []byte("class EntryController { void groupSubmit() {} }")); err != nil {
		t.Fatal(err)
	}
	report, err := Analyze(db, "调整 groupSubmit 的报名提交")
	if err != nil || len(report.Repositories) != 1 || report.Repositories[0].Repository != "entry-service" {
		t.Fatalf("report=%#v err=%v", report, err)
	}
}

func TestAnalyzeReturnsEntryPointsAndChangePoints(t *testing.T) {
	db, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	content := []byte(`package com.example;
import org.springframework.web.bind.annotation.RestController;
import org.springframework.web.bind.annotation.PostMapping;
@RestController
class EntryController {
  @PostMapping("/entry/submit")
  public void groupSubmit() {}
}`)
	if err := db.ReplaceEvidenceTx(tx, "entry-service", "EntryController.java", extract.File("EntryController.java", content)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	report, err := Analyze(db, "修改 groupSubmit")
	if err != nil || len(report.EntryPoints) == 0 || len(report.ChangePoints) == 0 {
		t.Fatalf("report=%#v err=%v", report, err)
	}
}

func TestAnalyzeMatchesChineseBusinessTerms(t *testing.T) {
	db, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.UpsertFile("entry-service", "EntryController.java", []byte("// 报名提交入口")); err != nil {
		t.Fatal(err)
	}
	report, err := Analyze(db, "调整报名提交的校验逻辑")
	if err != nil || len(report.Repositories) != 1 || report.Repositories[0].Repository != "entry-service" {
		t.Fatalf("report=%#v err=%v", report, err)
	}
}
