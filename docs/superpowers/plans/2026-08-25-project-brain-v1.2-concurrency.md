# Project Brain v1.2 并发一致性改造 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让同一工作区的多个 MCP/CLI 会话经由唯一协调者读写不可变索引快照，消除 SQLite 写锁竞争并返回可追溯的数据版本。

**Architecture:** `control.sqlite` 只保存协调状态、报告、反馈和规则版本；每份完成的源码索引成为独立、永不再写的 `snapshot-<id>.sqlite`。MCP 和 CLI 经由本机回环 TCP 的 JSON 行协议调用按工作区启动的协调者；协调者合并刷新请求、构建 staging 快照并在短事务中切换活跃快照。

**Tech Stack:** Go 1.25、标准库 `net`/`os`/`encoding/json`、现有 `modernc.org/sqlite`、SQLite WAL。

**Spec:** `docs/superpowers/specs/2026-08-25-project-brain-v1.2-concurrency-design.md`

## Global Constraints

- 所有数据、协调端点和快照只保存在用户本机；不得写入、fetch、checkout、构建或测试业务仓库。
- 不新增第三方依赖，继续使用默认远程主分支基线和现有静态关系提取规则。
- 默认 `freshness=stable` 立即读完整活跃快照；`latest` 等待合并刷新，超时仍返回完整旧快照与状态。
- 已激活快照不可修改；只有 `control.sqlite` 的短事务可切换 `active_snapshot_id`。
- 历史报告固定 `snapshot_id`、快照完成时间、仓库基线和 `rule_revision`；历史 JSON 不被反馈改写。
- Windows、macOS、Linux 均使用本机回环地址和用户数据目录；源文件与文档使用 UTF-8。

---

## 文件结构

- `internal/storage/db.go`：指定快照的只读/构建连接，保持 v1.1 数据库兼容。
- `internal/storage/control.go`：控制库的快照元数据、租约、刷新状态、报告、反馈和规则修订号。
- `internal/storage/snapshot.go`：快照命名、复制、磁盘检查、原子激活与回收。
- `internal/coordinator/protocol.go`、`server.go`、`client.go`：本机协议、唯一协调者、刷新合并、崩溃恢复和客户端启动握手。
- `internal/service/service.go`：在固定快照和规则版本内执行全部现有工具操作。
- `internal/mcp/server.go`、`internal/app/app.go`、`cmd/project-brain/main.go`：保留工具名/命令，改为协调者客户端并增加 `coordinator` 子命令。
- `internal/report/report.go`：报告写入控制库并固定快照和规则版本。
- `internal/*/*_test.go`：迁移、切换、并发、恢复和对外契约测试。
- `scripts/verify-v1.2.ps1`、`README.md`、`README.zh-CN.md`、`docs/architecture.md`：真实工作区只读验收及中英文使用说明。

## Task 1: 控制库与 v1.1 数据迁移

**Files:**
- Create: `internal/storage/control.go`
- Create: `internal/storage/control_test.go`
- Modify: `internal/storage/db.go`
- Modify: `internal/storage/report.go`
- Modify: `internal/storage/db_test.go`

**Interfaces:**
- Produces: `OpenControl(workspaceDir string) (*Control, error)`、`(*Control).ActiveSnapshot() (Snapshot, error)`、`(*Control).ActivateSnapshot(Snapshot) error`、`(*Control).RuleRevision() (int64, error)`、`(*Control).RecordFeedback(Feedback) (int64, error)`。
- Produces: `OpenSnapshot(path string, writable bool) (*DB, error)`；查询以 `writable=false` 打开，构建以 `writable=true` 打开。

- [ ] **Step 1: 写出迁移失败测试**

