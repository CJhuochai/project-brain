---
name: project-brain-requirement-analysis
description: 当用户提供需求文档、交互原型、功能改造或二次开发请求时，优先用本地 Project Brain MCP 定位涉及项目、代码证据与影响范围；不用于普通代码问答。
---

# Project Brain 需求分析

如果 `project_brain` MCP 工具可用，先调用 `analyze_requirement`，输入需求中的业务描述、路由、字段或标题。根据候选结果再按需调用 `find_business_context`、`trace_code_path`、`analyze_change_impact`。

输出时区分确定证据和可能关系，说明涉及仓库、入口、关联点与风险；不得将未解析到的动态关系写成事实。代码修改前先给出方案并等待用户确认。

如果 MCP 不可用，简要说明知识库工具未加载，再按普通源码分析流程继续；不要假装已经使用知识库。
