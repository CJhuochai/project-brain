package extract

import (
	"regexp"
	"strings"
)

var javaDeclarationPattern = regexp.MustCompile(`(?ms)(?:^[ \t]*@[\w.]+(?:\s*\([^\n]*\))?\s*)*^[ \t]*(?:(?:public|protected|private|default|static|abstract|final|synchronized|native)\s+)*(?:<[^>]+>\s+)?[\w.$<>?, \[\]]+?\s+(\w+)\s*\(([^)]*)\)\s*(?:throws\s+[\w., ]+)?([;{])`)
var javaReceiverCallPattern = regexp.MustCompile(`\b(\w+)\s*\.\s*(\w+)\s*\(`)
var javaLocalPattern = regexp.MustCompile(`\b(?:final\s+)?[A-Z]\w*(?:\s*<[^;=(){}]+>)?(?:\[\])?\s+(\w+)\s*(?:=|;)`)
var javaStringPattern = regexp.MustCompile(`"((?:\\.|[^"\\])*)"`)

// extractJava retains v1 evidence, then adds signatures, conservative calls and
// service contracts. The old extractor intentionally remains the source of
// legacy edge names while this pass supplies fields needed by v2 storage.
func extractJava(content []byte) Result {
	result := extractJavaLegacy(content)
	enrichJavaV2(&result, string(content))
	return result
}

type javaMethod struct {
	symbol, signature                string
	start, headerEnd, bodyStart, end int
	params                           map[string]bool
	unsupportedParams                bool
}

func enrichJavaV2(result *Result, text string) {
	masked := maskJava(text)
	typeMatch := typePattern.FindStringSubmatchIndex(masked)
	if typeMatch == nil {
		return
	}
	typeName := masked[typeMatch[4]:typeMatch[5]]
	imports, packageName := javaImportsAndPackage(strings.Split(text, "\n"))
	owner := qualify(packageName, typeName)
	kind := javaComponentKind(masked[:typeMatch[0]], typeName)
	bodyStart := strings.Index(masked[typeMatch[1]:], "{")
	if bodyStart < 0 {
		return
	}
	bodyStart += typeMatch[1]
	methods := findJavaMethods(text, maskAnnotationArguments(masked), bodyStart, owner, imports, packageName)
	// Legacy extraction cannot distinguish overloads on the same line. Rebuild
	// method symbols from the v2 declaration pass to avoid ghost identities.
	keptSymbols := result.Symbols[:0]
	for _, symbol := range result.Symbols {
		if !strings.HasSuffix(symbol.Kind, "_method") {
			keptSymbols = append(keptSymbols, symbol)
		}
	}
	result.Symbols = keptSymbols
	for _, method := range methods {
		result.Symbols = append(result.Symbols, Symbol{Name: method.symbol, Kind: kind + "_method", Line: lineAt(text, method.start), Signature: method.signature, EndLine: lineAt(text, method.end)})
		if method.unsupportedParams {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "方法参数注解未能静态解析", Line: lineAt(text, method.start), Confidence: Unresolved})
		}
	}

	// Calls and routes are reconstructed so a method parameter or local can
	// shadow a field and class-level Spring prefixes remain part of the route.
	kept := result.Edges[:0]
	for _, edge := range result.Edges {
		if edge.Kind != "calls" && edge.Kind != "route" {
			kept = append(kept, edge)
		}
	}
	result.Edges = kept
	fields := javaFields(masked[bodyStart+1:], imports, packageName, lineAt(text, bodyStart+1))
	for _, method := range methods {
		if method.bodyStart < 0 {
			continue
		}
		addConservativeCalls(result, text, masked, method, fields)
	}

	classAnnotations := javaWithoutComments(text[:typeMatch[0]])
	classPaths, _, classDynamic := mappingValues(classAnnotations)
	if classDynamic {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "Spring 映射包含动态路径，未生成 HTTP 合约", Line: lineAt(text, typeMatch[0]), Confidence: Unresolved})
	}
	feignService, feignPath, feignDynamic := feignMetadata(classAnnotations)
	if feignDynamic {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "Feign 元数据或 URL 覆盖未能安全匹配，未生成 HTTP 合约", Line: lineAt(text, typeMatch[0]), Confidence: Unresolved})
	}
	for _, method := range methods {
		annotationText := javaWithoutComments(text[method.start:method.headerEnd])
		paths, verb, dynamic := mappingValues(annotationText)
		if dynamic {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "Spring 映射包含动态路径，未生成 HTTP 合约", Line: lineAt(text, method.start), Confidence: Unresolved})
			continue
		}
		if len(paths) == 0 {
			continue
		}
		if kind != "controller" && feignService == "" {
			continue
		}
		if classDynamic && feignService == "" {
			continue
		}
		prefixes := []string{""}
		if kind == "controller" {
			prefixes = classPaths
			if len(prefixes) == 0 {
				prefixes = []string{""}
			}
		}
		if feignService != "" {
			prefixes = []string{feignPath}
		}
		for _, prefix := range prefixes {
			for _, path := range paths {
				role, service, reason := "provider", "", "Spring mapping"
				if feignService != "" {
					role, service, reason = "consumer", feignService, "Feign mapping"
				}
				fullPath := joinHTTPPath(prefix, path)
				result.Contracts = append(result.Contracts, Contract{Kind: "http", Role: role, Service: service, Key: verb + ":" + fullPath, Symbol: method.symbol, Signature: method.signature, Line: lineAt(text, method.start), Confidence: Certain, Reason: reason})
				if kind == "controller" {
					result.Edges = append(result.Edges, Edge{Source: method.symbol, SourceSignature: method.signature, Target: fullPath, Kind: "route", Line: lineAt(text, method.start), Confidence: Certain})
				}
			}
		}
	}

	addDubboContracts(result, text, masked, owner, imports, packageName, typeMatch, methods)
	addMQContracts(result, text, masked, owner, methods, fields)
}