```go
func TestOpenControlMigratesV11ReportsRulesAndIndex(t *testing.T) {
    directory := t.TempDir()
    legacy, err := Open(directory)
    if err != nil { t.Fatal(err) }
    if err := legacy.SaveReport(Report{ID: "r1", BaselineSnapshot: "repo@a", InputDigest: "d", JSON: `{"id":"r1"}`}); err != nil { t.Fatal(err) }
    if _, err := legacy.Exec(`INSERT INTO rules(kind,pattern,target,confidence,note,created_at) VALUES('repository','service','confirmed','certain','ok','2026-01-01T00:00:00Z')`); err != nil { t.Fatal(err) }
    if err := legacy.Close(); err != nil { t.Fatal(err) }
    control, err := OpenControl(directory)
    if err != nil { t.Fatal(err) }
    t.Cleanup(func() { _ = control.Close() })
    snapshot, err := control.ActiveSnapshot()
    if err != nil || snapshot.ID != "snapshot-001" { t.Fatalf("snapshot=%#v err=%v", snapshot, err) }
    if _, err := control.Report("r1"); err != nil { t.Fatal(err) }
    rules, revision, err := control.Rules("repository")
    if err != nil || len(rules) != 1 || revision != 1 { t.Fatalf("rules=%#v revision=%d err=%v", rules, revision, err) }
}
```

- [ ] **Step 2: 运行失败测试**

Run: `go test ./internal/storage -run TestOpenControlMigratesV11ReportsRulesAndIndex -count=1`

Expected: FAIL，提示 `OpenControl`、`ActiveSnapshot` 或 `Rules` 未定义。

- [ ] **Step 3: 实现最小控制库与连接边界**

```go
type Snapshot struct { ID, Path, Completed, Baseline string }

func OpenControl(workspaceDir string) (*Control, error) {
    db, err := openSQLite(filepath.Join(workspaceDir, "control.sqlite"), true)
    if err != nil { return nil, err }
    control := &Control{DB: db, workspaceDir: workspaceDir}
    if err := control.initializeAndMigrate(); err != nil { _ = db.Close(); return nil, err }
    return control, nil
}

func OpenSnapshot(path string, writable bool) (*DB, error) { return openSQLite(path, writable) }
```

`initializeAndMigrate` 必须在一个控制库事务内：尚无快照时将同盘 `index.sqlite` 重命名为 `snapshot-001.sqlite`，插入 completed 快照，复制旧 `reports`/`feedback`/`rules`，写 migration 标记。`openSQLite` 对写连接执行 `PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;`；只读快照使用 `mode=ro` URI 且不执行 DDL。重复打开不得重复复制。

- [ ] **Step 4: 写出只读连接测试**

```go
func TestOpenSnapshotReadOnlyRejectsWrite(t *testing.T) {
    path := filepath.Join(t.TempDir(), "snapshot-001.sqlite")
    writable, _ := OpenSnapshot(path, true)
    if _, err := writable.Exec(`CREATE TABLE marker(value TEXT)`); err != nil { t.Fatal(err) }
    _ = writable.Close()
    readOnly, err := OpenSnapshot(path, false)
    if err != nil { t.Fatal(err) }
    t.Cleanup(func() { _ = readOnly.Close() })
    if _, err := readOnly.Exec(`INSERT INTO marker(value) VALUES('no')`); err == nil { t.Fatal("read-only snapshot accepted a write") }
}
```

- [ ] **Step 5: 运行测试**

Run: `go test ./internal/storage -count=1`

Expected: PASS；重复迁移后只有一份 `snapshot-001`、报告和规则。

- [ ] **Step 6: 提交**

Run: `git add internal/storage/db.go internal/storage/report.go internal/storage/control.go internal/storage/control_test.go internal/storage/db_test.go && git commit -m "feat: add control store and v1 index migration"`

Expected: 一个仅包含控制库和迁移的提交。

## Task 2: 不可变快照构建、切换和回收

**Files:**
- Create: `internal/storage/snapshot.go`
- Create: `internal/storage/snapshot_test.go`
- Modify: `internal/indexer/index.go`
- Modify: `internal/indexer/index_test.go`

**Interfaces:**
- Consumes: `storage.Control`、`storage.Snapshot`、`storage.OpenSnapshot`。
- Produces: `PrepareStaging(control *Control) (Snapshot, func(), error)`、`ActivateStaging(control *Control, staging Snapshot) (Snapshot, error)`、`CleanupSnapshots(control *Control) error`。

- [ ] **Step 1: 写出原子切换失败测试**

