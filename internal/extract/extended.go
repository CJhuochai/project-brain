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
		extractConfigContracts(path, text, &result)
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

type configEntry struct {
	key, value string
	line       int
}

// extractConfigContracts only accepts literal values. Broker configuration is
// emitted separately so callers can associate it with nearby MQ contracts.
func extractConfigContracts(path, text string, result *Result) {
	for _, entry := range configEntries(text) {
		key := strings.ToLower(entry.key)
		if strings.Contains(entry.value, "${") || strings.Contains(entry.value, "#{") {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "动态服务或消息代理配置未能静态确定", Line: entry.line, Confidence: Unresolved})
			continue
		}
		switch key {
		case "spring.application.name":
			result.Contracts = append(result.Contracts, Contract{Kind: "service", Role: "provider", Service: entry.value, Key: entry.value, Symbol: "config:" + path, Line: entry.line, Confidence: Certain, Reason: "spring.application.name"})
		case "spring.kafka.bootstrap-servers", "spring.kafka.bootstrapservers":
			result.Contracts = append(result.Contracts, brokerConfigContract(path, entry, "kafka"))
		case "spring.rabbitmq.addresses", "spring.rabbitmq.host":
			result.Contracts = append(result.Contracts, brokerConfigContract(path, entry, "rabbit"))
		case "rocketmq.name-server", "rocketmq.nameserver":
			result.Contracts = append(result.Contracts, brokerConfigContract(path, entry, "rocketmq"))
		}
	}
}

func brokerConfigContract(path string, entry configEntry, broker string) Contract {
	return Contract{Kind: "broker", Role: "provider", Key: broker, Symbol: "config:" + path, Line: entry.line, Confidence: Certain, Reason: "message broker configuration", Broker: broker + ":" + normalizeBrokerAddress(entry.value)}
}

func normalizeBrokerAddress(value string) string { return strings.Join(strings.Fields(value), "") }

func configEntries(text string) []configEntry {
	var entries []configEntry
	type parent struct {
		indent int
		key    string
	}
	var parents []parent
	for lineNumber, line := range strings.Split(text, "\n") {
		raw := strings.TrimRight(line, " \t\r")
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if equals := strings.Index(trimmed, "="); equals > 0 {
			entries = append(entries, configEntry{key: strings.TrimSpace(trimmed[:equals]), value: unquoteConfig(strings.TrimSpace(trimmed[equals+1:])), line: lineNumber + 1})
			continue
		}
		colon := strings.Index(trimmed, ":")
		if colon <= 0 {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		for len(parents) > 0 && parents[len(parents)-1].indent >= indent {
			parents = parents[:len(parents)-1]
		}
		key, value := strings.Trim(strings.TrimSpace(trimmed[:colon]), `"'`), strings.TrimSpace(trimmed[colon+1:])
		parts := make([]string, 0, len(parents)+1)
		for _, item := range parents {
			parts = append(parts, item.key)
		}
		parts = append(parts, key)
		if value == "" {
			parents = append(parents, parent{indent: indent, key: key})
			continue
		}
		entries = append(entries, configEntry{key: strings.Join(parts, "."), value: unquoteConfig(value), line: lineNumber + 1})
	}
	return entries
}

func unquoteConfig(value string) string {
	value = strings.TrimSpace(strings.SplitN(value, " #", 2)[0])
	return strings.Trim(value, `"'`)
}
