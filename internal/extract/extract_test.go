package extract

import (
	"strings"
	"testing"
)

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

func TestFileExtractsMicroserviceRelationships(t *testing.T) {
	result := File("EntryService.java", []byte(`class EntryService {
  @DubboReference private EntryApi entryApi;
  @KafkaListener(topics = "entry.created") void consume() {}
  void send() { kafkaTemplate.send("entry.created", "x"); }
  @Scheduled(cron = "0 * * * * *") void sync() {}
}`))
	for _, want := range []struct{ kind, target string }{{"dubbo_reference", "dubbo:EntryApi"}, {"consumes_topic", "mq:entry.created"}, {"publishes_topic", "mq:entry.created"}, {"scheduled_job", "schedule:EntryService#sync"}} {
		if !hasEdge(result.Edges, want.kind, want.target, Certain) {
			t.Fatalf("missing=%#v edges=%#v", want, result.Edges)
		}
	}
}

func TestFileMarksDynamicBehaviorUnresolved(t *testing.T) {
	result := File("Mapper.xml", []byte(`<mapper namespace="X"><select id="x">select * from ${table}<if test="x"> where id=1</if></select></mapper>`))
	if !hasDiagnostic(result.Diagnostics, "动态 SQL") {
		t.Fatalf("diagnostics=%#v", result.Diagnostics)
	}
}

func TestFileExtractsConfigMigrationAndHTTPContracts(t *testing.T) {
	for _, sample := range []struct {
		path, content, kind, target string
	}{
		{"application.yml", "entry.enabled: true", "defines_config", "config:entry.enabled"},
		{"V1__entry.sql", "create table student_entry(id bigint)", "migration_table", "table:student_entry"},
		{"openapi.yaml", "/api/entry:\n  post: {}", "api_contract", "http:ANY:/api/entry"},
		{"entry.ts", `axios.post("/api/entry", body)`, "calls_http", "http:POST:/api/entry"},
	} {
		if !hasEdge(File(sample.path, []byte(sample.content)).Edges, sample.kind, sample.target, Certain) {
			t.Fatalf("missing %s from %s", sample.kind, sample.path)
		}
	}
}

func TestJavaV2CapturesOverloadSignaturesAndCallArity(t *testing.T) {
	result := File("Worker.java", []byte(`package com.example;
import com.remote.Api;
class Worker {
  private Api api;
  void submit(int id) { api.save(id); }
  void submit(String id) { api.save(); }
}`))
	if !hasSignature(result.Symbols, "com.example.Worker.submit", "com.example.Worker.submit(int)") || !hasSignature(result.Symbols, "com.example.Worker.submit", "com.example.Worker.submit(java.lang.String)") {
		t.Fatalf("overload signatures=%#v", result.Symbols)
	}
	arities := map[int]bool{}
	for _, edge := range result.Edges {
		if edge.Kind == "calls" && edge.Target == "com.remote.Api.save" && edge.TargetArity != nil {
			arities[*edge.TargetArity] = true
			if edge.SourceSignature == "" {
				t.Fatalf("missing source signature: %#v", edge)
			}
		}
	}
	if !arities[0] || !arities[1] {
		t.Fatalf("call arities=%#v", result.Edges)
	}
}

func TestJavaV2DoesNotBindShadowedFieldCall(t *testing.T) {
	result := File("Worker.java", []byte(`package com.example;
import com.remote.Api;
class Worker {
  private Api api;
  void parameter(Api api) { api.save(); }
  void local() { Api api = null; api.save(); }
}`))
	for _, edge := range result.Edges {
		if edge.Kind == "calls" && edge.Target == "com.remote.Api.save" {
			t.Fatalf("shadowed call=%#v", edge)
		}
	}
	if !hasDiagnostic(result.Diagnostics, "遮蔽") {
		t.Fatalf("shadow diagnostic=%#v", result.Diagnostics)
	}
}

