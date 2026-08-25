# Project Brain

English | [简体中文](README.md)

![Project Brain Logo](codex-plugin/project-brain-codex/assets/chenpi-brain.png)

![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)
![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux-2EA44F)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**A local-first, read-only code knowledge base for multi-repository projects.** Point Project Brain at a microservice workspace to locate the affected repositories, entry points, call paths, change impact, and risks behind a requirement.

> Indexes stay on your computer. Project Brain does not upload business source code or fetch, check out, or modify your business repositories.

## What it does

- Discovers Git repositories beneath a workspace and indexes each locally available `origin/HEAD` default branch.
- Extracts relationships from Java/Spring, MyBatis XML, and Maven `pom.xml` files.
- Analyzes requirement text, prototype text, and follow-up development requests into candidate repositories, evidence, impact points, related code, and risks.
- Analyzes a file, commit, or `base..target` range and refreshes only repositories whose baseline changed.
- Exposes standard MCP tools for Codex; the optional Codex plugin prioritizes the knowledge base for requirement and change requests.

## Install and use

Download the matching binary from the [latest release](https://github.com/CJhuochai/project-brain/releases/latest). Verify it with `sha256sum -c <file>.sha256` on macOS/Linux, or `Get-FileHash <file>` on Windows.

| Platform | Release file | Run |
| --- | --- | --- |
| Windows x64 | `project-brain-<version>-windows-amd64.exe` | `./project-brain-<version>-windows-amd64.exe status <workspace>` |
| Linux x64 | `project-brain-<version>-linux-amd64` | Run `chmod +x project-brain-<version>-linux-amd64`, then execute it |
| macOS Intel | `project-brain-<version>-darwin-amd64` | Run `chmod +x project-brain-<version>-darwin-amd64`, then execute it |
| macOS Apple Silicon | `project-brain-<version>-darwin-arm64` | Run `chmod +x project-brain-<version>-darwin-arm64`, then execute it |

Or build locally with Go 1.25+:

```powershell
go build -o bin\project-brain.exe ./cmd/project-brain
```

```powershell
bin\project-brain.exe discover E:\BasisProject
bin\project-brain.exe index E:\BasisProject
bin\project-brain.exe status E:\BasisProject
bin\project-brain.exe search E:\BasisProject groupSubmit
bin\project-brain.exe trace E:\BasisProject com.example.EntryController
bin\project-brain.exe impact E:\BasisProject com.example.EntryService
```

Every command returns JSON. `trace` follows a symbol only when it is unique; ambiguous symbols are returned as candidates. `impact` preserves `certain` and `probable` confidence.

## Codex MCP

```powershell
bin\project-brain.exe mcp E:\BasisProject
```

Supported tools: `workspace_status`, `find_business_context`, `trace_code_path`, `analyze_change_impact`, `analyze_change`, `analyze_requirement`, `analyze_inputs`, `get_analysis_report`, `record_analysis_feedback`, and `get_evidence`.

## 1.1 requirement loop

Keep requirement documents, HTML prototypes, local Figma JSON exports, or DOCX files on the local machine:

```powershell
bin\project-brain.exe analyze E:\BasisProject C:\local\entry-requirement.docx C:\local\prototype.html
bin\project-brain.exe report E:\BasisProject <report-id>
bin\project-brain.exe feedback E:\BasisProject <report-id> repository entry-service rule "confirmed"
```

The MCP equivalent is `analyze_inputs`, with `text`, local `paths`, and optional `source_ref`. Reports include modules, suggested (not created) branches, file/method evidence, test points, risks, and an input digest. Attachments are never copied or uploaded. Feedback writes only to the local index and is applied as an exact rule in later analyses.

Copy [examples/codex-config.toml](examples/codex-config.toml), update the local binary and workspace paths, then add it to Codex and restart the MCP client. On Windows `command` targets the `.exe`; on macOS/Linux it targets the matching executable after `chmod +x`:

```toml
[mcp_servers.project_brain]
command = "/absolute/path/project-brain-<version>-darwin-arm64"
args = ["mcp", "/absolute/path/to/workspace"]
```

The optional adapter at `codex-plugin/project-brain-codex` only tells Codex to use `analyze_requirement` first; it contains neither business source nor indexes.

## 1.2 concurrency consistency

Version 1.2 runs one local coordinator per workspace, so multiple Codex sessions and CLI calls no longer write the same SQLite index. Source evidence is stored in immutable `snapshot-<id>.sqlite` files; `control.sqlite` holds only coordination state, reports, feedback, and rule revisions.

- Default `stable` returns the latest complete snapshot immediately and marks a changed baseline as `refreshing`.
- `latest` waits for the merged refresh; on timeout it still returns a complete older snapshot.
- Read tools accept `freshness: "stable" | "latest"` and an optional fixed `snapshot_id`; responses include `snapshot_id`, `rule_revision`, `freshness`, and `active_baseline`.
- A report records the snapshot and rule revision selected at its start; later feedback never rewrites history.

See the [v1.2 architecture note](docs/architecture-v1.2.md). After a crash, the next `status` or MCP call removes unfinished staging snapshots and continues from the last complete snapshot. Removing a workspace data directory also removes its local index, reports, and feedback.

## Privacy and limits

- Indexes live in `%LOCALAPPDATA%\ProjectBrain` (or the XDG local data directory). Delete a workspace-hash directory to erase that workspace's index.
- Project Brain never checks out, fetches, commits, builds, tests, or writes to business repositories.
- Version 1.0 uses deterministic text extraction for Java/Spring, MyBatis XML, and Maven. Reflection, dynamic SQL, and runtime routing require human verification.

## Verification and release

- [1.0 local acceptance record](docs/acceptance/project-brain-1.0.md): 30 repositories discovered, 29 indexed, and 11,172 files indexed; validated against real requirement documents and a Git commit.
- [v1.0.0 release](https://github.com/CJhuochai/project-brain/releases/tag/v1.0.0): Windows x64, Linux x64, macOS Intel, and macOS Apple Silicon executables with SHA-256 checksums.
- [Changelog](CHANGELOG.md): user-facing changes. Pushing a `vX.Y.Z` tag runs tests, builds assets, checks their layout, and creates the GitHub Release automatically.

## License

Licensed under the [MIT License](LICENSE).
