package extract

import (
	"path/filepath"
	"regexp"
	"strings"
)

var (
	packagePattern   = regexp.MustCompile(`^\s*package\s+([\w.]+)\s*;`)
	typePattern      = regexp.MustCompile(`\b(class|interface|enum)\s+(\w+)`)
	fieldPattern     = regexp.MustCompile(`\b(?:private|protected)\s+(?:final\s+)?([A-Z]\w*)\s+\w+`)
	routePattern     = regexp.MustCompile(`@(Get|Post|Put|Delete|Patch|Request)Mapping\s*\(\s*"([^"]+)"`)
	namespacePattern = regexp.MustCompile(`(?i)<mapper\s+[^>]*namespace\s*=\s*"([^"]+)"`)
	statementPattern = regexp.MustCompile(`(?i)<(select|insert|update|delete)\s+[^>]*id\s*=\s*"([^"]+)"`)
	tablePattern     = regexp.MustCompile(`(?i)\b(from|join|update|into)\s+[` + "`" + `"]?([a-zA-Z0-9_]+)`)
	artifactPattern  = regexp.MustCompile(`(?is)<artifactId>\s*([^<\s]+)\s*</artifactId>`)
)

func File(path string, content []byte) Result {
	if strings.EqualFold(filepath.Base(path), "pom.xml") {
		return extractMaven(content)
	}
	if strings.EqualFold(filepath.Ext(path), ".xml") {
		return extractMapper(content)
	}
	if strings.EqualFold(filepath.Ext(path), ".java") {
		return extractJava(content)
	}
	return Result{}
}

func extractMaven(content []byte) Result {
	text := string(content)
	artifacts := artifactPattern.FindAllStringSubmatchIndex(text, -1)
	if len(artifacts) == 0 {
		return Result{}
	}
	module := "maven:" + text[artifacts[0][2]:artifacts[0][3]]
	result := Result{Symbols: []Symbol{{Name: module, Kind: "maven_module", Line: strings.Count(text[:artifacts[0][0]], "\n") + 1}}}
	for _, artifact := range artifacts[1:] {
		line := strings.Count(text[:artifact[0]], "\n") + 1
		result.Edges = append(result.Edges, Edge{Source: module, Target: "maven:" + text[artifact[2]:artifact[3]], Kind: "depends_on_module", Line: line, Confidence: Certain})
	}
	return result
}

func extractJava(content []byte) Result {
	lines := strings.Split(string(content), "\n")
	packageName, currentType, currentKind := "", "", "class"
	componentKind := ""
	for index, line := range lines {
		if match := packagePattern.FindStringSubmatch(line); match != nil {
			packageName = match[1]
		}
		if strings.Contains(line, "@RestController") || strings.Contains(line, "@Controller") {
			componentKind = "controller"
		}
		if strings.Contains(line, "@Service") {
			componentKind = "service"
		}
		if strings.Contains(line, "@Repository") {
			componentKind = "repository"
		}
		if match := typePattern.FindStringSubmatch(line); match != nil {
			currentType, currentKind = match[2], match[1]
			name := currentType
			if packageName != "" {
				name = packageName + "." + currentType
			}
			if componentKind != "" {
				currentKind = componentKind
			}
			result := Result{Symbols: []Symbol{{Name: name, Kind: currentKind, Line: index + 1}}}
			for routeIndex, routeLine := range lines {
				if route := routePattern.FindStringSubmatch(routeLine); route != nil && componentKind == "controller" {
					result.Edges = append(result.Edges, Edge{Source: name, Target: route[2], Kind: "route", Line: routeIndex + 1, Confidence: Certain})
				}
			}
			for fieldIndex, fieldLine := range lines {
				if field := fieldPattern.FindStringSubmatch(fieldLine); field != nil {
					result.Edges = append(result.Edges, Edge{Source: name, Target: field[1], Kind: "uses", Line: fieldIndex + 1, Confidence: Probable})
				}
			}
			return result
		}
	}
	return Result{}
}

func extractMapper(content []byte) Result {
	text := string(content)
	namespace := ""
	if match := namespacePattern.FindStringSubmatch(text); match != nil {
		namespace = match[1]
	}
	if namespace == "" {
		return Result{}
	}
	result := Result{Symbols: []Symbol{{Name: namespace, Kind: "mapper", Line: 1}}}
	for _, statement := range statementPattern.FindAllStringSubmatchIndex(text, -1) {
		id := text[statement[4]:statement[5]]
		line := strings.Count(text[:statement[0]], "\n") + 1
		name := namespace + "." + id
		result.Symbols = append(result.Symbols, Symbol{Name: name, Kind: "mapper_statement", Line: line})
		result.Edges = append(result.Edges, Edge{Source: namespace, Target: name, Kind: "maps_statement", Line: line, Confidence: Certain})
	}
	for _, table := range tablePattern.FindAllStringSubmatchIndex(text, -1) {
		line := strings.Count(text[:table[0]], "\n") + 1
		result.Edges = append(result.Edges, Edge{Source: namespace, Target: text[table[4]:table[5]], Kind: "queries_table", Line: line, Confidence: Certain})
	}
	return result
}