```go
func TestActivateStagingKeepsOldReaderOnOldSnapshot(t *testing.T) {
    control, active := controlWithSnapshot(t, "class Before {}")
    oldReader, _ := OpenSnapshot(active.Path, false)
    t.Cleanup(func() { _ = oldReader.Close() })
    staging, discard, err := PrepareStaging(control)
    if err != nil { t.Fatal(err) }
    defer discard()
    writer, _ := OpenSnapshot(staging.Path, true)
    if _, err := writer.Exec(`UPDATE files SET content='class After {}'`); err != nil { t.Fatal(err) }
    _ = writer.Close()
    if _, err := ActivateStaging(control, staging); err != nil { t.Fatal(err) }
    var oldContent string
    if err := oldReader.QueryRow(`SELECT content FROM files`).Scan(&oldContent); err != nil || oldContent != "class Before {}" { t.Fatalf("old=%q err=%v", oldContent, err) }
    current, _ := control.ActiveSnapshot()
    newReader, _ := OpenSnapshot(current.Path, false)
    defer newReader.Close()
    var newContent string
    if err := newReader.QueryRow(`SELECT content FROM files`).Scan(&newContent); err != nil || newContent != "class After {}" { t.Fatalf("new=%q err=%v", newContent, err) }
}
```

- [ ] **Step 2: 运行失败测试**

Run: `go test ./internal/storage -run TestActivateStagingKeepsOldReaderOnOldSnapshot -count=1`

Expected: FAIL，提示 `PrepareStaging` 或 `ActivateStaging` 未定义。

- [ ] **Step 3: 实现快照文件操作**

```go
func PrepareStaging(control *Control) (Snapshot, func(), error) {
    active, err := control.ActiveSnapshot()
    if err != nil { return Snapshot{}, nil, err }
    if err := requireFreeSpace(control.workspaceDir, active.Path); err != nil { return Snapshot{}, nil, err }
    staging := control.nextStagingSnapshot()
    if err := copyFile(active.Path, staging.Path); err != nil { return Snapshot{}, nil, err }
    return staging, func() { _ = os.Remove(staging.Path) }, nil
}

func ActivateStaging(control *Control, staging Snapshot) (Snapshot, error) {
    if err := validateSnapshot(staging.Path); err != nil { return Snapshot{}, err }
    staging.Completed = time.Now().UTC().Format(time.RFC3339)
    if err := control.ActivateSnapshot(staging); err != nil { return Snapshot{}, err }
    _ = CleanupSnapshots(control)
    return staging, nil
}
```

`copyFile` 采用 `io.Copy` 到 `.staging` 后 `os.Rename`，不覆盖活跃文件；`requireFreeSpace` 在平台小型实现中检查 `2*activeSize + 128MiB`，错误含所需与可用字节；`CleanupSnapshots` 保留活跃和前一完成快照，Windows 因读者而删除失败时下次切换重试。

- [ ] **Step 4: 写出 indexer staging 兼容测试**

```go
func TestRefreshUpdatesOnlyWritableStagingSnapshot(t *testing.T) {
    root := baselineRepository(t)
    control, active := indexedControlSnapshot(t, root)
    staging, discard, err := storage.PrepareStaging(control)
    if err != nil { t.Fatal(err) }
    defer discard()
    writable, _ := storage.OpenSnapshot(staging.Path, true)
    if _, err := Refresh(root, writable); err != nil { t.Fatal(err) }
    _ = writable.Close()
    if _, err := storage.ActivateStaging(control, staging); err != nil { t.Fatal(err) }
    original, _ := storage.OpenSnapshot(active.Path, false)
    defer original.Close()
    if paths, _ := original.SearchFiles("NewBaselineType"); len(paths) != 0 { t.Fatalf("active snapshot mutated: %#v", paths) }
}
```

- [ ] **Step 5: 运行测试**

Run: `go test ./internal/storage ./internal/indexer -count=1`

Expected: PASS；现有测试仍证明只索引 Git 基线，新增测试证明旧读者不会看到 staging 改动。

- [ ] **Step 6: 提交**

Run: `git add internal/storage/snapshot.go internal/storage/snapshot_test.go internal/indexer/index.go internal/indexer/index_test.go && git commit -m "feat: build and activate immutable index snapshots"`

