---
name: project-brain-requirement-analysis
description: 当用户提供需求文档、交互原型、功能改造，或本地提交/分支变更时，优先用本地 Project Brain MCP 定位涉及项目、代码证据与影响范围；不用于普通代码问答。
---

# Project Brain 需求分析

如果 `project_brain` MCP 工具可用：有本地需求文档、原型、截图或 Figma JSON 导出时，先调用 `analyze_inputs`，传入文字和本地文件路径；只有文字时调用 `analyze_requirement`。提交号、基线文件或分支变更先调用 `analyze_change`。根据候选结果再按需调用 `get_analysis_report`、`find_business_context`、`trace_code_path`、`analyze_change_impact`。

输出时区分确定证据和可能关系，说明涉及仓库、入口、关联点、建议分支、测试点与风险；不得将未解析到的动态关系写成事实。用户确认或拒绝候选项时，使用 `record_analysis_feedback` 回写本地规则。

PB v2 定位符号后优先调用 `get_symbol_context`，使用返回的符号 ID 和仓库范围，默认 `view: summary`；需要更多证据时再用 `detail` 或返回的展开入口。业务链路用 `trace_business_flow`，跨服务契约用 `list_contracts`。检索结果读取 `evidence` 数组；检查所有结果的 `coverage`、`gaps` 和 `risks`，说明截断、重载歧义、动态关系或解析缺口。`complete` 仅表示当前静态索引在查询预算内完整，不能证明运行时调用或需求覆盖完整。

用户仅请求分析时，不修改代码；已明确授权实施且范围清楚时，完成必要分析后继续实施与验证，不重复等待方案确认。只有关键歧义、需用户决定的取舍或扩大授权范围时才询问；已有授权不替代项目明确要求的发布、合并或其他外部操作审批。

如果 MCP 不可用，简要说明知识库工具未加载，再按普通源码分析流程继续；不要假装已经使用知识库。
