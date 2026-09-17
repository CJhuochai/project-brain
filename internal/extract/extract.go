package extract

import (
	"path/filepath"
	"regexp"
	"strings"
)

var (
	packagePattern     = regexp.MustCompile(`^\s*package\s+([\w.]+)\s*;`)
	typePattern        = regexp.MustCompile(`\b(class|interface|enum)\s+(\w+)`)
	fieldPattern       = regexp.MustCompile(`\b(?:private|protected)\s+(?:final\s+)?([A-Z]\w*)\s+\w+`)
	importPattern      = regexp.MustCompile(`^\s*import\s+([\w.]+)\s*;`)
	implementsPattern  = regexp.MustCompile(`\b(?:extends|implements)\s+([\w.,\s]+)`)
	feignPattern       = regexp.MustCompile(`@FeignClient\s*\(\s*(?:name|value)\s*=\s*"([^"]+)"`)
	routePattern       = regexp.MustCompile(`@(Get|Post|Put|Delete|Patch|Request)Mapping\s*\(\s*"([^"]+)"`)
	javaFieldPattern   = regexp.MustCompile(`(?m)^\s*(?:private|protected|public)?\s*(?:static\s+)?(?:final\s+)?([A-Z]\w*(?:\s*<[^;=(){}]+>)?)\s+(\w+)\s*(?:=[^;]*)?;`)
	javaMethodPattern  = regexp.MustCompile(`(?m)(?:^\s*@[\w.]+(?:\s*\([^)]*\))?\s*)*(?:^\s*(?:public|protected|private)\s+)?(?:static\s+)?[\w.<>\[\], ?]+\s+(\w+)\s*\([^;{}]*\)\s*(?:throws\s+[\w., ]+)?\{`)
	javaCallPattern    = regexp.MustCompile(`\b(\w+)\s*\.\s*(\w+)\s*\(`)
	namespacePattern   = regexp.MustCompile(`(?i)<mapper\s+[^>]*namespace\s*=\s*"([^"]+)"`)
	statementPattern   = regexp.MustCompile(`(?i)<(select|insert|update|delete)\s+[^>]*id\s*=\s*"([^"]+)"`)
	tablePattern       = regexp.MustCompile(`(?i)\b(from|join|update|into)\s+[` + "`" + `"]?([a-zA-Z0-9_]+)`)
	artifactPattern    = regexp.MustCompile(`(?is)<artifactId>\s*([^<\s]+)\s*</artifactId>`)
	dubboPattern       = regexp.MustCompile(`@DubboReference(?:\s*\([^)]*\))?\s+(?:private|protected|public)?\s*(?:final\s+)?([A-Z]\w*)\s+\w+`)
	kafkaListenPattern = regexp.MustCompile(`@(?:Kafka|RocketMQ|Rabbit)Listener\s*\([^)]*(?:topics|topic|value)\s*=\s*"([^"]+)"`)
	publishPattern     = regexp.MustCompile(`\b(?:send|convertAndSend|syncSend)\s*\(\s*"([^"]+)"`)
	scheduledPattern   = regexp.MustCompile(`@Scheduled\s*\([^)]*\)\s*(?:public|protected|private)?\s*(?:static\s+)?[\w.<>\[\], ?]+\s+(\w+)\s*\(`)
	valuePattern       = regexp.MustCompile(`@Value\s*\(\s*"\$\{([^}:]+)`)
	configPropsPattern = regexp.MustCompile(`@ConfigurationProperties\s*\(\s*(?:prefix\s*=\s*)?"([^"]+)"`)
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
	return extractExtended(path, content)
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

func extractJavaLegacy(content []byte) Result {
	text, masked := string(content), maskJava(string(content))
	typeMatch := typePattern.FindStringSubmatchIndex(masked)
	if typeMatch == nil {
		return Result{}
	}
	typeName := masked[typeMatch[4]:typeMatch[5]]
	lines := strings.Split(text, "\n")
	imports, packageName := javaImportsAndPackage(lines)
	qualifiedType := qualify(packageName, typeName)
	componentKind := javaComponentKind(masked[:typeMatch[0]], typeName)
	result := Result{Symbols: []Symbol{{Name: qualifiedType, Kind: componentKind, Line: lineAt(text, typeMatch[0])}}}
	for index, line := range lines {
		if imported := importPattern.FindStringSubmatch(line); imported != nil {
			result.Edges = append(result.Edges, Edge{Source: qualifiedType, Target: imported[1], Kind: "imports", Line: index + 1, Confidence: Certain})
		}
		if feign := feignPattern.FindStringSubmatch(line); feign != nil {
			result.Edges = append(result.Edges, Edge{Source: qualifiedType, Target: "feign:" + feign[1], Kind: "feign_client", Line: index + 1, Confidence: Certain})
		}
	}
	for _, match := range dubboPattern.FindAllStringSubmatchIndex(text, -1) {
		result.Edges = append(result.Edges, Edge{Source: qualifiedType, Target: "dubbo:" + text[match[2]:match[3]], Kind: "dubbo_reference", Line: lineAt(text, match[0]), Confidence: Certain})
	}
	for _, match := range kafkaListenPattern.FindAllStringSubmatchIndex(text, -1) {
		result.Edges = append(result.Edges, Edge{Source: qualifiedType, Target: "mq:" + text[match[2]:match[3]], Kind: "consumes_topic", Line: lineAt(text, match[0]), Confidence: Certain})
	}
	for _, match := range publishPattern.FindAllStringSubmatchIndex(text, -1) {
		result.Edges = append(result.Edges, Edge{Source: qualifiedType, Target: "mq:" + text[match[2]:match[3]], Kind: "publishes_topic", Line: lineAt(text, match[0]), Confidence: Certain})
	}
	for _, match := range scheduledPattern.FindAllStringSubmatchIndex(text, -1) {
		result.Edges = append(result.Edges, Edge{Source: qualifiedType, Target: "schedule:" + typeName + "#" + text[match[2]:match[3]], Kind: "scheduled_job", Line: lineAt(text, match[0]), Confidence: Certain})
	}
	if strings.Contains(masked[:typeMatch[0]], "@DubboService") {
		result.Edges = append(result.Edges, Edge{Source: qualifiedType, Target: "dubbo:" + typeName, Kind: "dubbo_service", Line: lineAt(text, typeMatch[0]), Confidence: Certain})
	}
	for _, match := range valuePattern.FindAllStringSubmatchIndex(text, -1) {
		result.Edges = append(result.Edges, Edge{Source: qualifiedType, Target: "config:" + text[match[2]:match[3]], Kind: "uses_config", Line: lineAt(text, match[0]), Confidence: Certain})
	}
	for _, match := range configPropsPattern.FindAllStringSubmatchIndex(text, -1) {
		result.Edges = append(result.Edges, Edge{Source: qualifiedType, Target: "config:" + text[match[2]:match[3]], Kind: "uses_config", Line: lineAt(text, match[0]), Confidence: Certain})
	}
	if strings.Contains(text, "Class.forName(") || strings.Contains(text, "Proxy.") || strings.Contains(text, "${") {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "反射、代理或动态路由未能静态确定", Line: 1, Confidence: Unresolved})
	}
	if relation := implementsPattern.FindStringSubmatch(masked[typeMatch[0]:]); relation != nil {
		for _, target := range strings.Split(relation[1], ",") {
			if target = strings.TrimSpace(target); target != "" {
				result.Edges = append(result.Edges, Edge{Source: qualifiedType, Target: target, Kind: "implements", Line: lineAt(text, typeMatch[0]), Confidence: Probable})
			}
		}
	}
	bodyStart := strings.Index(masked[typeMatch[1]:], "{") + typeMatch[1]
	if bodyStart < typeMatch[1] {
		return result
	}
	fields := javaFields(masked[bodyStart+1:], imports, packageName, lineAt(text, bodyStart+1))
	for _, dependency := range fields {
		result.Edges = append(result.Edges, Edge{Source: qualifiedType, Target: shortType(dependency.typeName), Kind: "uses", Line: dependency.line, Confidence: Probable})
	}
	for _, method := range javaMethodPattern.FindAllStringSubmatchIndex(masked[bodyStart+1:], -1) {
		start := bodyStart + 1 + method[0]
		if braceDepth(masked, bodyStart, start) != 1 {
			continue
		}
		name := masked[bodyStart+1+method[2] : bodyStart+1+method[3]]
		qualifiedMethod := qualifiedType + "." + name
		line := lineAt(text, start)
		result.Symbols = append(result.Symbols, Symbol{Name: qualifiedMethod, Kind: componentKind + "_method", Line: line})
		header := text[start : bodyStart+1+method[1]]
		if componentKind == "controller" {
			for _, route := range routePattern.FindAllStringSubmatch(header, -1) {
				result.Edges = append(result.Edges, Edge{Source: qualifiedMethod, Target: route[2], Kind: "route", Line: line, Confidence: Certain})
			}
		}
		end := matchingBrace(masked, bodyStart+1+method[1]-1)
		if end < 0 {
			continue
		}
		for _, call := range javaCallPattern.FindAllStringSubmatchIndex(masked[bodyStart+1+method[1]:end], -1) {
			fieldName := masked[bodyStart+1+method[1]+call[2] : bodyStart+1+method[1]+call[3]]
			if dependency, found := fields[fieldName]; found {
				callStart := bodyStart + 1 + method[1] + call[0]
				calledMethod := masked[bodyStart+1+method[1]+call[4] : bodyStart+1+method[1]+call[5]]
				result.Edges = append(result.Edges, Edge{Source: qualifiedMethod, Target: dependency.typeName + "." + calledMethod, Kind: "calls", Line: lineAt(text, callStart), Confidence: Certain})
			}
		}
	}
	return result
}