Expected: 一个仅包含快照构建、激活和回收的提交。

## Task 3: 本机协调者、租约和 JSON 行协议

**Files:**
- Create: `internal/coordinator/protocol.go`
- Create: `internal/coordinator/server.go`
- Create: `internal/coordinator/client.go`
- Create: `internal/coordinator/coordinator_test.go`
- Modify: `cmd/project-brain/main.go`

**Interfaces:**
- Consumes: `storage.OpenControl`、`storage.PrepareStaging`、`storage.ActivateStaging`、`indexer.Refresh`。
- Produces: `Serve(root string) error`、`Ensure(root string, executable string) (*Client, error)`、`(*Client).Call(ctx context.Context, request Request) (Response, error)`。
- Produces: `Request{Operation string; Arguments map[string]any; Freshness string; SnapshotID string}` 与 `Response{Result json.RawMessage; Meta Metadata; Error string}`。

- [ ] **Step 1: 写出“并发启动只有一个服务”的失败测试**

```go
func TestEnsureCoalescesThreeLatestRequests(t *testing.T) {
    root := baselineRepository(t)
    executable := testCoordinatorProcess(t)
    clients := make([]*Client, 3)
    var group sync.WaitGroup
    errors := make(chan error, 3)
    for i := range clients {
        group.Add(1); go func(i int) {
            defer group.Done()
            client, err := Ensure(root, executable)
            if err == nil { clients[i] = client }
            errors <- err
        }(i)
    }
    group.Wait(); close(errors)
    for err := range errors { if err != nil { t.Fatal(err) } }
    results := make(chan Response, 3)
    for _, client := range clients { go func(c *Client) { got, _ := c.Call(context.Background(), Request{Operation: "status", Freshness: "latest"}); results <- got }(client) }
    for range clients { if got := <-results; got.Meta.RefreshTaskCount != 1 { t.Fatalf("meta=%#v", got.Meta) } }
}
```

测试子进程运行当前测试二进制的 `-test.run=TestCoordinatorHelperProcess`，helper 调用 `Serve(os.Getenv("PB_TEST_ROOT"))`；不得为测试引入 `errgroup` 或新依赖。

- [ ] **Step 2: 运行失败测试**

Run: `go test ./internal/coordinator -run TestEnsureCoalescesThreeLatestRequests -count=1`

Expected: FAIL，提示 package 或 `Ensure` 未定义。

- [ ] **Step 3: 实现协议、唯一租约和启动握手**

```go
type Metadata struct {
    SnapshotID       string `json:"snapshot_id"`
    RuleRevision     int64  `json:"rule_revision"`
    Freshness        string `json:"freshness"`
    ActiveBaseline   string `json:"active_baseline"`
    RefreshTarget    string `json:"refresh_target,omitempty"`
    RefreshTaskCount int    `json:"refresh_task_count"`
}

func Ensure(root, executable string) (*Client, error) {
    if client, err := connectHealthy(root); err == nil { return client, nil }
    acquired, err := acquireLease(root)
    if err != nil { return nil, err }
    if acquired { startCoordinator(executable, root) }
    return waitForHealthy(root, 5*time.Second)
}
```

协调者在 `127.0.0.1:0` 监听，`control.sqlite` 的 `coordinator_state` 保存 endpoint、随机 token、PID、协议版本和 heartbeat。客户端第一条 JSON 请求携带 token；服务端拒绝非 loopback 或 token 不匹配请求。工作区使用 `coordinator.lock` 的 `O_CREATE|O_EXCL` 短租约；服务端每秒刷新 heartbeat，空闲两分钟退出并释放锁。`Ensure` 仅在连接失败且 heartbeat 超过 15 秒时，将旧锁原子 rename 为 PID 诊断文件后重试；不得删除健康进程锁。`cmd` 增加 `project-brain coordinator <workspace>`，启动时只写 stderr。

- [ ] **Step 4: 写出 stable/latest 的失败测试**

