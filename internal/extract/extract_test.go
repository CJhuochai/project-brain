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

func TestFileExtractsMethodLevelSpringAndMyBatisPath(t *testing.T) {
	controller := File("EntryController.java", []byte(`package com.example.entry;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RestController;
import com.example.application.EntryApplication;

@RestController
class EntryController {
  private final EntryApplication application;
  @PostMapping("/entry/submit")
  public void submit() { application.submit(); }
}`))
	if !hasSymbol(controller.Symbols, "com.example.entry.EntryController.submit", "controller_method") || !hasEdgeFrom(controller.Edges, "calls", "com.example.entry.EntryController.submit", "com.example.application.EntryApplication.submit", Certain) {
		t.Fatalf("controller evidence=%#v", controller)
	}

	application := File("EntryApplication.java", []byte(`package com.example.application;
import org.springframework.stereotype.Service;
import com.example.infrastructure.EntryMapper;

@Service
class EntryApplication {
  private final EntryMapper entryMapper;
  public void submit() { entryMapper.insert(); }
}`))
	if !hasSymbol(application.Symbols, "com.example.application.EntryApplication.submit", "application_method") || !hasEdgeFrom(application.Edges, "calls", "com.example.application.EntryApplication.submit", "com.example.infrastructure.EntryMapper.insert", Certain) {
		t.Fatalf("application evidence=%#v", application)
	}

	mapper := File("EntryMapper.xml", []byte(`<mapper namespace="com.example.infrastructure.EntryMapper"><insert id="insert">INSERT INTO student_entry(id) VALUES (1)</insert></mapper>`))
	if !hasEdgeFrom(mapper.Edges, "queries_table", "com.example.infrastructure.EntryMapper.insert", "student_entry", Certain) {
		t.Fatalf("mapper evidence=%#v", mapper)
	}
}

func TestFileDoesNotTreatMethodLocalAsInjectedDependency(t *testing.T) {
	result := File("EntryApplication.java", []byte(`package com.example.application;
import org.springframework.stereotype.Service;
import com.example.infrastructure.EntryMapper;
@Service
class EntryApplication {
  public void submit() {
    EntryMapper localMapper = null;
    localMapper.insert();
  }
}`))
	if hasEdgeFrom(result.Edges, "calls", "com.example.application.EntryApplication.submit", "com.example.infrastructure.EntryMapper.insert", Certain) {
		t.Fatalf("method local must not be a certain injected call: %#v", result.Edges)
	}
}

func TestFileDoesNotTreatCommentAsSpringAnnotation(t *testing.T) {
	result := File("Plain.java", []byte(`package com.example;
// @RestController
class Plain {}`))
	if !hasSymbol(result.Symbols, "com.example.Plain", "class") {
		t.Fatalf("comment changed component kind: %#v", result.Symbols)
	}
}

func TestFileExtractsMavenModuleDependency(t *testing.T) {
	result := File("pom.xml", []byte(`<project><artifactId>entry-service</artifactId><dependencies><dependency><artifactId>common-core</artifactId></dependency></dependencies></project>`))
	if !hasSymbol(result.Symbols, "maven:entry-service", "maven_module") || !hasEdge(result.Edges, "depends_on_module", "maven:common-core", Certain) {
		t.Fatalf("maven evidence=%#v", result)
	}
}

func TestFileExtractsImportInheritanceAndFeignEvidence(t *testing.T) {
	result := File("Client.java", []byte(`package com.example;
import com.example.BaseClient;
@FeignClient(name = "student")
class Client extends BaseClient {}`))
	if !hasEdge(result.Edges, "imports", "com.example.BaseClient", Certain) || !hasEdge(result.Edges, "implements", "BaseClient", Probable) || !hasEdge(result.Edges, "feign_client", "feign:student", Certain) {
		t.Fatalf("edges=%#v", result.Edges)
	}
}

func TestFileReportsUnresolvedMapperWithoutNamespace(t *testing.T) {
	result := File("Mapper.xml", []byte(`<mapper><select id="find">select 1</select></mapper>`))
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Confidence != Unresolved {
		t.Fatalf("diagnostics=%#v", result.Diagnostics)
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

func hasEdgeFrom(edges []Edge, kind, source, target string, confidence Confidence) bool {
	for _, edge := range edges {
		if edge.Kind == kind && edge.Source == source && edge.Target == target && edge.Confidence == confidence {
			return true
		}
	}
	return false
}
