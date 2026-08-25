# Project Brain v1.2: local concurrency model

## English

Each workspace has one local coordinator. MCP and CLI clients use a loopback TCP endpoint with a random local token; they never open an index database for writing. `control.sqlite` stores the active snapshot pointer, coordinator state, reports, feedback, and versioned rule snapshots. Source evidence lives in immutable `snapshot-<id>.sqlite` files.

`stable` (the default) returns the current complete snapshot immediately and starts or joins a background refresh when a remote-default-branch baseline differs. `latest` waits for that merged refresh. A request with `snapshot_id` always opens that exact completed file; a reclaimed ID returns an error instead of silently using newer data.

Refresh copies the active snapshot to `snapshot-<id>.sqlite.staging`, applies the existing incremental indexer there, checkpoints it, renames it to the completed name, then atomically changes `active_snapshot_id` in `control.sqlite`. The previous completed snapshot remains available for in-flight readers. The active and previous snapshots are retained; older files are removed when the platform permits it.

Reports persist the chosen snapshot and rule revision. Feedback creates a new local rule revision and rule-content snapshot, so an in-flight or historical report keeps its original interpretation. On coordinator startup, orphaned `.sqlite.staging` files are removed and the last active snapshot remains queryable.

## 简体中文

每个工作区只有一个本机协调者。MCP 与 CLI 通过“回环地址 + 随机本地令牌”访问它，客户端不会直接写索引库。`control.sqlite` 保存活跃快照指针、协调状态、报告、反馈及按版本保存的规则快照；源码证据只保存在不可变的 `snapshot-<id>.sqlite`。

默认 `stable` 会立即返回当前完整快照；若远程默认分支基线发生变化，则后台创建或合并刷新任务。`latest` 等待该合并任务完成。指定 `snapshot_id` 时只读取该完整快照；已回收的快照会明确报错，绝不会偷偷改读最新版。

刷新从活跃快照复制出 `snapshot-<id>.sqlite.staging`，在其中执行既有增量索引，完成 checkpoint 后改名为完成快照，再通过 `control.sqlite` 的短事务原子切换 `active_snapshot_id`。旧活跃快照会保留给进行中的读请求；默认保留活跃和上一份完成快照，其余快照在系统允许删除时回收。

分析报告会固定记录快照和规则修订号。反馈会创建新的本地规则版本及规则内容快照，因此进行中的分析和历史报告不会被后来反馈改变。协调者重启时会清理遗留 `.sqlite.staging`，并继续使用最后一份活跃快照。
