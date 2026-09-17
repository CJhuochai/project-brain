# Contributing / 贡献指南

欢迎提交问题、文档修正和聚焦的 Pull Request。较大功能先通过 Issue 说明问题、预期行为与边界。

## 开发与验证

需要 Go 1.25+ 和 Git。克隆仓库后运行：

```sh
go test ./... -count=1
go vet ./...
go build ./cmd/project-brain
git diff --check
python scripts/check-public-data.py
```

测试使用临时合成仓库。真实工作区验收必须显式传入本地路径，输出留在仓库外；不要提交业务源码、附件、索引、报告或凭据。可选真实增量基准使用 `PROJECT_BRAIN_REAL_WORKSPACE`，默认跳过。

保持本地优先、业务仓库只读、稳定快照以及证据/推测分离。修复或功能变更补充匹配风险的验证；文档变更检查链接和命令。提交前确认 UTF-8、测试结果、兼容性与未覆盖项。使用功能分支和 PR，不直接修改发布标签。

Contributions are licensed under the repository's MIT License. Please submit focused changes, include relevant validation, and never attach private source, credentials, or raw workspace evidence.

## 本地验收脚本

`scripts/accept-workspace.ps1` 要求 `-Executable` 和 `-Workspace`。
`scripts/verify-v1.2.ps1` 额外要求 `-Query`。
`scripts/verify-v2.ps1` 额外要求 `-OutputDirectory`、四个 `-Queries` 和 `-Requirement`；第一个查询应定位类，第四个查询应定位方法。输出目录必须留在仓库外，输出含源码证据和本地路径，不适合公开。
