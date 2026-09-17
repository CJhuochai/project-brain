# Project Brain v1 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**目标：** 交付一个本地只读的 Go CLI/MCP 工具，用于自动发现工作区 Git 仓库、索引其远程默认基线，并为 Java 微服务提供带源码证据的搜索、链路追踪与影响分析。

**架构：** 单一 Go 二进制将 CLI、SQLite/FTS5 存储、只读 Git 发现、Java/Spring/MyBatis 文本结构提取及 stdio MCP Server 组合在一起。所有状态放在用户本地数据目录；业务工作区只读。第一版不使用网络、Docker、向量库或图数据库服务。

**技术栈：** Go 1.25、标准库命令解析/XML/JSON、modernc.org/sqlite、MCP stdio JSON-RPC。

**规格：** `docs/superpowers/specs/2026-08-24-project-brain-v1-design.md`

## 全局约束

- 文本文件 UTF-8；默认输出 JSON。
- 严禁向被索引 Git 仓库写文件、切分支、fetch、checkout、build 或测试。
- 默认基线只从本地 `refs/remotes/origin/HEAD` 推导；缺失时报告 `baseline_unknown`。
- 每条代码关系必须返回来源文件和行号；解析不确定性必须显式标为 `probable` 或 `unresolved`。
- 所有索引数据只写入用户数据目录或用户明确指定的本地数据目录。

---

### 任务 1：初始化 Go 模块与本地目录/配置边界

**文件：**
- 创建：`go.mod`
- 创建：`cmd/project-brain/main.go`
- 创建：`internal/app/app.go`
- 创建：`internal/storage/path.go`
- 创建：`internal/storage/path_test.go`
- 创建：`.gitignore`
- 创建：`README.md`

**接口：**
- 提供：`storage.DataDir() (string, error)` 与 `storage.WorkspaceDir(root string) (string, error)`。
- 提供：`app.New() *cobra.Command`。

- [ ] 写失败测试，断言同一规范化工作区路径产生同一数据目录，且 Windows `LOCALAPPDATA` 优先。
- [ ] 运行 `go test ./internal/storage`，确认因实现不存在而失败。
- [ ] 创建 Go 模块、目录计算实现及只包含 `version` 的 CLI。
- [ ] 添加 `.gitignore`，忽略本地开发索引、测试产物和二进制，不忽略源代码与 fixture。
- [ ] 再次运行 `go test ./internal/storage`，确认通过。
- [ ] 提交：`chore: initialize project brain CLI`。

### 任务 2：只读 Git 仓库发现与远程默认基线识别

**文件：**
- 创建：`internal/workspace/discover.go`
- 创建：`internal/workspace/git.go`
- 创建：`internal/workspace/types.go`
- 创建：`internal/workspace/discover_test.go`
- 修改：`internal/app/app.go`

**接口：**
- 提供：`workspace.Discover(root string) ([]workspace.Repository, error)`。
- `Repository` 包含 `Path`、`Name`、`RemoteURL`、`BaselineBranch`、`BaselineCommit`、`BaselineState`。
- 提供 CLI：`project-brain discover <workspace>`。

- [ ] 用临时 Git fixture 创建两个仓库、一个 `origin/HEAD` 基线和一个没有基线的仓库。
- [ ] 写失败测试，断言发现只读仓库、跳过 `.git` 子目录和 `target`，并把缺失基线标为 `baseline_unknown`。
- [ ] 使用 `git -C <repo> symbolic-ref --quiet refs/remotes/origin/HEAD` 和 `rev-parse` 实现基线读取；不调用 checkout、fetch 或任何写操作。
- [ ] 为 `discover` 命令添加 JSON 输出与非 JSON 简表输出。
- [ ] 运行 `go test ./internal/workspace`，并在 fixture 上比较扫描前后的 `git status --porcelain`。
- [ ] 提交：`feat: discover repositories without changing workspaces`。

### 任务 3：SQLite 模式与增量文件索引

**文件：**
- 创建：`internal/storage/db.go`
- 创建：`internal/storage/schema.sql`
- 创建：`internal/storage/index.go`
- 创建：`internal/storage/index_test.go`
- 修改：`internal/app/app.go`

**接口：**
- 提供：`storage.Open(workspaceDir string) (*storage.DB, error)`。
- 提供：`(*DB).UpsertFile(repoID, path string, content []byte) (changed bool, err error)`。
- 提供 CLI：`project-brain index <workspace>` 与 `project-brain status <workspace>`。

- [ ] 写失败测试，验证 schema 初始化、文件内容哈希去重、删除文件本地记录及 FTS 精确查询。
- [ ] 创建 SQLite schema：`repositories`、`files`、`symbols`、`edges`、`diagnostics` 及 `file_fts`。
- [ ] 实现 SHA-256 内容哈希和事务性 upsert；只读取支持后缀的文件。
- [ ] `index` 命令先调用 Discover，再扫描 Java/XML/YAML/Properties/SQL/Markdown/POM 文件；写入用户数据目录。
- [ ] `status` 命令返回仓库、基线、文件数、上次索引时间和诊断数。
- [ ] 运行 `go test ./internal/storage ./internal/workspace`。
- [ ] 提交：`feat: add local incremental file index`。

