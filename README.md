# Project Brain

本地优先、只读的多仓库代码知识工具。它扫描一个工作区中的 Git 仓库，只读取每个仓库本地已记录的 `origin/HEAD` 默认基线，将索引保存到当前用户电脑，并提供代码证据检索、调用链和影响分析。

项目只保存自身源码和合成测试数据；不会提交用户工作区、索引库、日志或业务源码。

## 使用

需要 Go 1.25+。在项目目录构建：

```powershell
go build -o bin\project-brain.exe ./cmd/project-brain
```

对工作区执行：

```powershell
bin\project-brain.exe discover E:\ExampleWorkspace
bin\project-brain.exe index E:\ExampleWorkspace
bin\project-brain.exe status E:\ExampleWorkspace
bin\project-brain.exe search E:\ExampleWorkspace groupSubmit
bin\project-brain.exe trace E:\ExampleWorkspace com.example.EntryController
bin\project-brain.exe impact E:\ExampleWorkspace com.example.EntryService
```

所有命令默认输出 JSON。`trace` 只在目标符号唯一时向下遍历；同名符号会返回候选，不会猜测。`impact` 将 `certain` 与 `probable` 置信度原样带回。

MCP 工具调用前会比较本机 `origin/HEAD` 与已索引 commit：基线变化时自动增量刷新，未变化时复用现有索引。它不会 fetch、checkout 或修改业务仓库。

## MCP

标准输入输出模式：

```powershell
bin\project-brain.exe mcp E:\ExampleWorkspace
```

支持 `workspace_status`、`find_business_context`、`trace_code_path`、`analyze_change_impact`、`analyze_requirement` 和 `get_evidence` 六个工具。Codex 的配置样例见 [examples/codex-config.toml](examples/codex-config.toml)，需要按本机路径修改；本工具不会修改你的 Codex 配置。

可选的 Codex 适配插件位于 `codex-plugin/project-brain-codex`。它不包含索引或业务源码，只规定：收到需求文档、原型或二次开发请求时，优先调用 `analyze_requirement` 再给出代码方案。

## 隐私与边界

- 索引库位于 `%LOCALAPPDATA%\ProjectBrain`（其他系统遵循 XDG 本地数据目录）。删除对应工作区哈希目录即可清除索引。
- 不会执行 checkout、fetch、commit、build 或测试业务仓库；不会向业务仓库写入任何文件。
- 仅索引本地已经存在的远程默认分支引用。`origin/HEAD` 不存在时明确返回 `baseline_unknown`。
- 当前第一版使用确定性文本提取 Java/Spring、MyBatis XML 和 Maven `pom.xml` 关系；反射、动态 SQL、运行时路由等只能作为后续增强，不能据此认为分析完整。
- 开源前请确认 Git remote、配置样例、测试输出中没有个人令牌或私有地址。本项目已对发现到的 remote URL 移除用户信息。
