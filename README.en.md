# Project Brain

English | [简体中文](README.md)

![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)
![Platform](https://img.shields.io/badge/platform-Windows-0078D4?logo=windows&logoColor=white)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**A local-first, read-only code knowledge base for multi-repository projects.** Point Project Brain at a microservice workspace to locate the affected repositories, entry points, call paths, change impact, and risks behind a requirement.

> Indexes stay on your computer. Project Brain does not upload business source code or fetch, check out, or modify your business repositories.

## What it does

- Discovers Git repositories beneath a workspace and indexes each locally available `origin/HEAD` default branch.
- Extracts relationships from Java/Spring, MyBatis XML, and Maven `pom.xml` files.
- Analyzes requirement text, prototype text, and follow-up development requests into candidate repositories, evidence, impact points, related code, and risks.
- Analyzes a file, commit, or `base..target` range and refreshes only repositories whose baseline changed.
- Exposes standard MCP tools for Codex; the optional Codex plugin prioritizes the knowledge base for requirement and change requests.

## Quick start

Download the Windows binary from [v1.0.0](https://github.com/CJhuochai/project-brain/releases/tag/v1.0.0), or build locally with Go 1.25+:

```powershell
go build -o bin\project-brain.exe ./cmd/project-brain
```

```powershell
bin\project-brain.exe discover E:\ExampleWorkspace
bin\project-brain.exe index E:\ExampleWorkspace
bin\project-brain.exe status E:\ExampleWorkspace
bin\project-brain.exe search E:\ExampleWorkspace groupSubmit
bin\project-brain.exe trace E:\ExampleWorkspace com.example.EntryController
bin\project-brain.exe impact E:\ExampleWorkspace com.example.EntryService
```

Every command returns JSON. `trace` follows a symbol only when it is unique; ambiguous symbols are returned as candidates. `impact` preserves `certain` and `probable` confidence.

## Codex MCP

```powershell
bin\project-brain.exe mcp E:\ExampleWorkspace
```

Supported tools: `workspace_status`, `find_business_context`, `trace_code_path`, `analyze_change_impact`, `analyze_change`, `analyze_requirement`, and `get_evidence`.

Copy [examples/codex-config.toml](examples/codex-config.toml), update the local binary and workspace paths, then add it to Codex and restart the MCP client. The optional adapter at `codex-plugin/project-brain-codex` only tells Codex to use `analyze_requirement` first; it contains neither business source nor indexes.

## Privacy and limits

- Indexes live in `%LOCALAPPDATA%\ProjectBrain` (or the XDG local data directory). Delete a workspace-hash directory to erase that workspace's index.
- Project Brain never checks out, fetches, commits, builds, tests, or writes to business repositories.
- Version 1.0 uses deterministic text extraction for Java/Spring, MyBatis XML, and Maven. Reflection, dynamic SQL, and runtime routing require human verification.

## Verification and release

- [1.0 local acceptance record](docs/acceptance/project-brain-1.0.md): 30 repositories discovered, 29 indexed, and 11,172 files indexed; validated against real requirement documents and a Git commit.
- [v1.0.0 release](https://github.com/CJhuochai/project-brain/releases/tag/v1.0.0): Windows x64 executable and SHA-256 checksum.

## License

Licensed under the [MIT License](LICENSE).
