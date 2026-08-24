# Project Brain

[English](README.en.md) | 简体中文

![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)
![Platform](https://img.shields.io/badge/platform-Windows-0078D4?logo=windows&logoColor=white)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**本地优先、只读的多仓库代码知识库。** 给它一个微服务工作区，Project Brain 会扫描各仓库的远程默认分支索引，帮助开发 Agent 定位需求涉及的项目、代码入口、调用链、变更影响与待确认风险。

> 所有索引保存在你的电脑；不上传业务源码，不 fetch、checkout 或修改业务仓库。

## 它能做什么

- 自动发现工作区下的 Git 仓库，并以本机已有的 `origin/HEAD` 默认分支为基线。
- 从 Java/Spring、MyBatis XML、Maven `pom.xml` 提取接口、服务、Mapper、表与模块关系。
- 对需求文档、交互原型文字或二次开发描述，给出候选仓库、代码证据、影响点、关联点和风险。
- 对文件、提交或 `base..target` 比较范围分析改动影响；基线变化时自动增量刷新。
- 通过标准 MCP 接入 Codex；可选插件会在收到需求或变更请求时优先调用知识库。

## 快速开始

下载 [v1.0.0](https://github.com/CJhuochai/project-brain/releases/tag/v1.0.0) 的 Windows 单文件，或本地构建（需要 Go 1.25+）：

```powershell
go build -o bin\project-brain.exe ./cmd/project-brain
```

对一个微服务工作区执行：

```powershell
bin\project-brain.exe discover E:\ExampleWorkspace
bin\project-brain.exe index E:\ExampleWorkspace
bin\project-brain.exe status E:\ExampleWorkspace
bin\project-brain.exe search E:\ExampleWorkspace groupSubmit
bin\project-brain.exe trace E:\ExampleWorkspace com.example.EntryController
bin\project-brain.exe impact E:\ExampleWorkspace com.example.EntryService
```

所有命令默认输出 JSON。`trace` 只在目标符号唯一时向下遍历；同名符号会返回候选而不会猜测。`impact` 会保留 `certain` 与 `probable` 置信度。

## 接入 Codex MCP

```powershell
bin\project-brain.exe mcp E:\ExampleWorkspace
```

支持 `workspace_status`、`find_business_context`、`trace_code_path`、`analyze_change_impact`、`analyze_change`、`analyze_requirement` 与 `get_evidence`。

复制 [examples/codex-config.toml](examples/codex-config.toml) 的配置，按本机二进制与工作区路径修改后添加到 Codex 配置并重启客户端。可选的 Codex 适配插件位于 `codex-plugin/project-brain-codex`；它只规定需求/原型/二次开发请求优先走 `analyze_requirement`，不含业务源码或索引。

`analyze_change` 接收默认基线文件路径、单个本地 commit 或 `base..target`；单个 commit 会精确定位所在仓库，`HEAD~1..HEAD` 等相对范围则在每个仓库内分别解释。

## 隐私与边界

- 索引库位于 `%LOCALAPPDATA%\ProjectBrain`（其他系统遵循 XDG 本地数据目录），删除对应工作区哈希目录即可清除。
- 不会执行 checkout、fetch、commit、构建或测试业务仓库，也不会向业务仓库写任何文件。
- 当前 1.0 使用确定性文本提取 Java/Spring、MyBatis XML 与 Maven 关系；反射、动态 SQL、运行时路由等结论必须人工确认。

## 验收与发布

- [1.0 本地验收记录](docs/acceptance/project-brain-1.0.md)：30 个仓库发现、29 个完成索引、11,172 个文件；已用真实需求和 Git 提交验证。
- [v1.0.0 发布页](https://github.com/CJhuochai/project-brain/releases/tag/v1.0.0)：提供 Windows x64 可执行文件及 SHA-256 校验文件。

## 许可证

本项目采用 [MIT License](LICENSE)。