```go
func TestStableReturnsActiveWhileLatestWaitsForOneRefresh(t *testing.T) {
    client, advance := runningCoordinatorWithBlockedRefresh(t)
    stable, err := client.Call(context.Background(), Request{Operation: "search", Arguments: map[string]any{"text": "Before"}, Freshness: "stable"})
    if err != nil || stable.Meta.Freshness != "refreshing" || stable.Meta.SnapshotID != "snapshot-001" { t.Fatalf("stable=%#v err=%v", stable, err) }
    latestDone := make(chan Response, 1)
    go func() { got, _ := client.Call(context.Background(), Request{Operation: "search", Arguments: map[string]any{"text": "After"}, Freshness: "latest"}); latestDone <- got }()
    select { case <-latestDone: t.Fatal("latest returned before refresh"); case <-time.After(50 * time.Millisecond): }
    advance()
    latest := <-latestDone
    if latest.Meta.SnapshotID == "snapshot-001" || latest.Meta.Freshness != "up_to_date" { t.Fatalf("latest=%#v", latest) }
}
```

- [ ] **Step 5: 实现刷新合并、staging 构建和崩溃恢复**

```go
func (server *Server) chooseSnapshot(ctx context.Context, request Request) (storage.Snapshot, Metadata, error) {
    if request.SnapshotID != "" { return server.control.Snapshot(request.SnapshotID) }
    stale, target, err := server.baselineChanged()
    if err != nil || !stale { return server.activeMetadata() }
    done := server.refresh(target)
    if request.Freshness != "latest" { return server.activeRefreshing(target) }
    select { case <-done: return server.activeMetadata(); case <-ctx.Done(): return server.activeRefreshing(target) }
}
```

`refresh(target)` 以一个 `refreshDone chan struct{}` 合并同目标请求；创建 staging、打开可写快照、调用 `indexer.Refresh`、关闭、校验和激活。失败时记录 `failed` 和错误并删除 staging，绝不修改活跃指针。`Serve` 启动时清理 `*.sqlite.staging`、把未完成刷新记为 `recovering`，始终以最后 active 快照响应。测试仅注入 `beforeActivate func()` 阻塞刷新，生产值为 nil。

- [ ] **Step 6: 运行测试和竞态检测**

Run: `go test -race ./internal/coordinator -count=1`

Expected: PASS；三个客户端只看到一个刷新任务，stable 不中断，latest 在切换后完成，重启后 staging 被清理而旧快照可搜索。

- [ ] **Step 7: 提交**

Run: `git add internal/coordinator cmd/project-brain/main.go && git commit -m "feat: coordinate local snapshot refreshes"`

Expected: 一个仅包含本机协调服务和启动握手的提交。

## Task 4: 在协调者内固定快照与规则版本执行工具

**Files:**
- Create: `internal/service/service.go`
- Create: `internal/service/service_test.go`
- Modify: `internal/report/report.go`
- Modify: `internal/requirement/analyze.go`
- Modify: `internal/change/analyze.go`
- Modify: `internal/storage/report.go`

**Interfaces:**
- Consumes: `coordinator.Request`、固定 `storage.Snapshot`、`storage.Control`。
- Produces: `service.Execute(root string, snapshot *storage.DB, control *storage.Control, request coordinator.Request, meta coordinator.Metadata) (any, error)`。
- Produces: `report.AnalyzeRequirement(snapshot *storage.DB, control *storage.Control, source input.Result, provenance report.Provenance) (AnalysisReport, error)`。
- Produces: `Provenance{SnapshotID string; SnapshotCompleted string; Baselines string; RuleRevision int64}`。

- [ ] **Step 1: 写出报告版本固定的失败测试**

```go
func TestAnalyzeRequirementPersistsChosenSnapshotAndRuleRevision(t *testing.T) {
    index := indexedSnapshot(t, "class AdmitService {}")
    control := controlWithRule(t, storage.Rule{Kind: "repository", Pattern: "student", Target: "confirmed"})
    result, err := AnalyzeRequirement(index, control, input.Result{Facts: []input.Fact{{Value: "招生"}}}, Provenance{SnapshotID: "snapshot-007", SnapshotCompleted: "2026-08-25T01:02:03Z", Baselines: "student@abc", RuleRevision: 4})
    if err != nil { t.Fatal(err) }
    if result.SnapshotID != "snapshot-007" || result.RuleRevision != 4 { t.Fatalf("report=%#v", result) }
    stored, err := control.Report(result.ID)
    if err != nil || !strings.Contains(stored.JSON, `"rule_revision":4`) { t.Fatalf("stored=%#v err=%v", stored, err) }
}
```

