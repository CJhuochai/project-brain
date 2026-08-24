# Project Brain 下一版本设计说明

## 目标

将 Project Brain 从“即时静态分析工具”升级为本地优先的日常开发闭环：需求输入、跨微服务影响分析、可追溯报告、人工确认与本地知识回写。输出必须覆盖项目、基线分支、模块、文件、方法、测试点、风险、问题和来源证据。

## 固定约束

- 业务仓库始终只读：不 checkout、fetch、commit、构建、测试或写入任何业务文件。
- 文档、原型、报告、人工规则和索引都只保存在当前用户的 Project Brain 数据目录；不上传、不遥测、不访问 Figma 云端。
- 基线继续使用每个仓库本机已有的 `origin/HEAD`；报告记录分析时使用的基线分支与 commit。
- 自动关系必须包含仓库、文件、行号、解析器、置信度；无法静态证明时记录诊断，不能输出为确定关系。
- 人工回写只改变本地知识库，不改源码；每条规则和确认都记录创建时间、理由和关联报告。

## 1. 本地输入与结构化事实

新增 `input` 包，将每个输入转换为统一的 `Fact`：

```go
type Fact struct {
    Kind       string // business_term, api_path, field, operation, source_ref
    Value      string
    SourcePath string
    Line       int
    Confidence string // certain, probable, unresolved
}
```

支持以下本地输入：

| 输入 | 处理方式 | 输出事实 |
| --- | --- | --- |
| 文本、Markdown、HTML | UTF-8 文本与标签清理 | 业务词、标题、接口路径、字段名 |
| DOCX | 读取 ZIP 中的 WordprocessingML 文本 | 标题、段落、表格字段 |
| JSON、Figma 本地导出 | 递归读取字符串键和值 | 页面、组件、字段、操作、URL |
| 截图 | 调用本机已安装的 `tesseract`；无引擎时保留文件摘要并产生 `unresolved` 诊断 | OCR 文本及其字段候选 |

Figma URL 只可作为 `source_ref` 保存，不发起网络请求。截图 OCR 由可选本地可执行文件提供，缺失时不会阻断整份分析。

`analyze_requirement` 保持接收 `text` 的兼容契约。新增 `analyze_inputs` 接收 `text`、本地 `paths` 和可选 `source_ref`，先解析为事实，再进入需求分析。

## 2. 可追溯报告与人工回写

SQLite schema 从 7 升级到 8，保留索引表，新增：

```sql
CREATE TABLE reports (
  id TEXT PRIMARY KEY,
  created_at TEXT NOT NULL,
  baseline_snapshot TEXT NOT NULL,
  input_digest TEXT NOT NULL,
  report_json TEXT NOT NULL
);
CREATE TABLE feedback (
  id TEXT PRIMARY KEY,
  report_id TEXT NOT NULL,
  subject_kind TEXT NOT NULL,
  subject_key TEXT NOT NULL,
  decision TEXT NOT NULL,
  note TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE TABLE rules (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  pattern TEXT NOT NULL,
  target TEXT NOT NULL,
  confidence TEXT NOT NULL,
  note TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(kind, pattern, target)
);
```

报告 ID 使用 UUID；输入只保存 SHA-256 摘要、来源路径和提取后的必要事实，不复制原始附件。`baseline_snapshot` 保存参与仓库的名称、分支和 commit。

统一 `AnalysisReport` 在原有候选仓库、入口、修改点、影响、风险、问题基础上新增：

- `id`、`created_at`、`baseline_snapshot`、`input_facts`；
- `modules`：命中 Maven 模块或前端模块；
- `recommended_branches`：每个候选仓库的 `feature/<report-id>` 建议名，不创建分支；
- `test_points`：受影响 Controller/Service/Mapper/契约的测试建议；
- `feedback`：已应用的人工确认与规则。

新增 MCP 写工具 `record_analysis_feedback`，只写本地数据库。它必须接收 `report_id`、`subject_kind`、`subject_key`、`decision`（`confirmed`、`rejected`、`rule`）和 `note`；MCP 元数据明确标注非只读。新增只读工具 `get_analysis_report`。

