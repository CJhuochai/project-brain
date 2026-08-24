package extract

import (
	"path/filepath"
	"regexp"
	"strings"
)

var (
	configLinePattern = regexp.MustCompile(`(?m)^\s*([A-Za-z][\w.-]+)\s*(?:=|:)`)
	sqlTablePattern   = regexp.MustCompile(`(?i)\b(?:create|alter|drop|insert\s+into|update)\s+(?:table\s+)?[` + "`" + `"]?([a-zA-Z0-9_]+)`)
	apiPathPattern    = regexp.MustCompile(`(?m)^\s*['"]?(/[^'"\s:]+)['"]?\s*:`)
	httpCallPattern   = regexp.MustCompile(`(?i)\b(?:axios\s*\.\s*)?(get|post|put|delete|patch)\s*\(\s*['"](/[^'"]+)`)
)

func extractExtended(path string, content []byte) Result {
	text, extension := string(content), strings.ToLower(filepath.Ext(path))
	source := filepath.Base(path)
	result := Result{}
	if extension == ".yml" || extension == ".yaml" || extension == ".properties" {
		for _, match := range configLinePattern.FindAllStringSubmatchIndex(text, -1) {
			result.Edges = append(result.Edges, Edge{Source: source, Target: "config:" + text[match[2]:match[3]], Kind: "defines_config", Line: lineAt(text, match[0]), Confidence: Certain})
		}
	}
	switch extension {
	case ".sql":
		for _, match := range sqlTablePattern.FindAllStringSubmatchIndex(text, -1) {
			result.Edges = append(result.Edges, Edge{Source: source, Target: "table:" + text[match[2]:match[3]], Kind: "migration_table", Line: lineAt(text, match[0]), Confidence: Certain})
		}
	case ".ts", ".tsx", ".js", ".vue", ".html":
		for _, match := range httpCallPattern.FindAllStringSubmatchIndex(text, -1) {
			result.Edges = append(result.Edges, Edge{Source: source, Target: "http:" + strings.ToUpper(text[match[2]:match[3]]) + ":" + text[match[4]:match[5]], Kind: "calls_http", Line: lineAt(text, match[0]), Confidence: Certain})
		}
	}
	if extension == ".json" || extension == ".yaml" || extension == ".yml" {
		for _, match := range apiPathPattern.FindAllStringSubmatchIndex(text, -1) {
			result.Edges = append(result.Edges, Edge{Source: source, Target: "http:ANY:" + text[match[2]:match[3]], Kind: "api_contract", Line: lineAt(text, match[0]), Confidence: Certain})
		}
	}
	if strings.Contains(text, "${") {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "动态配置或运行时路由未能静态确定", Line: 1, Confidence: Unresolved})
	}
	return result
}
