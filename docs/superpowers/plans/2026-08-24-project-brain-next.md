# Project Brain Next Version Implementation Plan
> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
**Goal:** Build the local requirement-to-report-to-feedback loop and deterministic microservice relation evidence.
**Architecture:** Keep the existing Go CLI, stdio MCP server and SQLite database. Add local input facts, persisted reports/feedback/rules, then route analyses through the report writer after extending the static extractor.
**Tech Stack:** Go 1.25, standard library (`archive/zip`, `encoding/json`, `os/exec`), SQLite via `modernc.org/sqlite`, MCP JSON-RPC.
**Spec:** `docs/superpowers/specs/2026-08-24-project-brain-next-design.md`
## Global Constraints
- UTF-8 and local-only data; no source, document or telemetry upload.
- Never checkout, fetch, commit, build, test or write a business repository.
- Preserve existing CLI and MCP contracts; only local feedback is a non-read-only MCP operation.
- Every edge has source location and `certain`, `probable` or `unresolved` confidence.
- Schema 8 adds data without deleting existing index data.
- Release targets are Windows amd64, Linux amd64, Darwin amd64 and Darwin arm64.
---
### Task 1: Schema 8 report, feedback and rule storage
**Files:** Modify `internal/storage/db.go`; create `internal/storage/report.go`; create `internal/storage/report_test.go`.
**Interfaces:** `type ReportRecord struct { ID, CreatedAt, BaselineSnapshot, InputDigest, JSON string }`; `type Feedback struct { ID, ReportID, SubjectKind, SubjectKey, Decision, Note, CreatedAt string }`; `type Rule struct { ID, Kind, Pattern, Target, Confidence, Note, CreatedAt string }`; `SaveReport`, `Report`, `RecordFeedback`, `Rules` methods on `*storage.DB`.
- [ ] **Step 1: Write the failing test.**
```go
func TestReportAndFeedbackRemainLocal(t *testing.T) {
 db, _ := Open(t.TempDir()); defer db.Close()
 record := ReportRecord{ID:"r1", CreatedAt:"2026-08-24T00:00:00Z", BaselineSnapshot:"[]", InputDigest:"sha256", JSON:`{"id":"r1"}`}
 if err := db.SaveReport(record); err != nil { t.Fatal(err) }
 if err := db.RecordFeedback(Feedback{ID:"f1", ReportID:"r1", SubjectKind:"repository", SubjectKey:"student", Decision:"rule", Note:"confirmed", CreatedAt:record.CreatedAt}); err != nil { t.Fatal(err) }
 got, err := db.Report("r1"); if err != nil || got.InputDigest != "sha256" { t.Fatalf("got=%#v err=%v", got, err) }
 rules, err := db.Rules(); if err != nil || len(rules) != 1 || rules[0].Pattern != "student" { t.Fatalf("rules=%#v err=%v", rules, err) }
}
```
- [ ] **Step 2: Verify RED.** Run `go test ./internal/storage -run TestReportAndFeedbackRemainLocal -count=1`; expect undefined storage types/methods.
- [ ] **Step 3: Implement minimal storage.** Change `PRAGMA user_version` to 8; for schema 7 create `reports`, `feedback`, `rules` without dropping index tables. `RecordFeedback` only inserts/replaces a rule for `Decision == "rule"`.
- [ ] **Step 4: Verify GREEN.** Run `go test ./internal/storage -count=1`; expect all storage tests pass.
- [ ] **Step 5: Commit.** Run `git add internal/storage/db.go internal/storage/report.go internal/storage/report_test.go; git commit -m "feat: persist local analysis reports and feedback"`.
### Task 2: Local document and prototype facts
**Files:** Create `internal/input/input.go`; create `internal/input/input_test.go`.
**Interfaces:** `type Fact struct { Kind, Value, SourcePath string; Line int; Confidence string }`; `type Result struct { Facts []Fact; Diagnostics []extract.Diagnostic }`; `Parse(text string, paths []string, sourceRef string) (Result, error)`.
- [ ] **Step 1: Write failing parser tests.**
```go
func TestParseExtractsFactsFromHTMLAndJSON(t *testing.T) {
 dir := t.TempDir(); html, jsonFile := filepath.Join(dir,"prototype.html"), filepath.Join(dir,"figma.json")
 os.WriteFile(html, []byte(`<h1>报名</h1><input name="studentId"><a href="/api/entry">提交</a>`), 0o644)
 os.WriteFile(jsonFile, []byte(`{"name":"Entry","route":"/api/entry","studentId":""}`), 0o644)
 result, err := Parse("调整报名", []string{html,jsonFile}, "figma://entry")
 if err != nil || !hasFact(result.Facts,"api_path","/api/entry") || !hasFact(result.Facts,"field","studentId") { t.Fatalf("result=%#v err=%v",result,err) }
}
func TestParseImageWithoutOCRReturnsDiagnostic(t *testing.T) {
 image := filepath.Join(t.TempDir(),"prototype.png"); os.WriteFile(image, []byte("image"), 0o644)
 result, err := Parse("",[]string{image},""); if err != nil || !hasDiagnostic(result.Diagnostics,"OCR") { t.Fatalf("result=%#v err=%v",result,err) }
}
```
- [ ] **Step 2: Verify RED.** Run `go test ./internal/input -count=1`; expect package missing.
- [ ] **Step 3: Implement local parsers.** Use `archive/zip` for DOCX WordprocessingML, tag stripping for HTML, recursive JSON strings, and optional `exec.LookPath("tesseract")` OCR. Reject directories, missing paths and files over 20 MiB. Missing OCR emits an unresolved diagnostic; source references are never fetched.
- [ ] **Step 4: Verify GREEN.** Run `go test ./internal/input -count=1`; expect the HTML/JSON facts and no-OCR diagnostic.
- [ ] **Step 5: Commit.** Run `git add internal/input; git commit -m "feat: parse local requirement and prototype facts"`.
### Task 3: Extended deterministic microservice evidence
**Files:** Modify `internal/extract/extract.go`, `internal/extract/extract_test.go`, `internal/indexer/index.go`.
**Interfaces:** `extract.File` emits `dubbo_reference`, `dubbo_service`, `publishes_topic`, `consumes_topic`, `defines_config`, `uses_config`, `scheduled_job`, `migration_table`, `api_contract`, `calls_http` edges and unresolved diagnostics.
- [ ] **Step 1: Write failing extractor tests.**
```go
func TestFileExtractsMicroserviceRelationships(t *testing.T) {
 result := File("EntryService.java", []byte(`@DubboReference private EntryApi entryApi; @KafkaListener(topics = "entry.created") void consume() {} void send() { kafkaTemplate.send("entry.created", "x"); } @Scheduled(cron = "0 * * * * *") void sync() {}`))
 for _, want := range []struct{kind,target string}{{"dubbo_reference","dubbo:EntryApi"},{"consumes_topic","mq:entry.created"},{"publishes_topic","mq:entry.created"},{"scheduled_job","schedule:EntryService#sync"}} { if !hasEdge(result.Edges,want.kind,want.target,Certain) { t.Fatalf("missing=%#v edges=%#v",want,result.Edges) } }
}
func TestFileMarksDynamicBehaviorUnresolved(t *testing.T) {
 result := File("Mapper.xml", []byte(`<mapper namespace="X"><select id="x">select * from ${table}<if test="x"> where id=1</if></select></mapper>`)); if !hasDiagnostic(result.Diagnostics,"动态 SQL") { t.Fatalf("diagnostics=%#v",result.Diagnostics) }
}
```
- [ ] **Step 2: Verify RED.** Run `go test ./internal/extract -run 'TestFileExtractsMicroserviceRelationships|TestFileMarksDynamicBehaviorUnresolved' -count=1`; expect missing edges/diagnostics.
- [ ] **Step 3: Implement parsers.** Recognize Java Dubbo, Kafka/RocketMQ/Rabbit annotations and literal send APIs, `@Value`, `@ConfigurationProperties`, `@Scheduled`; parse YAML/properties config keys, SQL migration table mutations, OpenAPI paths and TS/JS/Vue HTTP literals. Index `.ts`, `.tsx`, `.js`, `.vue`, `.html`. Emit unresolved diagnostics for dynamic MyBatis, route placeholders, reflection/proxy, concatenated SQL and unknown dynamic topics/config.
- [ ] **Step 4: Verify GREEN.** Run `go test ./internal/extract ./internal/indexer -count=1`; expect legacy and new parser tests pass.
- [ ] **Step 5: Commit.** Run `git add internal/extract internal/indexer/index.go; git commit -m "feat: index microservice contracts and uncertainty"`.
### Task 4: Unified persisted analysis report and local rule application
**Files:** Create `internal/report/report.go`, `internal/report/report_test.go`; modify `internal/requirement/analyze.go`, `internal/requirement/analyze_test.go`.
**Interfaces:** `type AnalysisReport` includes `ID`, `CreatedAt`, `BaselineSnapshot`, `InputFacts`, `Modules`, `RecommendedBranches`, `TestPoints`, `Feedback` plus existing analysis fields. `AnalyzeRequirement(db *storage.DB, source input.Result, repositories []workspace.Repository) (AnalysisReport,error)` persists the report.
- [ ] **Step 1: Write failing end-to-end report test.**
```go
func TestAnalysisReportPersistsAndAppliesRule(t *testing.T) {
 db := openIndexedDB(t); source := input.Result{Facts:[]input.Fact{{Kind:"business_term",Value:"entry"}}}
 first, err := AnalyzeRequirement(db,source,nil); if err != nil || first.ID == "" || len(first.TestPoints) == 0 { t.Fatalf("report=%#v err=%v",first,err) }
 db.RecordFeedback(storage.Feedback{ID:"f1",ReportID:first.ID,SubjectKind:"repository",SubjectKey:"entry-service",Decision:"rule",Note:"confirmed",CreatedAt:first.CreatedAt})
 second, err := AnalyzeRequirement(db,source,nil); if err != nil || len(second.Feedback)==0 { t.Fatalf("report=%#v err=%v",second,err) }
}
```
- [ ] **Step 2: Verify RED.** Run `go test ./internal/report -run TestAnalysisReportPersistsAndAppliesRule -count=1`; expect package/API missing.
- [ ] **Step 3: Implement report assembly.** Generate IDs with existing `github.com/google/uuid`; derive baseline snapshot, modules, branch suggestions `feature/<report-id>` without Git mutation, test points from Controller/Service/Mapper/API evidence, and risks/questions from unresolved diagnostics. Apply exact local rules before sorting candidates, serialize and save the final report.
- [ ] **Step 4: Keep compatibility.** Make `requirement.Analyze(db,text)` construct text facts and map the unified report back to its current result fields.
- [ ] **Step 5: Verify GREEN.** Run `go test ./internal/report ./internal/requirement -count=1`; expect report and all 1.0 analysis tests pass.
- [ ] **Step 6: Commit.** Run `git add internal/report internal/requirement; git commit -m "feat: generate auditable analysis reports"`.
### Task 5: MCP and CLI report loop
**Files:** Modify `internal/mcp/server.go`, `internal/mcp/server_test.go`, `internal/app/app.go`, `internal/app/app_test.go`, `cmd/project-brain/main.go`.
**Interfaces:** MCP tools `analyze_inputs`, `get_analysis_report`, `record_analysis_feedback`; CLI commands `analyze`, `report`, `feedback`.
- [ ] **Step 1: Write failing MCP contract test.**
```go
func TestToolsExposeReportLoopContracts(t *testing.T) {
 listed := tools(); if !toolHas(listed,"analyze_inputs",true) || !toolHas(listed,"get_analysis_report",true) || !toolHas(listed,"record_analysis_feedback",false) { t.Fatalf("tools=%#v",listed) }
}
```
- [ ] **Step 2: Verify RED.** Run `go test ./internal/mcp -run TestToolsExposeReportLoopContracts -count=1`; expect absent tools.
- [ ] **Step 3: Implement schemas and dispatch.** `analyze_inputs` takes `{text?:string,paths?:[]string,source_ref?:string}`; report lookup takes `{id:string}`; feedback requires `{report_id,subject_kind,subject_key,decision,note}` and has `readOnlyHint:false`. Add equivalent CLI commands without business-repository write calls.
- [ ] **Step 4: Verify GREEN.** Run `go test ./internal/mcp ./internal/app -count=1`; expect old initialization and new report-loop contracts pass.
- [ ] **Step 5: Commit.** Run `git add internal/mcp internal/app cmd/project-brain; git commit -m "feat: expose analysis report loop through mcp"`.
### Task 6: Real read-only acceptance and release
**Files:** Modify `README.md`, `README.en.md`, `codex-plugin/project-brain-codex/skills/project-brain-requirement-analysis/SKILL.md`, `scripts/build-release.ps1`, `scripts/test-release-layout.ps1`; create `docs/acceptance/project-brain-next.md`.
**Interfaces:** Versioned release build emits four binaries and checksums; acceptance records report ID, local input digest, feedback decision, index state and unchanged business-repository Git status.
- [ ] **Step 1: Write failing workflow assertions.** Add tests requiring `analyze_inputs`, report lookup and feedback examples in both README files and the Codex skill; run them before documentation changes.
- [ ] **Step 2: Document exact workflow.** Add Chinese/English examples for local path analysis, report lookup and feedback. Update the Codex skill to call `analyze_inputs` when local attachments/prototypes exist, then request code-plan confirmation.
- [ ] **Step 3: Create real acceptance evidence.** Run against `E:\ExampleWorkspace` using a local requirement document and benign HTML/JSON sample; record before/after `git status --short` for every business repository. Do not copy business source or attachments into this repository.
- [ ] **Step 4: Verify release gate.** Run `go test ./...`, `go vet ./...`, `.\scripts\build-release.ps1 -Version 1.1.0`, `.\scripts\test-release-layout.ps1 -Version 1.1.0`, and `git diff --check`; expect all checks pass.
- [ ] **Step 5: Commit.** Run `git add README.md README.en.md codex-plugin/project-brain-codex docs/acceptance scripts; git commit -m "docs: document next version analysis workflow"`.
## Plan Self-Review
- Spec coverage: Tasks 1/4 implement reports, feedback, rules, branches/modules/test points; Task 2 implements documents/prototypes; Task 3 implements every named relation and uncertainty; Task 5 makes the loop usable; Task 6 provides real read-only acceptance and release evidence.
- Placeholder scan: every task has a named file scope, RED check, GREEN command and commit boundary.
- Type consistency: `input.Result` enters `report.AnalyzeRequirement`; only `storage.ReportRecord`, `storage.Feedback` and `storage.Rule` persist loop state; CLI/MCP pass the `AnalysisReport.ID` unchanged.