func findJavaMethods(text, masked string, bodyStart int, owner string, imports map[string]string, packageName string) []javaMethod {
	var methods []javaMethod
	for _, match := range javaDeclarationPattern.FindAllStringSubmatchIndex(masked[bodyStart+1:], -1) {
		start := bodyStart + 1 + match[0]
		if braceDepth(masked, bodyStart, start) != 1 {
			continue
		}
		name := masked[bodyStart+1+match[2] : bodyStart+1+match[3]]
		paramsText := text[bodyStart+1+match[4] : bodyStart+1+match[5]]
		terminator := masked[bodyStart+1+match[6] : bodyStart+1+match[7]]
		end, methodBody := bodyStart+1+match[1]-1, -1
		if terminator == "{" {
			methodBody = bodyStart + 1 + match[1] - 1
			end = matchingBrace(masked, methodBody)
			if end < 0 {
				continue
			}
		}
		types, names, unsupportedParams := parseParameters(paramsText, imports, packageName)
		signature := owner + "." + name + "(" + strings.Join(types, ",") + ")"
		methods = append(methods, javaMethod{symbol: owner + "." + name, signature: signature, start: start, headerEnd: bodyStart + 1 + match[1], bodyStart: methodBody, end: end, params: names, unsupportedParams: unsupportedParams})
	}
	return methods
}

func parseParameters(text string, imports map[string]string, packageName string) ([]string, map[string]bool, bool) {
	var types []string
	names := map[string]bool{}
	unsupported := false
	for _, parameter := range splitJavaArgs(text) {
		var valid bool
		parameter, valid = stripJavaAnnotations(parameter)
		unsupported = unsupported || !valid
		parameter = strings.TrimSpace(strings.ReplaceAll(parameter, "final ", ""))
		parts := strings.Fields(parameter)
		if len(parts) < 2 {
			if parameter != "" {
				unsupported = true
			}
			continue
		}
		name := strings.TrimSuffix(parts[len(parts)-1], "[]")
		typeName := strings.Join(parts[:len(parts)-1], "")
		if strings.HasSuffix(name, "...") {
			name = strings.TrimSuffix(name, "...")
		}
		if strings.HasSuffix(typeName, "...") {
			typeName = strings.TrimSuffix(typeName, "...") + "[]"
		}
		types = append(types, qualifyJavaType(typeName, imports, packageName))
		names[name] = true
	}
	return types, names, unsupported
}