- [ ] **Step 2: 运行失败测试**

Run: `go test ./internal/report -run TestAnalyzeRequirementPersistsChosenSnapshotAndRuleRevision -count=1`

Expected: FAIL，提示 `Provenance`、`SnapshotID` 或新函数参数不存在。

- [ ] **Step 3: 实现服务分发和报告持久化**

```go
func Execute(root string, snapshot *storage.DB, control *storage.Control, request coordinator.Request, meta coordinator.Metadata) (any, error) {
    switch request.Operation {
    case "search", "evidence":
        return query.Search(snapshot, stringArgument(request.Arguments, "text"))
    case "report":
        return control.Report(stringArgument(request.Arguments, "id"))
    case "feedback":
        return control.RecordFeedback(feedbackFrom(request.Arguments))
    }
    return executeIndexedOperation(root, snapshot, control, request, meta)
}
```

`executeIndexedOperation` 明确分派 `status`、`trace`、`impact`、`change`、`requirement`、`analyze_inputs`：前五项只用传入的 `snapshot`，`analyze_inputs` 以 `meta` 创建报告。`AnalysisReport` 和 `storage.Report` 增加 `SnapshotID`、`SnapshotCompleted`、`RuleRevision`。`AnalyzeRequirement` 查询固定规则修订并向 `control.SaveReport` 写入。反馈在控制库单事务插入反馈、upsert 规则、`rule_revision = rule_revision + 1`，返回提交后修订号；读取报告只返回原 JSON。

- [ ] **Step 4: 写出反馈不污染在途分析的失败测试**

```go
func TestFeedbackDoesNotChangeInFlightReportRevision(t *testing.T) {
    service, pauseRules := serviceBlockedAfterRuleRead(t)
    done := make(chan report.AnalysisReport, 1)
    go func() { got, _ := service.Analyze(inputs("招生")); done <- got }()
    pauseRules()
    revision, err := service.Feedback(storage.Feedback{ID: "f1", ReportID: "r", SubjectKind: "repository", SubjectKey: "student", Decision: "confirmed"})
    if err != nil || revision != 2 { t.Fatalf("revision=%d err=%v", revision, err) }
    got := <-done
    if got.RuleRevision != 1 { t.Fatalf("in-flight report used revision %d", got.RuleRevision) }
}
```

- [ ] **Step 5: 运行测试并提交**

Run: `go test ./internal/service ./internal/report ./internal/requirement ./internal/change -count=1`

Expected: PASS；新报告包含快照和规则版本，反馈只影响后续报告。

Run: `git add internal/service internal/report/report.go internal/requirement/analyze.go internal/change/analyze.go internal/storage/report.go && git commit -m "feat: bind analyses to snapshot and rule revisions"`

Expected: 一个仅包含固定读视图和报告版本的提交。

## Task 5: MCP/CLI 迁移、公开契约和兼容性

**Files:**
- Modify: `internal/mcp/server.go`
- Modify: `internal/mcp/server_test.go`
- Modify: `internal/app/app.go`
- Modify: `internal/app/app_test.go`

**Interfaces:**
- Consumes: `coordinator.Ensure`、`coordinator.Client.Call`。
- Produces: 所有读 MCP 工具可选 `freshness`、`snapshot_id`；响应含 `snapshot_id`、`rule_revision`、`freshness`、`active_baseline`、`refresh_target`。
- Produces: `project-brain refresh/status/search/trace/impact/analyze/report/feedback` 保持可用，但都经由协调者协议。

- [ ] **Step 1: 写出 MCP schema 和响应元数据失败测试**

