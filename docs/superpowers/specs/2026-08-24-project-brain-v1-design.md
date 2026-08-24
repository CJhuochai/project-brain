# Project Brain v1 设计说明

## 目标

提供一个仅在本机运行、只读工作区的索引工具。它让支持 MCP 的编码 Agent 在改业务代码前，定位相关 Java 微服务仓库、追踪有证据的代码链路，并报告修改影响面。

## 范围

第一版接收一个工作区根目录，自动发现其中的 Git 仓库；在不改变仓库状态的前提下，索引每个仓库本地记录的远程默认主分支。支持 Java、Maven POM、Spring 注解、MyBatis Mapper XML、YAML/Properties 配置、SQL、Markdown 和 OpenAPI 风格的 YAML/JSON 文档。

工具提供可移植的 CLI 和标准 stdio MCP Server。Codex 插件仅是一个可选的薄封装，负责提示 Codex 在提出代码修改方案前调用 MCP 工具。所有项目数据仅保存在用户电脑。

## 非目标

- 不修改、切换、拉取、构建、测试、提交业务仓库，也不安装 Git Hook。
- 不向远端服务发送源码、文档文本、向量或遥测数据。
- 不把 Spring 动态装配、反射、配置中心路由或工作区外服务当成静态代码已经证明的事实。
- 不交付浏览器 UI、图数据库服务、向量数据库或自动改业务代码的工作流。
- 不要求 BasisProject 专属的名称、路径、服务名或分支名。

## 用户使用流程

```text
project-brain init E:\\BasisProject
project-brain update E:\\BasisProject
project-brain status E:\\BasisProject
project-brain search "招生 面试 排考"
project-brain trace --route /group-submit
project-brain impact --symbol StudentEntryApplication.groupSubmit
project-brain mcp serve --workspace E:\\BasisProject
```

`init` 只创建用户自己的本地数据；`update` 只刷新已变化文件的记录；`search`、`trace` 和 `impact` 默认输出结构化 JSON，也可输出简洁终端文本。

## 隐私与存储

使用平台本地数据目录：

- Windows：`%LOCALAPPDATA%\\ProjectBrain`
- macOS：`~/Library/Application Support/ProjectBrain`
- Linux：`${XDG_DATA_HOME:-~/.local/share}/project-brain`

每个工作区以规范化路径和发现到的仓库身份生成稳定 ID；目录下包含 `workspace.json`、`index.sqlite`、`artifacts/` 与 `logs/`。工作区和业务仓库本身不写入任何文件。

工具在本地保存源码事实和可选的用户需求文档提取文本。每一条关系都记录来源文件、行范围、解析器版本、索引提交、置信度和创建时间；所有查询结果必须返回这些证据字段。

## 仓库与基线发现

扫描器仅遍历用户选择的工作区根目录，识别 `.git` 目录或 Git worktree 文件；跳过构建产物和依赖目录，将嵌套仓库视为独立仓库。

基线从本地远程跟踪分支的 HEAD 引用读取，优先使用 `refs/remotes/origin/HEAD`。工具记录该引用指向的分支和提交，但绝不 checkout。若无法从本地 Git 数据确定远程默认主分支，则该仓库标记为 `baseline_unknown`，不会用于生成代码事实结论。未来可提供显式刷新远程引用的命令，但 v1 没有隐式网络操作。

## 总体架构

```text
CLI / MCP
    → 工作区服务
        → Git 仓库发现
        → 内容变更检测
        → 解析器适配器注册表
        → SQLite 事实与关系存储
        → 按证据排序的查询服务
```

实现采用 Go 1.23 模块。Go 提供跨平台单二进制 CLI 和 stdio MCP Server；SQLite + FTS5 保存元数据、源码片段、图关系和全文索引。图遍历通过索引后的邻接关系表和递归 CTE 完成，不需要 Neo4j、Docker 或任何常驻服务。

解析器适配器拥有统一接口：发现文件、提取符号、提取关系、报告不支持语法。Java 适配器使用 Tree-sitter 做确定性结构提取；XML/YAML/Properties/SQL 适配器只产生可显式证明的引用。禁止使用 LLM 凭空生成代码图关系。