func TestJavaV2ExtractsSpringAndFeignContracts(t *testing.T) {
	controller := File("EntryController.java", []byte(`package com.example;
import org.springframework.web.bind.annotation.*;
@RestController
@RequestMapping(path = {"/api", "/v2"})
class EntryController {
  @RequestMapping(value = {"/entry", "/submit"}, method = RequestMethod.POST)
  void submit() {}
}`))
	for _, key := range []string{"POST:/api/entry", "POST:/api/submit", "POST:/v2/entry", "POST:/v2/submit"} {
		if !hasContract(controller.Contracts, "http", "provider", "", key, "") {
			t.Fatalf("missing controller %s: %#v", key, controller.Contracts)
		}
	}
	feign := File("EntryClient.java", []byte(`package com.example;
import org.springframework.web.bind.annotation.GetMapping;
@FeignClient(name = "student", path = "/remote")
interface EntryClient {
  @GetMapping(value = "/entry")
  String find();
}`))
	if !hasSymbol(feign.Symbols, "com.example.EntryClient.find", "class_method") || !hasContract(feign.Contracts, "http", "consumer", "student", "GET:/remote/entry", "") {
		t.Fatalf("feign=%#v", feign)
	}
	for _, symbol := range feign.Symbols {
		if symbol.Name == "com.example.EntryClient.find" && (symbol.EndLine != 6 || symbol.Signature != "com.example.EntryClient.find()") {
			t.Fatalf("interface method range=%#v", symbol)
		}
	}
}

func TestJavaV2ExtractsQualifiedDubboAndLiteralMQContracts(t *testing.T) {
	result := File("EntryService.java", []byte(`package com.example;
import com.api.EntryApi;
import org.springframework.kafka.core.KafkaTemplate;
@DubboService
class EntryService implements EntryApi {
  @DubboReference private EntryApi entryApi;
  private KafkaTemplate kafkaTemplate;
  @KafkaListener(topics = "entry.created") void receive() {}
  void send() { kafkaTemplate.send("entry.created", "x"); }
}`))
	if !hasContract(result.Contracts, "dubbo", "consumer", "", "com.api.EntryApi", "") || !hasContract(result.Contracts, "dubbo", "provider", "", "com.api.EntryApi", "") || !hasContract(result.Contracts, "mq", "consumer", "", "entry.created", "kafka") || !hasContract(result.Contracts, "mq", "provider", "", "entry.created", "kafka") {
		t.Fatalf("contracts=%#v", result.Contracts)
	}
}

func TestConfigContractsExtractServiceAndBrokers(t *testing.T) {
	result := File("src/main/resources/application.yml", []byte(`spring:
  application:
    name: entry-service
  kafka:
    bootstrap-servers: kafka-1:9092
  rabbitmq:
    addresses: rabbit-1:5672
rocketmq:
  name-server: rocket-1:9876
# spring.application.name: ignored
`))
	if !hasContract(result.Contracts, "service", "provider", "entry-service", "entry-service", "") || !hasContract(result.Contracts, "broker", "provider", "", "kafka", "kafka:kafka-1:9092") || !hasContract(result.Contracts, "broker", "provider", "", "rabbit", "rabbit:rabbit-1:5672") || !hasContract(result.Contracts, "broker", "provider", "", "rocketmq", "rocketmq:rocket-1:9876") {
		t.Fatalf("config contracts=%#v", result.Contracts)
	}
}

func TestJavaV2IgnoresCommentedContractAnnotation(t *testing.T) {
	result := File("EntryController.java", []byte(`package com.example;
import org.springframework.web.bind.annotation.*;
@RestController
class EntryController {
  // @GetMapping("/not-a-route")
  @GetMapping("/route")
  void route() {}
}`))
	if hasContract(result.Contracts, "http", "provider", "", "GET:/not-a-route", "") || !hasContract(result.Contracts, "http", "provider", "", "GET:/route", "") {
		t.Fatalf("comment contract=%#v", result.Contracts)
	}
}

