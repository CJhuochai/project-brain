package requirement

import (
	"fmt"
	"slices"
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

func TestAnalyzeLimitsImpactTraces(t *testing.T) {
	db, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	application := []byte(`package com.example;
class EntryApplication {
  public void submit0() {}
  public void submit1() {}
  public void submit2() {}
  public void submit3() {}
  public void submit4() {}
  public void submit5() {}
  public void submit6() {}
  public void submit7() {}
  public void submit8() {}
}`)
	if _, err := db.UpsertFileTx(tx, "entry-service", "EntryApplication.java", application); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceEvidenceTx(tx, "entry-service", "EntryApplication.java", extract.File("EntryApplication.java", application)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 9; i++ {
		path := fmt.Sprintf("Entry%dController.java", i)
		content := []byte(fmt.Sprintf(`package com.example;
import org.springframework.web.bind.annotation.RestController;
@RestController
class Entry%dController {
	  private final EntryApplication application;
  // 报名提交入口
  public void submit%d() { application.submit%d(); }
}`, i, i, i))
		if _, err := db.UpsertFileTx(tx, "entry-service", path, content); err != nil {
			t.Fatal(err)
		}
		if err := db.ReplaceEvidenceTx(tx, "entry-service", path, extract.File(path, content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	report, err := Analyze(db, "报名提交")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Impacts) > 8 {
		t.Fatalf("impact traces=%d, want at most 8", len(report.Impacts))
	}
}

func TestKeywordsAvoidsTwoCharacterFragmentsInsideLongChineseText(t *testing.T) {
	got := keywords("招生访校活动报名截止规则")
	if slices.Contains(got, "招生") || slices.Contains(got, "报名") {
		t.Fatalf("keywords should not contain broad two-character fragments: %#v", got)
	}
	if !slices.Contains(got, "报名截") {
		t.Fatalf("keywords should retain specific Chinese context: %#v", got)
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
import com.example.application.EntryApplication;
@RestController
class EntryController {
  private final EntryApplication application;
  @PostMapping("/entry/submit")
  public void groupSubmit() { application.groupSubmit(); }
}`)
	if err := db.ReplaceEvidenceTx(tx, "entry-service", "EntryController.java", extract.File("EntryController.java", content)); err != nil {
		t.Fatal(err)
	}
	application := []byte(`package com.example.application;
class EntryApplication { public void groupSubmit() {} }`)
	if err := db.ReplaceEvidenceTx(tx, "entry-service", "EntryApplication.java", extract.File("EntryApplication.java", application)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	report, err := Analyze(db, "修改 groupSubmit")
	if err != nil || len(report.EntryPoints) == 0 || len(report.ChangePoints) == 0 || len(report.Impacts) != 1 {
		t.Fatalf("report=%#v err=%v", report, err)
	}
}

func TestAnalyzeMatchesChineseBusinessTerms(t *testing.T) {
	db, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	content := []byte(`import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RestController;
import com.example.application.EntryApplication;
@RestController
class EntryController {
	  private final EntryApplication application;
  // 报名提交入口
  @PostMapping("/entry/submit")
  public void groupSubmit() { application.groupSubmit(); }
}`)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertFileTx(tx, "entry-service", "EntryController.java", content); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceEvidenceTx(tx, "entry-service", "EntryController.java", extract.File("EntryController.java", content)); err != nil {
		t.Fatal(err)
	}
	application := []byte(`package com.example.application;
class EntryApplication { public void groupSubmit() {} }`)
	if err := db.ReplaceEvidenceTx(tx, "entry-service", "EntryApplication.java", extract.File("EntryApplication.java", application)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	report, err := Analyze(db, "调整报名提交的校验逻辑")
	if err != nil || len(report.Repositories) != 1 || report.Repositories[0].Repository != "entry-service" || len(report.EntryPoints) == 0 || len(report.Impacts) != 1 {
		t.Fatalf("report=%#v err=%v", report, err)
	}
	for _, point := range report.ChangePoints {
		if point.Kind == "imports" {
			t.Fatalf("imports must not be reported as a change point: %#v", report.ChangePoints)
		}
	}
}

func TestAnalyzeLimitsEvidencePayload(t *testing.T) {
	db, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for index := 0; index < maxEvidence+1; index++ {
		if _, err := db.UpsertFile("entry-service", fmt.Sprintf("Entry%d.java", index), []byte("class Entry {}")); err != nil {
			t.Fatal(err)
		}
	}
	report, err := Analyze(db, "Entry")
	if err != nil || len(report.Evidence) != maxEvidence {
		t.Fatalf("evidence=%d err=%v", len(report.Evidence), err)
	}
}
