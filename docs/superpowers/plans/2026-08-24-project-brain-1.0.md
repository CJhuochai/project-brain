# Project Brain 1.0 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付可在真实微服务日常开发中使用的本地代码知识库 1.0。

**Architecture:** 保留单 Go 二进制、SQLite 和 stdio MCP；先让索引快照可删除、可验证，再将静态关系从类型级升级为方法级，并将底层证据聚合为固定分析报告。所有业务仓库访问继续只读。

**Tech Stack:** Go 1.25、SQLite FTS5、Git 本地对象、确定性 Java/XML 解析、MCP stdio JSON-RPC。

**Spec:** `docs/superpowers/specs/2026-08-24-project-brain-1.0-design.md`

## Global Constraints

- UTF-8；不上传数据，不写业务工作区，不隐式联网。
- 每项行为先写失败测试，观察失败后写最小实现。
- 每次大任务独立运行 `go test ./...`、`go vet ./...`、`go build ./cmd/project-brain`。

---

### Task 1: 基线快照删除与索引状态

**Files:** `internal/storage/*`、`internal/indexer/*`、`internal/mcp/*`

**Produces:** 删除归档中不存在的旧文件/证据；每仓库返回精确索引状态。

- [ ] 写 fixture：初次索引两个文件，更新远程默认基线后删除一个文件；断言旧文件、符号和边均不可查询。
- [ ] 在单个仓库索引事务中记录本次归档路径，并删除未出现路径的 files、symbols、edges、diagnostics、FTS 行。
- [ ] 为 `workspace_status` 增加每仓库状态与过期原因。
- [ ] 运行索引、MCP 和全量测试；提交 `fix: reconcile deleted baseline files`。

### Task 2: Java 方法与 MyBatis 精确关系

**Files:** `internal/extract/*`、`internal/storage/*`、`internal/query/*`

**Produces:** 方法符号、路由方法、调用、Mapper 方法到 XML statement 的证据边。

- [ ] 写 fixture：Controller 方法调用 Service 方法、Service 调用 Mapper 方法、Mapper XML statement 访问表。
- [ ] 解析方法声明、方法体调用、注解绑定和 Mapper 接口方法；歧义调用标记 probable。
- [ ] 将 XML namespace/statement 与 Mapper 方法 ID 精确匹配；保留 SQL 表边。
- [ ] 运行提取、查询和全量测试；提交 `feat: add method-level service evidence`。

### Task 3: 结构化需求与变更影响报告

**Files:** `internal/requirement/*`、`internal/change/*`、`internal/mcp/*`、`internal/app/*`

**Produces:** `AnalysisReport`、`analyze_change` MCP 工具和 CLI。

- [ ] 写 fixture：需求词命中跨仓库路径；本地两个 commit 的文件变更返回影响报告。
- [ ] 聚合仓库候选、入口、修改点、路径、风险和待确认项，所有条目带来源证据。
- [ ] 解析文件、commit 和 `base..target` 为本地 Git 差异；禁止缺失对象时猜测。
- [ ] 运行全量测试；提交 `feat: report requirement and change impact`。

### Task 4: 正式发布与真实验收

**Files:** `scripts/*`、`README.md`、`examples/*`、`docs/release/*`

**Produces:** Windows 发布包、校验和、安装/升级/卸载说明、三份真实验收报告。

- [ ] 构建 Windows 单文件与 SHA-256；验证配置样例不含真实路径、令牌或业务源码。
- [ ] 运行 ExampleWorkspace 只读验收，并保留汇总结果。
- [ ] 使用用户提供的 3 个真实需求，保存脱敏验收报告，逐项确认涉及项目、入口、影响和风险。
- [ ] 运行最终命令并提交 `release: project brain 1.0`。
