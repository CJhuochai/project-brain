# Project Brain v1 Design

## Goal

Provide a local-only, read-only workspace index that lets an MCP-capable coding agent locate relevant Java microservice repositories, trace evidence-backed code paths, and report change impact before any business code is edited.

## Scope

The first release accepts a workspace root, discovers Git repositories below it, and indexes the locally recorded remote default branch for each repository without changing the repository. It supports Java, Maven POM files, Spring annotations, MyBatis Mapper XML, YAML/properties configuration, SQL, Markdown, and OpenAPI-like YAML/JSON documents.

It exposes a portable CLI and a standard stdio MCP server. A Codex plugin is an optional thin package that teaches Codex to call the MCP tools before proposing code changes. All project data remains on the user's machine.

## Non-goals

- Do not modify, checkout, fetch, build, test, commit, or install hooks in indexed repositories.
- Do not send source, document text, embeddings, or telemetry to a remote service.
- Do not claim dynamic Spring wiring, reflection, config-center routing, or external services are statically proven.
- Do not ship a browser UI, graph database server, vector database, or automatic code-editing workflow.
- Do not require ExampleWorkspace-specific names, paths, service names, or branch names.

## User-visible workflow

```text
project-brain init E:\\ExampleWorkspace
project-brain update E:\\ExampleWorkspace
project-brain status E:\\ExampleWorkspace
project-brain search "招生 面试 排考"
project-brain trace --route /group-submit
project-brain impact --symbol StudentEntryApplication.groupSubmit
project-brain mcp serve --workspace E:\\ExampleWorkspace
```

`init` creates only user-owned local data. `update` detects changed source files and refreshes those records. `search`, `trace`, and `impact` return structured JSON by default, with a concise terminal rendering when requested.

## Privacy and storage

Use a platform-local data directory:

- Windows: `%LOCALAPPDATA%\\ProjectBrain`
- macOS: `~/Library/Application Support/ProjectBrain`
- Linux: `${XDG_DATA_HOME:-~/.local/share}/project-brain`

Each workspace gets a stable ID derived from its normalized path and discovered repository identities. Its data directory contains `workspace.json`, `index.sqlite`, `artifacts/`, and `logs/`. The workspace itself is never written.

The tool stores source-derived facts and optional user-provided requirement extracts locally. Each relation stores a source file, line range, parser version, indexed commit, confidence, and creation time. Any answer must return these evidence fields.

## Repository and baseline discovery

The scanner walks only directories under the selected workspace root. It recognizes a repository by `.git` directory or Git worktree file, skips nested build/dependency directories, and treats nested repositories as separate repositories.

The baseline is read from the local remote-tracking HEAD reference, preferring `refs/remotes/origin/HEAD`. The target branch and commit are recorded without checking it out. If no remote default branch can be determined from local Git data, the repository is listed as `baseline_unknown` and excluded from code-claiming results. An explicit future refresh command may fetch, but v1 has no implicit network operation.

## Architecture

```text
CLI / MCP
    -> workspace service
        -> Git repository discovery
        -> content-change detector
        -> parser adapter registry
        -> SQLite fact and edge store
        -> evidence-ranked query service
```

The implementation is a Go 1.23 module. Go provides a single cross-platform CLI and stdio MCP server. SQLite with FTS5 stores all metadata, source fragments, graph edges, and full-text indexes. Graph traversals use indexed adjacency tables and recursive CTEs; no server or graph database is required.

Parser adapters have a common interface: discover files, extract symbols, extract relations, and report unsupported constructs. The Java adapter uses Tree-sitter for deterministic structural extraction. XML/YAML/Properties/SQL adapters create only explicit references. No LLM creates code-graph edges.

## v1 extracted facts

Generic facts:

- workspace, repository, baseline, Maven module, source file, content hash;
- symbol, symbol kind, import, inheritance, implementation, call reference;
- document term, configuration key, SQL table token, test reference.

Java/Spring/MyBatis facts:

- classes, interfaces, methods, constructors, packages, imports;
- `@Controller`, `@RestController`, mapping annotations and HTTP routes;
- `@Service`, `@Component`, `@Repository`, `@FeignClient` and explicit request mappings;
- MyBatis Mapper interfaces, XML namespaces, statement IDs, SQL table tokens;
- Maven modules and inter-module dependencies.

Every inferred edge has `certain`, `probable`, or `unresolved` confidence. Method calls resolved only by name or receiver heuristics are `probable`; unresolvable dynamic behavior is stored as a diagnostic, not a call edge.

## Queries

`search` combines FTS matches across files, symbols, routes, documents, and configuration terms. Results are ranked by exact identifier/route match, module proximity, graph connectivity, and evidence confidence.

`trace` starts at an exact route, fully-qualified symbol, or unambiguous symbol candidate. It performs bounded downstream traversal and returns paths such as controller to service to mapper, including source evidence. Ambiguous symbols return candidates rather than guessing.

`impact` performs bounded reverse traversal from a file, route, or symbol and lists callers, dependent modules, routes, Mapper XML statements, and test references. It separates confirmed impacts from possible impacts.

`workspace_status` reports repository count, baseline state, last index time, changed files, parser diagnostics, and stale repositories. Agents must check this before trusting results.

## Requirement analysis

v1 accepts UTF-8 text, Markdown, README/docs files, OpenAPI files, SQL, and text extracted from a supplied document. It does not treat a visual prototype image as code evidence.

The analysis pipeline extracts literal business terms, routes, entity names, field names, and error terms. It calls `search`, expands graph-neighbor evidence, then produces a proposal with repository candidates, likely code paths, impacts, risks, and unresolved questions. It must label all ranking-based results as candidates and cite the indexed files that caused the ranking.

## MCP and Codex integration

The MCP server uses stdio and exposes `workspace_status`, `find_business_context`, `trace_code_path`, `analyze_change_impact`, `analyze_requirement`, and `get_evidence`.

The optional Codex plugin contains no indexer logic. It registers the MCP command and a skill that requires: refresh/staleness check, evidence retrieval, a read-only plan, explicit risks, and explicit unresolved items before code changes. Other MCP clients run the same server command without the plugin.

## Update semantics

`update` reads Git metadata and file content only. It identifies file changes using the baseline commit plus content hashes and re-parses only changed supported files. Deleted files remove only their local index records. The command never changes project files or Git state.

The MCP adapter may run `update` at task start because it writes only the user's local Project Brain index. It must show whether it completed and which repositories remain stale.

## Verification

Use synthetic fixture workspaces for unit and integration tests. Tests cover repository discovery, no-write guarantees, baseline detection, incremental updates, Java/Spring/MyBatis extraction, ambiguous symbols, trace/impact evidence, FTS search, and MCP JSON-RPC protocol behavior.

Add a read-only ExampleWorkspace acceptance script that verifies discovery and status only; it must not store production source text in the test repository or alter any ExampleWorkspace checkout.

## Release criteria

- A clean install initializes and scans a multi-repository Java workspace locally.
- Repositories with no known remote default baseline are reported, never silently indexed from a development branch.
- `trace` and `impact` return source evidence for every reported relation.
- A repeated update with no changes does not re-parse source files.
- Git status before and after scanning is byte-for-byte unchanged for every fixture repository.
- MCP calls work through the same local CLI binary and require no network connection.

## Deferred work

Offline embeddings, document rendering/OCR, file watchers, TypeScript adapters, Git diff/PR analysis, shared metadata export, and a UI are deferred until the evidence-first CLI/MCP workflow is accepted.