### 任务 4：Java、Spring、Maven 与 MyBatis 的证据提取

**文件：**
- 创建：`internal/extract/types.go`
- 创建：`internal/extract/java.go`
- 创建：`internal/extract/maven.go`
- 创建：`internal/extract/mybatis.go`
- 创建：`internal/extract/extract_test.go`
- 修改：`internal/storage/index.go`

**接口：**
- 提供：`extract.File(path string, content []byte) extract.Result`。
- `Result` 产生 `Symbol`、`Edge`、`Diagnostic`；每项都有文件、行、置信度。
- 关系类型至少为：`declares`、`imports`、`implements`、`route`、`uses_mapper`、`maps_statement`、`queries_table`、`depends_on_module`。

- [ ] 写失败 fixture：Spring Controller 调用 Service，Mapper 接口关联 XML，POM 依赖另一个模块。
- [ ] 实现确定性正则/词法提取：包、类型、方法、导入、Spring 注解和路径、Feign 名称、Mapper namespace/statement、SQL 表名、POM artifact/dependency。
- [ ] 标记正则无法可靠解析的内容为诊断，禁止从它生成确定关系。
- [ ] 将提取结果写入符号/关系表，并保留行号、置信度和来源文件。
- [ ] 运行 `go test ./internal/extract ./internal/storage`。
- [ ] 提交：`feat: extract Java service evidence`。

### 任务 5：搜索、追踪与影响分析 CLI

**文件：**
- 创建：`internal/query/types.go`
- 创建：`internal/query/search.go`
- 创建：`internal/query/trace.go`
- 创建：`internal/query/query_test.go`
- 修改：`internal/app/app.go`

**接口：**
- 提供：`query.Search(db, text string) ([]query.Evidence, error)`。
- 提供：`query.Trace(db, target string, maxDepth int) (query.TraceResult, error)`。
- 提供：`query.Impact(db, target string, maxDepth int) (query.ImpactResult, error)`。
- 提供 CLI：`search`、`trace`、`impact`。

- [ ] 写失败测试：FTS 命中路由/符号；唯一目标 trace 出 Controller→Service→Mapper；重名目标返回歧义；impact 返回调用方并分开 certain/probable。
- [ ] 实现带证据的 FTS 查询、目标消歧和使用递归 CTE 的有深度限制遍历。
- [ ] 所有结果统一返回 JSON，包含 `file`、`line_start`、`line_end`、`relation_type`、`confidence`。
- [ ] 对未索引、歧义或无基线仓库返回可读诊断和非零退出码。
- [ ] 运行 `go test ./internal/query ./...`。
- [ ] 提交：`feat: add evidence-backed search and impact analysis`。

### 任务 6：需求分析与标准 MCP Server

**文件：**
- 创建：`internal/requirement/analyze.go`
- 创建：`internal/requirement/analyze_test.go`
- 创建：`internal/mcp/server.go`
- 创建：`internal/mcp/server_test.go`
- 修改：`internal/app/app.go`
- 创建：`examples/codex-config.toml`

**接口：**
- 提供：`requirement.Analyze(db, text string) (requirement.Report, error)`。
- 提供：`mcp.Serve(stdin io.Reader, stdout io.Writer, workspace string) error`。
- MCP 工具：`workspace_status`、`find_business_context`、`trace_code_path`、`analyze_change_impact`、`analyze_requirement`、`get_evidence`。

- [ ] 写失败测试：需求词按来源证据返回仓库候选；JSON-RPC initialize/tools/list/tools/call 返回合法响应；未知工具返回 JSON-RPC 错误。
- [ ] 需求分析从文本提取标识符、路由、驼峰/下划线字段和普通词，调用 Search 并聚合仓库/模块候选；不把排序结果写成确定事实。
- [ ] 实现不依赖网络的 stdio JSON-RPC Server 和六个 MCP 工具。
- [ ] 添加 Codex `config.toml` 配置样例，不自动改用户配置。
- [ ] 运行 `go test ./internal/requirement ./internal/mcp ./...`。
- [ ] 提交：`feat: expose project brain through MCP`。

### 任务 7：发布验证、ExampleWorkspace 只读验收与文档

**文件：**
- 创建：`scripts/verify-readonly.ps1`
- 创建：`scripts/accept-exampleworkspace.ps1`
- 修改：`README.md`
- 修改：`docs/superpowers/specs/2026-08-24-project-brain-v1-design.md`

**接口：**
- `verify-readonly.ps1` 接收工作区路径，记录并比较每个仓库 `git status --porcelain`。
- `accept-exampleworkspace.ps1` 只调用 `discover`、`index`、`status`，不读取或输出任何源码正文。

- [ ] 写 PowerShell 自检：扫描前后仓库状态不变，否则返回失败。
- [ ] 运行 ExampleWorkspace 验收，只输出仓库数量、基线状态、文件计数和诊断摘要。
- [ ] 在 README 写明安装、使用、隐私、非目标、MCP 接入、删除本地数据和 GitHub 开源前的脱敏规则。
- [ ] 运行 `go test ./...`、`go vet ./...`、`go build ./cmd/project-brain` 和只读验收。
- [ ] 提交：`docs: document local-first project brain v1`。