func TestJavaV2OnlyUsesStaticMappingPathsAndAnnotatedParameters(t *testing.T) {
	result := File("C.java", []byte(`package p;
import org.springframework.web.bind.annotation.*;
import org.springframework.kafka.core.KafkaTemplate;
@RestController
@RequestMapping("/base")
class C {
  private Api service;
  private KafkaTemplate kafka;
  @GetMapping(path = "/real", produces = "application/json", headers = "X-Mode=fast")
  void run(@RequestParam("id") String id) { service.run(); kafka.send("a,b", id); }
  @GetMapping
  void root() {}
}`))
	if !hasContract(result.Contracts, "http", "provider", "", "GET:/base/real", "") || !hasContract(result.Contracts, "http", "provider", "", "GET:/base", "") || hasContract(result.Contracts, "http", "provider", "", "GET:application/json", "") || hasContract(result.Contracts, "http", "provider", "", "GET:X-Mode=fast", "") {
		t.Fatalf("mapping contracts=%#v", result.Contracts)
	}
	if !hasEdgeFrom(result.Edges, "calls", "p.C.run", "p.Api.run", Certain) || !hasContract(result.Contracts, "mq", "provider", "", "a,b", "kafka") {
		t.Fatalf("annotated parameter or string comma call lost: %#v", result)
	}
}

func TestJavaV2SuppressesUnsafeExternalContracts(t *testing.T) {
	feign := File("C.java", []byte(`package p;
@FeignClient(name="students", url="http://foreign-host")
interface C { @GetMapping("/real") String run(); }`))
	if len(feign.Contracts) != 0 || !hasDiagnostic(feign.Diagnostics, "Feign") {
		t.Fatalf("fixed-url feign=%#v", feign)
	}
	mq := File("Publisher.java", []byte(`package p;
import org.springframework.amqp.rabbit.core.RabbitTemplate;
import org.apache.rocketmq.spring.core.RocketMQTemplate;
class Publisher {
  private Email email;
  private RabbitTemplate rabbit;
  private RocketMQTemplate rocket;
  void send() { email.send("orders", "x"); rabbit.convertAndSend("orders", "x"); rocket.syncSend("orders:tag", "x"); }
}`))
	for _, contract := range mq.Contracts {
		if contract.Kind == "mq" && contract.Role == "provider" {
			t.Fatalf("unsafe publisher contract=%#v", contract)
		}
	}
	if !hasDiagnostic(mq.Diagnostics, "RabbitMQ") || !hasDiagnostic(mq.Diagnostics, "RocketMQ") {
		t.Fatalf("mq diagnostics=%#v", mq.Diagnostics)
	}
}

func TestJavaV2KeepsDubboQualifiersAndRebuildsOverloads(t *testing.T) {
	dubbo := File("C.java", []byte(`package p;
import api.OrderApi;
@DubboService(group="blue", version="2") class C implements OrderApi {
  @DubboReference(group="red", version="1") private OrderApi api;
}`))
	for _, contract := range dubbo.Contracts {
		if contract.Kind == "dubbo" {
			t.Fatalf("qualified dubbo contract=%#v", contract)
		}
	}
	if !hasDiagnostic(dubbo.Diagnostics, "Dubbo group/version") {
		t.Fatalf("dubbo diagnostics=%#v", dubbo.Diagnostics)
	}
	overloads := File("C.java", []byte(`package p;
class C {
  void run(int x) {}
  void run(String x) {}
}`))
	seen := map[string]bool{}
	for _, symbol := range overloads.Symbols {
		if symbol.Name == "p.C.run" {
			if symbol.Signature == "" {
				t.Fatalf("ghost method=%#v", overloads.Symbols)
			}
			seen[symbol.Signature] = true
		}
	}
	if !seen["p.C.run(int)"] || !seen["p.C.run(java.lang.String)"] {
		t.Fatalf("overloads=%#v", overloads.Symbols)
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

func hasDiagnostic(diagnostics []Diagnostic, text string) bool {
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.Message, text) {
			return true
		}
	}
	return false
}

func hasSignature(symbols []Symbol, name, signature string) bool {
	for _, symbol := range symbols {
		if symbol.Name == name && symbol.Signature == signature {
			return true
		}
	}
	return false
}

func hasContract(contracts []Contract, kind, role, service, key, broker string) bool {
	for _, contract := range contracts {
		if contract.Kind == kind && contract.Role == role && contract.Service == service && contract.Key == key && contract.Broker == broker {
			return true
		}
	}
	return false
}