func addConservativeCalls(result *Result, text, masked string, method javaMethod, fields map[string]javaField) {
	body := masked[method.bodyStart+1 : method.end]
	for _, call := range javaReceiverCallPattern.FindAllStringSubmatchIndex(body, -1) {
		receiver := body[call[2]:call[3]]
		name := body[call[4]:call[5]]
		callStart := method.bodyStart + 1 + call[0]
		open := method.bodyStart + 1 + call[1] - 1
		close := matchingParen(masked, open)
		arity := -1
		if close >= 0 && close <= method.end {
			arity = len(splitJavaArgs(text[open+1 : close]))
		}
		field, isField := fields[receiver]
		shadowed := method.params[receiver] || localDeclaresBefore(body[:call[0]], receiver)
		if isField && !shadowed && arity >= 0 {
			arityCopy := arity
			result.Edges = append(result.Edges, Edge{Source: method.symbol, SourceSignature: method.signature, Target: field.typeName + "." + name, TargetArity: &arityCopy, Kind: "calls", Line: lineAt(text, callStart), Confidence: Certain})
			continue
		}
		if isField && shadowed {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "字段调用被参数或局部变量遮蔽，未绑定目标: " + receiver + "." + name, Line: lineAt(text, callStart), Confidence: Unresolved})
		} else if !isUbiquitousReceiver(receiver) {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "未绑定接收者调用，未生成调用关系: " + receiver + "." + name, Line: lineAt(text, callStart), Confidence: Unresolved})
		}
	}
}

func localDeclaresBefore(body, name string) bool {
	for _, match := range javaLocalPattern.FindAllStringSubmatch(body, -1) {
		if match[1] == name {
			return true
		}
	}
	return false
}

func isUbiquitousReceiver(name string) bool {
	switch name {
	case "log", "logger", "System", "Objects", "String", "Collections", "Optional", "Math", "Arrays":
		return true
	}
	return false
}

func addDubboContracts(result *Result, text, masked, owner string, imports map[string]string, packageName string, typeMatch []int, methods []javaMethod) {
	for _, match := range dubboPattern.FindAllStringSubmatchIndex(masked, -1) {
		_, args, _, valid := annotationArgs(javaWithoutComments(text[match[0]:]), "DubboReference")
		if !valid || dubboHasNonDefaultQualifier(args) {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "Dubbo group/version 未能安全跨服务匹配，未生成合约", Line: lineAt(text, match[0]), Confidence: Unresolved})
			continue
		}
		short := masked[match[2]:match[3]]
		result.Contracts = append(result.Contracts, Contract{Kind: "dubbo", Role: "consumer", Key: qualifyJavaType(short, imports, packageName), Symbol: owner, Line: lineAt(text, match[0]), Confidence: Certain, Reason: "@DubboReference"})
	}
	prefix := javaWithoutComments(text[:typeMatch[0]])
	if !strings.Contains(prefix, "@DubboService") {
		return
	}
	_, args, _, valid := annotationArgs(prefix, "DubboService")
	if !valid || dubboHasNonDefaultQualifier(args) {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "Dubbo group/version 未能安全跨服务匹配，未生成合约", Line: lineAt(text, typeMatch[0]), Confidence: Unresolved})
		return
	}
	headerEnd := strings.Index(masked[typeMatch[1]:], "{")
	if headerEnd < 0 {
		return
	}
	header := masked[typeMatch[1] : typeMatch[1]+headerEnd]
	for _, name := range interfaceNames(header) {
		result.Contracts = append(result.Contracts, Contract{Kind: "dubbo", Role: "provider", Key: qualifyJavaType(name, imports, packageName), Symbol: owner, Line: lineAt(text, typeMatch[0]), Confidence: Certain, Reason: "@DubboService implements interface"})
	}
}