```go
func TestToolsAcceptSnapshotSelectionAndReturnMetadata(t *testing.T) {
    for _, tool := range tools() {
        if tool["name"] == "record_analysis_feedback" { continue }
        properties := tool["inputSchema"].(map[string]any)["properties"].(map[string]any)
        if _, ok := properties["freshness"]; !ok { t.Fatalf("%s missing freshness", tool["name"]) }
        if _, ok := properties["snapshot_id"]; !ok { t.Fatalf("%s missing snapshot_id", tool["name"]) }
    }
    text := workspaceStatus(t, createBaselineRepository(t))
    for _, key := range []string{"snapshot_id", "rule_revision", "freshness", "active_baseline"} {
        if !strings.Contains(text, `"`+key+`"`) { t.Fatalf("status missing %s: %s", key, text) }
    }
}
```

- [ ] **Step 2: 运行失败测试**

Run: `go test ./internal/mcp -run TestToolsAcceptSnapshotSelectionAndReturnMetadata -count=1`

Expected: FAIL，提示工具 schema 缺少 `freshness` 或 `snapshot_id`。

- [ ] **Step 3: 将 MCP 和 CLI 改为协调者客户端**

```go
func call(root, name string, arguments map[string]any) (any, error) {
    client, err := coordinator.Ensure(root, os.Args[0])
    if err != nil { return nil, err }
    response, err := client.Call(context.Background(), coordinator.Request{
        Operation: operationForTool(name), Arguments: arguments,
        Freshness: freshnessArgument(arguments), SnapshotID: stringArgument(arguments, "snapshot_id"),
    })
    if err != nil { return nil, err }
    return mergeResultAndMetadata(response)
}
```

`tools()` 给全部读工具 schema 合并 `freshness` 枚举和 `snapshot_id` 字符串；服务端将缺失 freshness 解释为 `stable`。反馈不接收快照参数且响应含 `rule_revision`。CLI 的 `index`、`refresh` 映射为 `latest` 刷新；`status/search/trace/impact/analyze` 保持参数顺序和 JSON 输出兼容。固定快照被回收时返回 `snapshot not available: <id>`，不得替换为 active。

- [ ] **Step 4: 写出 CLI 兼容性测试**

```go
func TestRunSearchUsesStableSnapshotByDefault(t *testing.T) {
    root := createBaselineRepository(t)
    t.Setenv("LOCALAPPDATA", t.TempDir())
    var output bytes.Buffer
    if err := Run([]string{"search", root, "Main"}, &output); err != nil { t.Fatal(err) }
    if !strings.Contains(output.String(), `"snapshot_id"`) || !strings.Contains(output.String(), "Main.java") { t.Fatalf("output=%s", output.String()) }
}
```

- [ ] **Step 5: 运行测试并提交**

Run: `go test ./internal/mcp ./internal/app -count=1 && go test ./... -count=1`

Expected: PASS；连续两个 `workspace_status` 仍通过，任意读工具可在刷新中返回完整旧快照。

Run: `git add internal/mcp/server.go internal/mcp/server_test.go internal/app/app.go internal/app/app_test.go && git commit -m "feat: route MCP and CLI through workspace coordinator"`

Expected: 一个仅包含入口迁移和公开契约的提交。

## Task 6: 崩溃恢复、磁盘限制与真实工作区验收

**Files:**
- Modify: `internal/coordinator/coordinator_test.go`
- Modify: `internal/storage/snapshot_test.go`
- Create: `scripts/verify-v1.2.ps1`
- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify: `docs/architecture.md`

**Interfaces:**
- Consumes: `project-brain coordinator`、既有 MCP 与 CLI 入口。
- Produces: 可重复运行的本机验证脚本，输出快照、刷新任务数量、耗时、磁盘大小和每个业务仓库 `git status --short` 的差异。

- [ ] **Step 1: 写出崩溃恢复失败测试**

```go
func TestRestartRemovesStagingAndServesLastActiveSnapshot(t *testing.T) {
    root, workspaceDir := preparedWorkspace(t)
    writeStagingMarker(t, workspaceDir, "snapshot-002.sqlite.staging")
    writeExpiredCoordinatorState(t, workspaceDir)
    client, err := Ensure(root, testCoordinatorProcess(t))
    if err != nil { t.Fatal(err) }
    response, err := client.Call(context.Background(), Request{Operation: "search", Arguments: map[string]any{"text": "Main"}, Freshness: "stable"})
    if err != nil || response.Meta.SnapshotID != "snapshot-001" { t.Fatalf("response=%#v err=%v", response, err) }
    if _, err := os.Stat(filepath.Join(workspaceDir, "snapshot-002.sqlite.staging")); !errors.Is(err, os.ErrNotExist) { t.Fatalf("staging err=%v", err) }
}
```

