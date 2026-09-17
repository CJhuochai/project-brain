# Project Brain 1.0 设计说明

## 目标

让本地多仓库微服务代码知识库可稳定服务日常需求开发：从需求文档、原型提取文本或文件/提交/分支变更中，输出可追溯的项目、方法入口、影响范围、风险和待确认项。

## 固定约束

- 仅保存用户本机数据；不上传源码、文档或遥测。
- 默认只读业务仓库：不得 checkout、commit、构建、测试或修改工作区文件。
- 默认基线仍是本地 `refs/remotes/origin/HEAD`；远程同步仅允许显式用户命令，不能隐式联网。
- 任何关系都必须包含来源仓库、文件、行号、索引基线和置信度。
- 解析失败必须生成诊断，不得伪造确定关系。

## 1. 索引正确性

索引以“仓库 + 基线 commit”快照为单位。一次归档完成后，未出现在该归档中的旧文件、符号、边和诊断必须从本地库删除。解析器版本变化时，工具应标记索引需重建，不能静默复用旧关系。

`workspace_status` 必须返回每个仓库的 `index_state`、基线 commit、索引时间、文件数、诊断数和过期原因。

## 2. 方法级 Java/MyBatis 关系

Java 解析器产生类型、方法、参数和注解位置；方法 ID 为 `全限定类型#方法名(参数类型列表)`。建立：

- Controller 路由 → Controller 方法；
- Controller/Service 方法 → 本类或注入依赖的方法调用；
- Mapper 接口方法 → XML namespace + statement ID；
- XML statement → SQL 表；
- Maven 模块 → Maven 模块依赖。

调用解析无法静态唯一确定时，记录 `probable` 候选；反射、动态 SQL、配置中心等记录 `unresolved` 诊断。

## 3. 可用分析输出

新增统一的 `AnalysisReport`：

- `projects`：候选仓库、命中证据和置信度；
- `entry_points`：路由、方法、Mapper statement；
- `change_points`：建议修改位置及来源证据；
- `impacts`：正向/反向关系路径；
- `risks`：跨模块、数据表、动态行为和索引过期风险；
- `questions`：无法由静态证据确认的事项。

需求文本调用和变更输入调用均输出该结构。变更输入支持文件路径、Git commit 和 `base..target` 比较范围，只读取本地 Git 对象。

## 4. 客户端与发布

MCP 提供 `analyze_requirement` 与 `analyze_change`，Codex 插件在需求/原型/二次开发时优先调用前者。发布产物包含 Windows 单文件、校验和、安装/卸载说明和不改用户配置的 MCP 配置样例。

## 5. 验收

- 合成多仓库 fixture 覆盖新增、修改、删除、基线变更、方法调用、Mapper/XML、歧义与诊断。
- `go test ./...`、`go vet ./...`、交叉平台构建通过。
- ExampleWorkspace 只读验收保持所有 Git 工作区状态不变。
- 至少 3 个真实需求由用户验收，分别覆盖单仓库、跨仓库和二次开发/变更影响场景。
