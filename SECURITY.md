# Security policy / 安全政策

## 支持范围

安全修复优先针对最新正式发布版本。旧版本用户应升级后复测。

## 私密报告

请使用 GitHub 的私密漏洞报告入口（仓库 Security → Advisories → Report a vulnerability）；维护者需要在 Settings → Code security 中启用该功能。如果入口不可用，请先建立不含漏洞细节、源码或秘密值的 Issue 请求私密沟通渠道。不要在公开 Issue/PR 中发布可利用细节或凭据。

提供受影响版本、平台、最小合成复现、预期/实际行为及影响范围。删除客户代码、个人信息、本地路径和索引正文。

## 数据边界

PB 本身不上传业务源码；MCP 客户端会接收查询结果，客户端或模型提供商如何处理这些结果由其自身配置和政策决定。索引、源码证据、需求报告和反馈可能包含敏感信息，均应视为私人数据。不要提交本地数据库、原始验收输出、真实需求附件、客户端配置或工作区图导出。

提交前检查当前文件和 Git 历史。删除当前文件不能清除历史中的秘密；泄露凭据应立即撤销/轮换，再评估历史清理。不要把来自第三方私有仓库的代码作为公开测试夹具。

Please use GitHub private vulnerability reporting where available. Never include credentials, customer source, or raw local index data in public reports.