func interfaceNames(header string) []string {
	index := strings.Index(header, "implements")
	if index < 0 {
		return nil
	}
	var result []string
	for _, name := range strings.Split(header[index+len("implements"):], ",") {
		parts := strings.Fields(name)
		if len(parts) == 0 {
			continue
		}
		name = strings.Trim(strings.Split(parts[0], "<")[0], "<>")
		if name != "" {
			result = append(result, name)
		}
	}
	return result
}

func addMQContracts(result *Result, text, masked, owner string, methods []javaMethod, fields map[string]javaField) {
	listener := regexp.MustCompile(`@([A-Za-z]+)(?:Listener)\s*\(([^)]*)\)`)
	for _, match := range listener.FindAllStringSubmatchIndex(masked, -1) {
		annotation := masked[match[2]:match[3]]
		broker := brokerFor(annotation)
		for _, topic := range listenerTopics(text[match[0]:match[1]]) {
			method := methodAt(methods, match[0])
			symbol, signature := owner, ""
			if method != nil {
				symbol, signature = method.symbol, method.signature
			}
			result.Contracts = append(result.Contracts, Contract{Kind: "mq", Role: "consumer", Key: topic, Symbol: symbol, Signature: signature, Line: lineAt(text, match[0]), Confidence: Certain, Reason: "message listener literal topic", Broker: broker})
		}
	}
	publish := regexp.MustCompile(`\b(\w+)\s*\.\s*(?:send|convertAndSend|syncSend)\s*\(`)
	for _, match := range publish.FindAllStringSubmatchIndex(masked, -1) {
		open := match[1] - 1
		close := matchingParen(masked, open)
		if close < 0 {
			continue
		}
		args := splitJavaArgs(text[open+1 : close])
		if len(args) == 0 || !isStringLiteral(args[0]) {
			continue
		}
		method := methodAt(methods, match[0])
		if method == nil {
			continue
		}
		receiver := masked[match[2]:match[3]]
		field, found := fields[receiver]
		if !found || method.params[receiver] || localDeclaresBefore(masked[method.bodyStart+1:match[0]], receiver) {
			continue
		}
		broker := mqBrokerForFieldType(field.typeName)
		if broker == "" {
			continue
		}
		if broker == "rabbit" {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "RabbitMQ 发布参数可能是 exchange/routingKey，未生成 MQ 合约", Line: lineAt(text, match[0]), Confidence: Unresolved})
			continue
		}
		topic := unquoteJava(args[0])
		if broker == "rocketmq" && strings.Contains(topic, ":") {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "RocketMQ destination 含 tag，未生成精确 MQ 合约", Line: lineAt(text, match[0]), Confidence: Unresolved})
			continue
		}
		result.Contracts = append(result.Contracts, Contract{Kind: "mq", Role: "provider", Key: topic, Symbol: method.symbol, Signature: method.signature, Line: lineAt(text, match[0]), Confidence: Certain, Reason: "message publish literal topic", Broker: broker})
	}
}

func methodAt(methods []javaMethod, offset int) *javaMethod {
	for i := range methods {
		if methods[i].start <= offset && offset <= methods[i].end {
			return &methods[i]
		}
	}
	return nil
}