type javaField struct {
	typeName string
	line     int
}

func javaImportsAndPackage(lines []string) (map[string]string, string) {
	imports, packageName := map[string]string{}, ""
	for _, line := range lines {
		if match := packagePattern.FindStringSubmatch(line); match != nil {
			packageName = match[1]
		}
		if match := importPattern.FindStringSubmatch(line); match != nil {
			imports[shortType(match[1])] = match[1]
		}
	}
	return imports, packageName
}

func javaComponentKind(text, typeName string) string {
	if strings.Contains(text, "@RestController") || strings.Contains(text, "@Controller") {
		return "controller"
	}
	if strings.Contains(text, "@Mapper") {
		return "mapper"
	}
	if strings.Contains(text, "@Repository") {
		return "repository"
	}
	if strings.Contains(text, "@Service") && strings.HasSuffix(typeName, "Application") {
		return "application"
	}
	if strings.Contains(text, "@Service") {
		return "service"
	}
	return "class"
}

func javaFields(text string, imports map[string]string, packageName string, baseLine int) map[string]javaField {
	fields := map[string]javaField{}
	for _, field := range javaFieldPattern.FindAllStringSubmatchIndex(text, -1) {
		if braceDepth(text, 0, field[0]) != 0 {
			continue
		}
		name := text[field[4]:field[5]]
		fields[name] = javaField{typeName: qualifyJavaType(text[field[2]:field[3]], imports, packageName), line: baseLine + strings.Count(text[:field[0]], "\n")}
	}
	return fields
}

