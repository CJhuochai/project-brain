package input

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseExtractsFactsFromHTMLAndJSON(t *testing.T) {
	directory := t.TempDir()
	html := filepath.Join(directory, "prototype.html")
	jsonFile := filepath.Join(directory, "figma.json")
	if err := os.WriteFile(html, []byte(`<h1>报名</h1><input name="studentId"><a href="/api/entry">提交</a>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jsonFile, []byte(`{"name":"Entry","route":"/api/entry","studentId":""}`), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Parse("调整报名", []string{html, jsonFile}, "figma://entry")
	if err != nil || !hasFact(result.Facts, "api_path", "/api/entry") || !hasFact(result.Facts, "field", "studentId") {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestParseImageWithoutOCRReturnsDiagnostic(t *testing.T) {
	image := filepath.Join(t.TempDir(), "prototype.png")
	if err := os.WriteFile(image, []byte("image"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "")
	result, err := Parse("", []string{image}, "")
	if err != nil || !hasDiagnostic(result.Diagnostics, "OCR") {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func hasFact(facts []Fact, kind, value string) bool {
	for _, fact := range facts {
		if fact.Kind == kind && fact.Value == value {
			return true
		}
	}
	return false
}

func hasDiagnostic(diagnostics []Diagnostic, text string) bool {
	for _, diagnostic := range diagnostics {
		if contains(diagnostic.Message, text) {
			return true
		}
	}
	return false
}
