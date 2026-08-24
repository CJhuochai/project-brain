package input

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const maxFileSize = 20 << 20

type Fact struct {
	Kind       string `json:"kind"`
	Value      string `json:"value"`
	SourcePath string `json:"source_path,omitempty"`
	Line       int    `json:"line,omitempty"`
	Confidence string `json:"confidence"`
}

type Diagnostic struct {
	Message    string `json:"message"`
	SourcePath string `json:"source_path,omitempty"`
	Line       int    `json:"line,omitempty"`
	Confidence string `json:"confidence"`
}

type Result struct {
	Facts       []Fact       `json:"facts"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

var (
	pathPattern  = regexp.MustCompile(`/(?:[A-Za-z0-9._-]+/?)+`)
	fieldPattern = regexp.MustCompile(`(?i)(?:name|id)\s*=\s*["']([A-Za-z_][A-Za-z0-9_]*)["']`)
	tagPattern   = regexp.MustCompile(`<[^>]+>`)
)

func Parse(text string, paths []string, sourceRef string) (Result, error) {
	result := Result{}
	addTextFacts(&result, text, sourceRef)
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return result, err
		}
		if !info.Mode().IsRegular() || info.Size() > maxFileSize {
			return result, fmt.Errorf("input file must be a regular file no larger than 20 MiB: %s", path)
		}
		extension := strings.ToLower(filepath.Ext(path))
		if isImage(extension) {
			parseImage(&result, path)
			continue
		}
		content, err := readContent(path, extension)
		if err != nil {
			return result, err
		}
		if extension == ".json" {
			addJSONFacts(&result, content, path)
		} else {
			addTextFacts(&result, tagPattern.ReplaceAllString(content, " "), path)
			for _, match := range fieldPattern.FindAllStringSubmatch(content, -1) {
				addFact(&result, Fact{Kind: "field", Value: match[1], SourcePath: path, Confidence: "certain"})
			}
		}
	}
	return result, nil
}

func readContent(path, extension string) (string, error) {
	if extension != ".docx" {
		bytes, err := os.ReadFile(path)
		return string(bytes), err
	}
	reader, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.Name != "word/document.xml" {
			continue
		}
		r, err := file.Open()
		if err != nil {
			return "", err
		}
		bytes, readErr := io.ReadAll(r)
		_ = r.Close()
		if readErr == nil {
			return string(bytes), nil
		}
		return "", readErr
	}
	return "", fmt.Errorf("docx has no word/document.xml: %s", path)
}

func addTextFacts(result *Result, text, source string) {
	for _, match := range pathPattern.FindAllString(text, -1) {
		addFact(result, Fact{Kind: "api_path", Value: match, SourcePath: source, Confidence: "certain"})
	}
	for line, value := range strings.Split(text, "\n") {
		value = strings.TrimSpace(value)
		if value != "" && !pathPattern.MatchString(value) {
			addFact(result, Fact{Kind: "business_term", Value: value, SourcePath: source, Line: line + 1, Confidence: "probable"})
		}
	}
}

func addJSONFacts(result *Result, content, path string) {
	var value any
	if json.Unmarshal([]byte(content), &value) != nil {
		addTextFacts(result, content, path)
		return
	}
	visitJSON(result, value, path)
}

func visitJSON(result *Result, value any, path string) {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if key != "route" && key != "path" && key != "url" {
				addFact(result, Fact{Kind: "field", Value: key, SourcePath: path, Confidence: "certain"})
			}
			visitJSON(result, child, path)
		}
	case []any:
		for _, child := range current {
			visitJSON(result, child, path)
		}
	case string:
		addTextFacts(result, current, path)
	}
}

func parseImage(result *Result, path string) {
	if _, err := exec.LookPath("tesseract"); err != nil {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "OCR unavailable for image input", SourcePath: path, Confidence: "unresolved"})
		return
	}
	output, err := exec.Command("tesseract", path, "stdout").Output()
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "OCR could not read image input", SourcePath: path, Confidence: "unresolved"})
		return
	}
	addTextFacts(result, string(output), path)
}

func isImage(extension string) bool {
	return extension == ".png" || extension == ".jpg" || extension == ".jpeg" || extension == ".webp"
}

func addFact(result *Result, fact Fact) {
	for _, existing := range result.Facts {
		if existing.Kind == fact.Kind && existing.Value == fact.Value && existing.SourcePath == fact.SourcePath {
			return
		}
	}
	result.Facts = append(result.Facts, fact)
}

func contains(value, text string) bool { return strings.Contains(value, text) }