func qualifyJavaType(name string, imports map[string]string, packageName string) string {
	name = strings.TrimSpace(name)
	base, suffix := name, ""
	if index := strings.Index(base, "<"); index >= 0 {
		base = base[:index]
	}
	if strings.HasSuffix(base, "[]") {
		base, suffix = strings.TrimSuffix(base, "[]"), "[]"
	}
	if qualified, found := imports[base]; found {
		return qualified + suffix
	}
	switch base {
	case "byte", "short", "int", "long", "float", "double", "boolean", "char", "void":
		return base + suffix
	case "String", "Object", "Integer", "Long", "Boolean", "Double", "Float", "Short", "Byte", "Character", "Void":
		return "java.lang." + base + suffix
	}
	return qualify(packageName, base) + suffix
}

func qualify(packageName, name string) string {
	if packageName == "" || strings.Contains(name, ".") {
		return name
	}
	return packageName + "." + name
}

func shortType(name string) string {
	if index := strings.LastIndex(name, "."); index >= 0 {
		return name[index+1:]
	}
	return name
}

func maskJava(text string) string {
	masked := []byte(text)
	for index := 0; index < len(masked); index++ {
		if masked[index] == '/' && index+1 < len(masked) && masked[index+1] == '/' {
			for index < len(masked) && masked[index] != '\n' {
				masked[index], index = ' ', index+1
			}
			index--
		} else if masked[index] == '/' && index+1 < len(masked) && masked[index+1] == '*' {
			for index < len(masked)-1 && !(masked[index] == '*' && masked[index+1] == '/') {
				if masked[index] != '\n' {
					masked[index] = ' '
				}
				index++
			}
			if index+1 < len(masked) {
				masked[index], masked[index+1] = ' ', ' '
				index++
			}
		} else if masked[index] == '"' || masked[index] == '\'' {
			quote := masked[index]
			masked[index] = ' '
			index++
			for index < len(masked) && masked[index] != quote {
				if masked[index] == '\\' && index+1 < len(masked) {
					masked[index], masked[index+1], index = ' ', ' ', index+2
					continue
				}
				if masked[index] != '\n' {
					masked[index] = ' '
				}
				index++
			}
			if index < len(masked) {
				masked[index] = ' '
			}
		}
	}
	return string(masked)
}