func mappingValues(annotationText string) ([]string, string, bool) {
	name, args, found, valid := annotationArgs(annotationText, "Get", "Post", "Put", "Delete", "Patch", "Request")
	if !found {
		return nil, "", false
	}
	if !valid {
		return nil, "", true
	}
	verb := strings.ToUpper(name)
	if verb == "REQUEST" {
		verb = "ANY"
	}
	if strings.TrimSpace(args) == "" {
		return []string{""}, verb, false
	}
	var paths []string
	hasPath := false
	for _, part := range splitJavaArgs(args) {
		key, value, named := annotationAssignment(part)
		if !named {
			if len(splitJavaArgs(args)) != 1 || hasPath {
				return nil, verb, true
			}
			values, literal := literalStrings(value)
			if !literal {
				return nil, verb, true
			}
			paths, hasPath = values, true
			continue
		}
		switch key {
		case "path", "value":
			if hasPath {
				return nil, verb, true
			}
			values, literal := literalStrings(value)
			if !literal {
				return nil, verb, true
			}
			paths, hasPath = values, true
		case "method":
			resolved, single := requestMethod(value)
			if !single {
				return nil, verb, true
			}
			verb = resolved
		}
	}
	if !hasPath {
		return []string{""}, verb, false
	}
	return paths, verb, false
}

func feignMetadata(annotationText string) (string, string, bool) {
	_, args, found, valid := annotationArgs(annotationText, "FeignClient")
	if !found {
		return "", "", false
	}
	if !valid || strings.TrimSpace(args) == "" {
		return "", "", true
	}
	service, path, hasService := "", "", false
	for _, part := range splitJavaArgs(args) {
		key, value, named := annotationAssignment(part)
		if !named {
			if hasService {
				return "", "", true
			}
			values, literal := literalStrings(value)
			if !literal || len(values) != 1 {
				return "", "", true
			}
			service, hasService = values[0], true
			continue
		}
		switch key {
		case "name", "value":
			if hasService {
				return "", "", true
			}
			values, literal := literalStrings(value)
			if !literal || len(values) != 1 {
				return "", "", true
			}
			service, hasService = values[0], true
		case "path":
			values, literal := literalStrings(value)
			if !literal || len(values) != 1 {
				return "", "", true
			}
			path = values[0]
		case "url":
			// Fixed URLs bypass logical service discovery and cannot safely bridge.
			return "", "", true
		}
	}
	return service, path, !hasService
}

func dubboHasNonDefaultQualifier(args string) bool {
	for _, part := range splitJavaArgs(args) {
		key, value, named := annotationAssignment(part)
		if !named || (key != "group" && key != "version") {
			continue
		}
		values, literal := literalStrings(value)
		if !literal || len(values) != 1 || values[0] != "" {
			return true
		}
	}
	return false
}

func annotationArgs(text string, names ...string) (string, string, bool, bool) {
	for _, name := range names {
		annotation := "@" + name
		if name != "FeignClient" && name != "DubboService" && name != "DubboReference" && !strings.HasSuffix(name, "Listener") {
			annotation += "Mapping"
		}
		match := regexp.MustCompile(regexp.QuoteMeta(annotation) + `\b`).FindStringIndex(text)
		if match == nil {
			continue
		}
		index := match[1]
		for index < len(text) && (text[index] == ' ' || text[index] == '\t' || text[index] == '\r' || text[index] == '\n') {
			index++
		}
		if index == len(text) || text[index] != '(' {
			return name, "", true, true
		}
		end := matchingParenJava(text, index)
		if end < 0 {
			return name, "", true, false
		}
		return name, text[index+1 : end], true, true
	}
	return "", "", false, true
}

func annotationAssignment(part string) (string, string, bool) {
	depth := 0
	quote, escaped := byte(0), false
	for i := 0; i < len(part); i++ {
		ch := part[i]
		if quote != 0 {
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		switch ch {
		case '(', '[', '{', '<':
			depth++
		case ')', ']', '}', '>':
			if depth > 0 {
				depth--
			}
		case '=':
			if depth == 0 {
				key := strings.TrimSpace(part[:i])
				if regexp.MustCompile(`^[A-Za-z_]\w*$`).MatchString(key) {
					return key, strings.TrimSpace(part[i+1:]), true
				}
				return "", "", false
			}
		}
	}
	return "", strings.TrimSpace(part), false
}

func literalStrings(value string) ([]string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, false
	}
	if value[0] == '"' {
		literal, end, ok := javaStringAt(value, 0)
		return []string{literal}, ok && strings.TrimSpace(value[end:]) == ""
	}
	if value[0] != '{' || value[len(value)-1] != '}' {
		return nil, false
	}
	parts := splitJavaArgs(value[1 : len(value)-1])
	if len(parts) == 0 {
		return nil, false
	}
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		literal, end, ok := javaStringAt(strings.TrimSpace(part), 0)
		if !ok || strings.TrimSpace(strings.TrimSpace(part)[end:]) != "" {
			return nil, false
		}
		values = append(values, literal)
	}
	return values, true
}

