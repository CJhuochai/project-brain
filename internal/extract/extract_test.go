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