func braceDepth(text string, start, end int) int {
	return strings.Count(text[start:end], "{") - strings.Count(text[start:end], "}")
}

func matchingBrace(text string, start int) int {
	depth := 0
	for index := start; index < len(text); index++ {
		if text[index] == '{' {
			depth++
		}
		if text[index] == '}' {
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func lineAt(text string, index int) int { return strings.Count(text[:index], "\n") + 1 }

func extractMapper(content []byte) Result {
	text := string(content)
	namespace := ""
	if match := namespacePattern.FindStringSubmatch(text); match != nil {
		namespace = match[1]
	}
	if namespace == "" {
		return Result{Diagnostics: []Diagnostic{{Message: "MyBatis XML 缺少 mapper namespace，未生成关系", Line: 1, Confidence: Unresolved}}}
	}
	result := Result{Symbols: []Symbol{{Name: namespace, Kind: "mapper", Line: 1}}}
	if strings.Contains(text, "${") || strings.Contains(strings.ToLower(text), "<if") {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "动态 SQL 可能导致表和条件关系不完整", Line: 1, Confidence: Unresolved})
	}
	for _, statement := range statementPattern.FindAllStringSubmatchIndex(text, -1) {
		statementType := text[statement[2]:statement[3]]
		id := text[statement[4]:statement[5]]
		line := strings.Count(text[:statement[0]], "\n") + 1
		name := namespace + "." + id
		result.Symbols = append(result.Symbols, Symbol{Name: name, Kind: "mapper_statement", Line: line})
		result.Edges = append(result.Edges, Edge{Source: namespace, Target: name, Kind: "maps_statement", Line: line, Confidence: Certain})
		close := regexp.MustCompile(`(?is)</` + statementType + `\s*>`).FindStringIndex(text[statement[1]:])
		if close == nil {
			continue
		}
		bodyStart, bodyEnd := statement[1], statement[1]+close[0]
		for _, table := range tablePattern.FindAllStringSubmatchIndex(text[bodyStart:bodyEnd], -1) {
			tableStart := bodyStart + table[0]
			result.Edges = append(result.Edges, Edge{Source: name, Target: text[bodyStart+table[4] : bodyStart+table[5]], Kind: "queries_table", Line: strings.Count(text[:tableStart], "\n") + 1, Confidence: Certain})
		}
	}
	return result
}