func javaStringAt(value string, start int) (string, int, bool) {
	if start >= len(value) || value[start] != '"' {
		return "", start, false
	}
	var builder strings.Builder
	escaped := false
	for i := start + 1; i < len(value); i++ {
		if escaped {
			builder.WriteByte(value[i])
			escaped = false
			continue
		}
		if value[i] == '\\' {
			escaped = true
			continue
		}
		if value[i] == '"' {
			return builder.String(), i + 1, true
		}
		builder.WriteByte(value[i])
	}
	return "", len(value), false
}

func requestMethod(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "{") && strings.HasSuffix(value, "}") {
		value = strings.TrimSpace(value[1 : len(value)-1])
	}
	parts := splitJavaArgs(value)
	if len(parts) != 1 {
		return "", false
	}
	match := regexp.MustCompile(`^RequestMethod\.(GET|POST|PUT|DELETE|PATCH)$`).FindStringSubmatch(strings.TrimSpace(parts[0]))
	if match == nil {
		return "", false
	}
	return match[1], true
}

func maskAnnotationArguments(text string) string {
	masked := []byte(text)
	for i := 0; i < len(masked); i++ {
		if masked[i] != '@' {
			continue
		}
		index := i + 1
		for index < len(masked) && (masked[index] == '.' || masked[index] == '_' || masked[index] >= 'A' && masked[index] <= 'Z' || masked[index] >= 'a' && masked[index] <= 'z' || masked[index] >= '0' && masked[index] <= '9') {
			index++
		}
		for index < len(masked) && (masked[index] == ' ' || masked[index] == '\t') {
			index++
		}
		if index >= len(masked) || masked[index] != '(' {
			continue
		}
		end := matchingParen(string(masked), index)
		if end < 0 {
			continue
		}
		for j := index; j <= end; j++ {
			if masked[j] != '\n' {
				masked[j] = ' '
			}
		}
		i = end
	}
	return string(masked)
}

func stripJavaAnnotations(value string) (string, bool) {
	var result strings.Builder
	valid := true
	for i := 0; i < len(value); {
		if value[i] != '@' {
			result.WriteByte(value[i])
			i++
			continue
		}
		i++
		for i < len(value) && (value[i] == '.' || value[i] == '_' || value[i] >= 'A' && value[i] <= 'Z' || value[i] >= 'a' && value[i] <= 'z' || value[i] >= '0' && value[i] <= '9') {
			i++
		}
		for i < len(value) && (value[i] == ' ' || value[i] == '\t') {
			i++
		}
		if i < len(value) && value[i] == '(' {
			end := matchingParenJava(value, i)
			if end < 0 {
				valid = false
				break
			}
			i = end + 1
		}
	}
	return result.String(), valid
}

func annotationStrings(text string) []string {
	var values []string
	for _, match := range javaStringPattern.FindAllStringSubmatch(text, -1) {
		values = append(values, strings.ReplaceAll(match[1], `\"`, `"`))
	}
	return values
}

func listenerTopics(annotation string) []string {
	match := regexp.MustCompile(`@([A-Za-z]+Listener)\b`).FindStringSubmatch(annotation)
	if match == nil {
		return nil
	}
	_, args, found, valid := annotationArgs(annotation, match[1])
	if !found || !valid {
		return nil
	}
	parts := splitJavaArgs(args)
	if len(parts) == 1 {
		_, value, named := annotationAssignment(parts[0])
		if !named {
			values, literal := literalStrings(value)
			if literal {
				return values
			}
			return nil
		}
	}
	for _, part := range parts {
		key, value, named := annotationAssignment(part)
		if !named || (key != "topics" && key != "topic" && key != "value") {
			continue
		}
		values, literal := literalStrings(value)
		if literal {
			return values
		}
	}
	return nil
}