- [ ] **Step 2: 运行失败测试**

Run: `go test ./internal/coordinator -run TestRestartRemovesStagingAndServesLastActiveSnapshot -count=1`

Expected: FAIL，staging 文件仍存在或启动不能接管失联状态。

- [ ] **Step 3: 实现恢复和磁盘可观测性**

在 `Serve` 取得租约后调用 `storage.RemoveStaging(workspaceDir)`，只删除后缀严格为 `.sqlite.staging` 的当前工作区文件。`status` 元数据增加 `refresh_started_at`、`completed_repositories`、`refresh_error`、`snapshot_bytes`；磁盘不足错误必须包含所需字节和可用字节，且保持原 active 指针。`verify-v1.2.ps1` 只执行 `project-brain`、`Get-ChildItem`、`Measure-Command`、`git -C <repo> status --short` 和只读并发 MCP 请求，禁止业务仓库写操作。

- [ ] **Step 4: 写出磁盘边界失败测试**

```go
func TestPrepareStagingPreservesActiveSnapshotWhenSpaceIsInsufficient(t *testing.T) {
    control, active := controlWithSnapshot(t, "class Main {}")
    setFreeSpaceProbe(t, func(string) uint64 { return uint64(fileSize(t, active.Path))*2 + 128*1024*1024 - 1 })
    if _, _, err := PrepareStaging(control); err == nil || !strings.Contains(err.Error(), "insufficient disk") { t.Fatalf("err=%v", err) }
    got, err := control.ActiveSnapshot()
    if err != nil || got.ID != active.ID { t.Fatalf("active=%#v err=%v", got, err) }
}
```

- [ ] **Step 5: 运行全量自动化验证**

Run: `go test -race ./... -count=1`

Expected: PASS；无 data race，旧 v1.1 测试和 v1.2 并发/恢复/迁移测试全部通过。

- [ ] **Step 6: 在真实工作区执行只读验收**

Run: `powershell -ExecutionPolicy Bypass -File .\scripts\verify-v1.2.ps1 -Workspace E:\ExampleWorkspace -Executable .\dist\project-brain.exe`

Expected: 输出一次迁移或已存在快照、一次增量刷新、三个并发请求只对应一个 refresh task、快照大小/耗时、所有业务仓库前后 `git status --short` 无新增差异。没有默认分支变化时，脚本输出“未触发增量变化”，但仍验证三客户端合并和零写入。

- [ ] **Step 7: 更新中英文文档**

写清楚 `control.sqlite`、`snapshot-*.sqlite`、协调者状态的职责；说明默认 `stable` 与可选 `latest`；说明“`database is locked` 不再是正常路径，残留时执行 `project-brain status <workspace>` 触发恢复”；说明删除工作区数据目录会移除本地索引和报告。

- [ ] **Step 8: 提交**

Run: `git add internal/coordinator/coordinator_test.go internal/storage/snapshot_test.go scripts/verify-v1.2.ps1 README.md README.zh-CN.md docs/architecture.md && git commit -m "docs: document local snapshot concurrency workflow"`

Expected: 一个仅包含验收脚本、恢复补测和文档的提交。

## 实施后复核

- [ ] 将每个任务与设计文档的“快照与原子切换”“刷新合并与新鲜度”“反馈与报告一致性”“本地 IPC 与崩溃恢复”“对外契约”“验收”对应，确认无遗漏。
- [ ] 用 `rg` 扫描本计划中的未完成占位标记，预期无结果。
- [ ] 执行 `go vet ./...`、`go test -race ./... -count=1`；记录实际命令和输出摘要。
- [ ] 执行 `git diff --check` 与 `git status --short`；只提交 Project Brain 改动，不触及 `E:\ExampleWorkspace` 业务仓库文件。