反馈在下次分析前按以下顺序生效：确认规则提升匹配候选的置信度；拒绝规则抑制完全相同的候选；报告级确认仅作为历史证据，不跨报告强行覆盖新基线。

## 3. 微服务关系与不确定性

扩展 `extract.File`，所有新增边沿用现有 `Edge` 的来源行和置信度：

| 来源 | 关系类型 | 目标格式 | 置信度 |
| --- | --- | --- | --- |
| `@FeignClient`、Feign 调用 | `feign_client`、`calls_remote` | `feign:<service>`、`http:<method>:<path>` | certain/probable |
| `@DubboReference`、`@DubboService` | `dubbo_reference`、`dubbo_service` | `dubbo:<interface>` | certain |
| Kafka、RocketMQ、Rabbit 注解/发送 API | `consumes_topic`、`publishes_topic` | `mq:<topic>` | certain/probable |
| `@Value`、`@ConfigurationProperties`、YAML、Properties | `uses_config`、`defines_config` | `config:<key>` | certain |
| `@Scheduled` | `scheduled_job` | `schedule:<class>#<method>` | certain |
| 迁移 SQL 与 MyBatis SQL | `migration_table`、`queries_table` | `table:<name>` | certain |
| OpenAPI JSON/YAML 与前端 HTTP 调用 | `api_contract`、`calls_http` | `http:<method>:<path>` | certain/probable |

支持 `.java`、`.xml`、`.yml`、`.yaml`、`.properties`、`.sql`、`.json`、`.md`、`.html`、`.ts`、`.tsx`、`.js` 与 `.vue` 的索引。Maven 关系保留原行为。

以下情况必须写 `diagnostics`，并由分析报告加入风险或问题：MyBatis 动态标签、`${}` 路由占位、`Class.forName`/反射调用、代理生成、字符串拼接 SQL、无法识别的消息 topic 或配置值。诊断置信度为 `unresolved`；存在明显但不唯一的候选时使用 `probable`。

## 4. 分析流程

```text
本地需求文档 / 原型 / 文本
  -> input.Facts
  -> 本地规则匹配
  -> 关键词、字段、接口和关系图检索
  -> 统一 AnalysisReport
  -> 本地 reports 持久化
  -> 用户通过 record_analysis_feedback 确认/驳回/新增规则
  -> 下一次分析应用本地规则
```

`analyze_change` 也生成并保存同一报告结构，输入事实标记为 Git 范围。所有报告生成前仍调用增量刷新；刷新失败或基线未知必须显示在报告风险中。

## 5. CLI、MCP 与兼容性

- CLI 新增 `analyze <workspace> <path...>`、`report <workspace> <id>`、`feedback <workspace> <report-id> <kind> <key> <decision> <note>`；旧 CLI 不变。
- MCP 旧工具和 `analyze_requirement(text)` 不变；新工具提供结构化输入、报告读取和本地反馈。
- Schema 8 迁移只新增本地报告/规则表。索引证据表结构变化时再显式重建；不得因为新增报告表删除已有索引。
- 所有输入路径通过 `filepath.Clean`、常规文件检查和大小上限验证；不能读取目录、设备或不存在的文件。

## 6. 验收标准

1. DOCX、HTML、JSON/Figma 导出和截图 OCR/缺失 OCR 均产生可追溯事实或明确诊断。
2. 一次需求分析生成可读取报告，包含基线、项目、模块、文件/方法、测试点、风险和问题。
3. 确认、驳回和规则反馈只落本地数据库；下一次分析可证明规则提升或抑制候选。
4. 合成多仓库 fixture 覆盖 Feign、Dubbo、MQ、配置、定时任务、迁移 SQL、OpenAPI/前端 HTTP、动态 SQL、反射和运行时路由。
5. `go test ./...`、`go vet ./...`、四平台构建和发布布局测试通过；真实 BasisProject 只读验收不改变任一业务仓库状态。
