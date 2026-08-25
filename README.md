# Project Brain

[English](README.en.md) | 简体中文

![Project Brain Logo](codex-plugin/project-brain-codex/assets/chenpi-brain.png)

![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)
![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux-2EA44F)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**本地优先、只读的多仓库代码知识库。** 给它一个微服务工作区，Project Brain 会扫描各仓库的远程默认分支索引，帮助开发 Agent 定位需求涉及的项目、代码入口、调用链、变更影响与待确认风险。

> 所有索引保存在你的电脑；不上传业务源码，不 fetch、checkout 或修改业务仓库。

## 它能做什么

- 自动发现工作区下的 Git 仓库，并以本机已有的 `origin/HEAD` 默认分支为基线。
- 从 Java/Spring、MyBatis XML、Maven `pom.xml` 提取接口、服务、Mapper、表与模块关系。
- 对需求文档、交互原型文字或二次开发描述，给出候选仓库、代码证据、影响点、关联点和风险。
- 对文件、提交或 `base..target` 比较范围分析改动影响；基线变化时自动增量刷新。
- 通过标准 MCP 接入 Codex；可选插件会在收到需求或变更请求时优先调用知识库。

## 安装与使用

从 [v1.0.0](https://github.com/CJhuochai/project-brain/releases/tag/v1.0.0) 下载与你平台匹配的文件；下载校验文件后，可用 `sha256sum -c <文件名>.sha256`（macOS/Linux）或 `Get-FileHash <文件名>`（Windows）校验。

| 平台 | 发布文件 | 运行方式 |
| --- | --- | --- |
| Windows x64 | `project-brain-1.0.0-windows-amd64.exe` | `./project-brain-1.0.0-windows-amd64.exe status <工作区>` |
| Linux x64 | `project-brain-1.0.0-linux-amd64` | `chmod +x project-brain-1.0.0-linux-amd64` 后执行 `./project-brain-1.0.0-linux-amd64 status <工作区>` |
| macOS Intel | `project-brain-1.0.0-darwin-amd64` | `chmod +x project-brain-1.0.0-darwin-amd64` 后执行 |
| macOS Apple Silicon | `project-brain-1.0.0-darwin-arm64` | `chmod +x project-brain-1.0.0-darwin-arm64` 后执行 |

也可本地构建（需要 Go 1.25+）：

```powershell
go build -o bin\project-brain.exe ./cmd/project-brain
```

以 Windows 为例，对一个微服务工作区执行：

```powershell
bin\project-brain.exe discover E:\BasisProject
bin\project-brain.exe index E:\BasisProject
bin\project-brain.exe status E:\BasisProject
bin\project-brain.exe search E:\BasisProject groupSubmit
bin\project-brain.exe trace E:\BasisProject com.example.EntryController
bin\project-brain.exe impact E:\BasisProject com.example.EntryService
```

所有命令默认输出 JSON。`trace` 只在目标符号唯一时向下遍历；同名符号会返回候选而不会猜测。`impact` 会保留 `certain` 与 `probable` 置信度。

## 接入 Codex MCP

```powershell
bin\project-brain.exe mcp E:\BasisProject
```

支持 `workspace_status`、`find_business_context`、`trace_code_path`、`analyze_change_impact`、`analyze_change`、`analyze_requirement`、`analyze_inputs`、`get_analysis_report`、`record_analysis_feedback` 与 `get_evidence`。

## 1.1 日常需求闭环

把需求文档、HTML 原型、Figma 本地 JSON 导出或 DOCX 放在本机后执行：

```powershell
bin\project-brain.exe analyze E:\BasisProject C:\local\entry-requirement.docx C:\local\prototype.html
bin\project-brain.exe report E:\BasisProject <报告ID>
bin\project-brain.exe feedback E:\BasisProject <报告ID> repository entry-service rule "人工确认"
```

`analyze_inputs` 是对应的 MCP 工具，接收 `text`、本地 `paths` 与可选 `source_ref`。结果包含候选项目/模块、建议分支（仅建议，不创建 Git 分支）、文件/方法证据、测试点、风险和输入摘要；原始附件不会被复制或上传。人工反馈只写入本机索引库，并会在下一次分析时按精确项目规则显示。

复制 [examples/codex-config.toml](examples/codex-config.toml) 的配置，按本机二进制与工作区路径修改后添加到 Codex 配置并重启客户端。Windows 的 `command` 指向 `.exe`；macOS/Linux 指向已 `chmod +x` 的对应二进制，例如：

```toml
[mcp_servers.project_brain]
command = "/absolute/path/project-brain-1.0.0-darwin-arm64"
args = ["mcp", "/absolute/path/to/workspace"]
```

可选的 Codex 适配插件位于 `codex-plugin/project-brain-codex`；它只规定需求/原型/二次开发请求优先走 `analyze_requirement`，不含业务源码或索引。

`analyze_change` 接收默认基线文件路径、单个本地 commit 或 `base..target`；单个 commit 会精确定位所在仓库，`HEAD~1..HEAD` 等相对范围则在每个仓库内分别解释。

## 1.2 并发一致性

从 1.2 起，同一工作区由一个本机协调者管理，多个 Codex 会话或 CLI 不会再同时写同一 SQLite 索引。索引以不可变 `snapshot-<id>.sqlite` 保存；`control.sqlite` 只保存协调状态、报告、反馈和规则版本。

- 默认 `stable`：立即读最近完整快照；基线变化时响应会标记 `refreshing`。
- `latest`：等待已合并的刷新任务完成；超时时仍返回完整旧快照。
- 所有读工具可传 `freshness: "stable" | "latest"` 和固定 `snapshot_id`；响应带 `snapshot_id`、`rule_revision`、`freshness` 与 `active_baseline`。
- 报告固定使用启动时的快照和规则修订号；历史报告不会被后续反馈改写。

详见 [v1.2 架构说明](docs/architecture-v1.2.md)。若进程意外退出，下一次 `status` 或 MCP 调用会清理未完成 staging 快照，并继续使用最后完整快照。删除工作区数据目录会同时删除本地索引、报告和反馈。

## 隐私与边界

- 索引库位于 `%LOCALAPPDATA%\ProjectBrain`（其他系统遵循 XDG 本地数据目录），删除对应工作区哈希目录即可清除。
- 不会执行 checkout、fetch、commit、构建或测试业务仓库，也不会向业务仓库写任何文件。
- 当前 1.0 使用确定性文本提取 Java/Spring、MyBatis XML 与 Maven 关系；反射、动态 SQL、运行时路由等结论必须人工确认。

## 验收与发布

- [1.0 本地验收记录](docs/acceptance/project-brain-1.0.md)：30 个仓库发现、29 个完成索引、11,172 个文件；已用真实需求和 Git 提交验证。
- [v1.0.0 发布页](https://github.com/CJhuochai/project-brain/releases/tag/v1.0.0)：提供 Windows x64、Linux x64、macOS Intel、macOS Apple Silicon 可执行文件及 SHA-256 校验文件。

## 许可证

本项目采用 [MIT License](LICENSE)。