## v1 提取的事实

通用事实：

- 工作区、仓库、基线、Maven 模块、源码文件、内容哈希；
- 符号、符号类型、导入、继承、实现、调用引用；
- 文档业务词、配置键、SQL 表名词元、测试引用。

Java/Spring/MyBatis 事实：

- 类、接口、方法、构造函数、包、导入；
- `@Controller`、`@RestController`、路由注解与 HTTP 路径；
- `@Service`、`@Component`、`@Repository`、`@FeignClient` 与显式请求映射；
- MyBatis Mapper 接口、XML namespace、statement ID、SQL 表名词元；
- Maven 模块及模块间依赖。

每条推导关系均标记为 `certain`、`probable` 或 `unresolved`。仅靠名称或接收者启发式解析到的方法调用标记为 `probable`；不能解析的动态行为只记录诊断信息，不伪造调用边。

## 查询能力

`search` 在文件、符号、路由、文档和配置词中做 FTS 检索；排序优先精确标识符/路由命中、模块接近度、图连接度和证据置信度。

`trace` 从精确路由、全限定符号或无歧义的符号候选开始，进行有深度上限的下游遍历，返回如 Controller → Service → Mapper 的路径和来源证据。符号重名时返回候选列表，禁止猜测。

`impact` 从文件、路由或符号做有深度上限的反向遍历，列出调用方、依赖模块、路由、Mapper XML statement 和测试引用；确定影响与可能影响分开显示。

`workspace_status` 返回仓库数量、基线状态、上次索引时间、变化文件、解析诊断和过期仓库。Agent 在采用索引结论前必须先检查该状态。

## 需求分析

v1 接收 UTF-8 文本、Markdown、README/docs、OpenAPI、SQL 和用户提供文档提取出的文本；不把视觉原型图片本身当作代码证据。

分析流程先提取字面业务词、路由、实体名、字段名与错误词；再调用 `search` 扩展图邻居证据，输出仓库候选、可能代码路径、影响、风险和未确认问题。所有依赖排序得到的结论必须标为候选，并引用使其命中的索引文件。

## MCP 与 Codex 集成

MCP Server 使用 stdio，暴露：`workspace_status`、`find_business_context`、`trace_code_path`、`analyze_change_impact`、`analyze_requirement`、`get_evidence`。

可选 Codex 插件不包含索引逻辑，只注册 MCP 命令和一个 Skill。该 Skill 强制要求：检查新鲜度、获取证据、先给只读方案、写明风险与未确认项，再允许修改业务代码。其他支持 MCP 的客户端可直接运行同一条 Server 命令，不依赖 Codex 插件。

## 更新语义

`update` 仅读取 Git 元数据和文件内容，使用基线提交与内容哈希识别变化，仅重解析变化的受支持文件；删除文件时只删除本地索引记录。该命令不改变项目文件和 Git 状态。

MCP 适配层可在任务开始时执行 `update`，因为它只写用户自己的 Project Brain 索引；必须报告更新是否完成以及哪些仓库仍过期。

## 验证

使用合成的多仓库 fixture 工作区进行单元与集成测试，覆盖仓库发现、零写入保证、基线识别、增量更新、Java/Spring/MyBatis 提取、符号歧义、trace/impact 证据、FTS 检索和 MCP JSON-RPC 协议。

增加一个只读 BasisProject 验收脚本，只验证发现和状态；不得将生产源码文本写入测试仓库，也不得修改任何 BasisProject checkout。

## 发布标准

- 全新安装可在本地扫描多仓库 Java 工作区。
- 没有可知远程默认基线的仓库必须显式报告，不能静默使用开发分支。
- `trace` 和 `impact` 报出的每条关系都有源码证据。
- 没有源码变化时，重复 `update` 不重新解析文件。
- fixture 仓库在扫描前后 `git status` 完全一致。
- MCP 工具由同一个本地 CLI 二进制提供，不依赖网络连接。

## 延后功能

本地离线 Embedding、文档渲染/OCR、文件监听、TypeScript 适配器、Git diff/PR 分析、共享元数据导出和 UI，均在证据优先的 CLI/MCP 工作流通过验收后再考虑。