func joinHTTPPath(prefix, path string) string {
	if prefix == "" {
		return path
	}
	if path == "" {
		return prefix
	}
	return "/" + strings.Trim(prefix, "/") + "/" + strings.Trim(path, "/")
}

func brokerFor(value string) string {
	lower := strings.ToLower(value)
	switch {
	case strings.Contains(lower, "kafka"):
		return "kafka"
	case strings.Contains(lower, "rabbit"):
		return "rabbit"
	case strings.Contains(lower, "rocket"):
		return "rocketmq"
	}
	return ""
}

func mqBrokerForFieldType(typeName string) string {
	switch {
	case strings.HasSuffix(typeName, ".KafkaTemplate") || typeName == "KafkaTemplate":
		return "kafka"
	case strings.HasSuffix(typeName, ".RabbitTemplate") || typeName == "RabbitTemplate":
		return "rabbit"
	case strings.HasSuffix(typeName, ".RocketMQTemplate") || typeName == "RocketMQTemplate":
		return "rocketmq"
	}
	return ""
}

func splitJavaArgs(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var result []string
	start, depth := 0, 0
	quote, escaped := byte(0), false
	for i := 0; i < len(text); i++ {
		if quote != 0 {
			if escaped {
				escaped = false
			} else if text[i] == '\\' {
				escaped = true
			} else if text[i] == quote {
				quote = 0
			}
			continue
		}
		if text[i] == '"' || text[i] == '\'' {
			quote = text[i]
			continue
		}
		switch text[i] {
		case '(', '[', '{', '<':
			depth++
		case ')', ']', '}', '>':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				result = append(result, text[start:i])
				start = i + 1
			}
		}
	}
	return append(result, text[start:])
}

func matchingParen(text string, start int) int {
	depth := 0
	for i := start; i < len(text); i++ {
		if text[i] == '(' {
			depth++
		}
		if text[i] == ')' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func matchingParenJava(text string, start int) int {
	depth := 0
	quote, escaped := byte(0), false
	for i := start; i < len(text); i++ {
		if quote != 0 {
			if escaped {
				escaped = false
			} else if text[i] == '\\' {
				escaped = true
			} else if text[i] == quote {
				quote = 0
			}
			continue
		}
		if text[i] == '"' || text[i] == '\'' {
			quote = text[i]
			continue
		}
		if text[i] == '(' {
			depth++
		}
		if text[i] == ')' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func isStringLiteral(value string) bool {
	value = strings.TrimSpace(value)
	return len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"'
}
func unquoteJava(value string) string { return strings.Trim(strings.TrimSpace(value), `"`) }

func javaWithoutComments(text string) string {
	masked := []byte(text)
	quote, escaped := byte(0), false
	for i := 0; i < len(masked); i++ {
		if quote != 0 {
			if escaped {
				escaped = false
			} else if masked[i] == '\\' {
				escaped = true
			} else if masked[i] == quote {
				quote = 0
			}
			continue
		}
		if masked[i] == '"' || masked[i] == '\'' {
			quote = masked[i]
			continue
		}
		if masked[i] == '/' && i+1 < len(masked) && masked[i+1] == '/' {
			for i < len(masked) && masked[i] != '\n' {
				masked[i] = ' '
				i++
			}
			continue
		}
		if masked[i] == '/' && i+1 < len(masked) && masked[i+1] == '*' {
			masked[i], masked[i+1] = ' ', ' '
			i += 2
			for i+1 < len(masked) && !(masked[i] == '*' && masked[i+1] == '/') {
				if masked[i] != '\n' {
					masked[i] = ' '
				}
				i++
			}
			if i+1 < len(masked) {
				masked[i], masked[i+1] = ' ', ' '
				i++
			}
		}
	}
	return string(masked)
}
