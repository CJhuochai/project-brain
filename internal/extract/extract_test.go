package extract

import "testing"

func TestFileExtractsSpringRouteAndMapperEvidence(t *testing.T) {
	content := []byte(`package com.example.entry;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
public class EntryController {
  @PostMapping("/entry/submit")
  public void submit() {}
}`)
	result := File("src/main/java/com/example/entry/EntryController.java", content)
	if !hasSymbol(result.Symbols, "com.example.entry.EntryController", "controller") {
		t.Fatalf("controller symbol missing: %#v", result.Symbols)
	}
	if !hasEdge(result.Edges, "route", "/entry/submit", Certain) {
		t.Fatalf("route evidence missing: %#v", result.Edges)
	}
}

func TestFileExtractsMyBatisNamespaceStatementAndTable(t *testing.T) {
	content := []byte(`<mapper namespace="com.example.EntryMapper">
  <select id="findById">select * from student_entry where id = #{id}</select>
</mapper>`)
	result := File("EntryMapper.xml", content)
	if !hasSymbol(result.Symbols, "com.example.EntryMapper.findById", "mapper_statement") {
		t.Fatalf("mapper statement missing: %#v", result.Symbols)
	}
	if !hasEdge(result.Edges, "queries_table", "student_entry", Certain) {
		t.Fatalf("table edge missing: %#v", result.Edges)
	}
}

func TestFileExtractsServiceDependencyAsProbableEvidence(t *testing.T) {
	content := []byte(`package com.example;
import org.springframework.stereotype.Service;
@Service
class EntryService {
  private final EntryMapper entryMapper = null;
}`)
	result := File("EntryService.java", content)
	if !hasSymbol(result.Symbols, "com.example.EntryService", "service") {
		t.Fatalf("service symbol missing: %#v", result.Symbols)
	}
	if !hasEdge(result.Edges, "uses", "EntryMapper", Probable) {
		t.Fatalf("dependency edge missing: %#v", result.Edges)
	}
}

func TestFileExtractsMavenModuleDependency(t *testing.T) {
	result := File("pom.xml", []byte(`<project><artifactId>entry-service</artifactId><dependencies><dependency><artifactId>common-core</artifactId></dependency></dependencies></project>`))
	if !hasSymbol(result.Symbols, "maven:entry-service", "maven_module") || !hasEdge(result.Edges, "depends_on_module", "maven:common-core", Certain) {
		t.Fatalf("maven evidence=%#v", result)
	}
}

func hasSymbol(symbols []Symbol, name, kind string) bool {
	for _, symbol := range symbols {
		if symbol.Name == name && symbol.Kind == kind {
			return true
		}
	}
	return false
}

func hasEdge(edges []Edge, kind, target string, confidence Confidence) bool {
	for _, edge := range edges {
		if edge.Kind == kind && edge.Target == target && edge.Confidence == confidence {
			return true
		}
	}
	return false
}
