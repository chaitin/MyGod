# 市场调研：AI 原生工作平台

日期：2026-06-29

## 调研问题

市场上是否已经有创业项目、成熟产品或开源项目，在探索一种 AI 原生企业协作平台：Agent 可以作为一等工作成员存在，每个用户有个人助理，外部邮件和 IM 可以被统一接入，并由 AI 帮忙处理、执行和升级？

## 简短结论

有，而且方向已经很明确。市场正在从“AI 功能”走向“AI teammate”“AI employee”“AI agents in Slack/Teams”“AI-native inbox”。不过大多数现有产品仍然属于以下三类：

1. 传统协作套件把 Agent 加到现有产品中。
2. AI 助理或 AI employee 产品寄生在 Slack、Teams、邮件或浏览器工作流里。
3. 开源 Agent 框架和 workflow builder 提供运行时、工具调用和集成能力，但不是完整企业协作产品。

真正的机会不是简单“给聊天加 AI”，而是设计一个 Agent 原生工作系统，把身份、权限、记忆、消息入口、任务执行、人类升级和跨渠道集成都作为基础能力。

## 最接近的商业产品

### Junior

Junior 把自己定位为可以加入 Slack 或 Teams 的 “AI employee”，连接多种工具，运行销售、营销和运营工作流。

值得关注：

- 与“Agent 作为同事”的 thesis 高度重合。
- 使用工作消息渠道作为 Agent 的社交和执行界面。
- 更像 AI employee layer，而不是完整协作平台替代品。

来源：https://junior.so/

### Viktor

Viktor 是一个生活在 Slack 和 Microsoft Teams 里的 AI employee，并带有自己的 cloud computer 来完成工作。

值得关注：

- 很接近“Agent 有工作身份”的思路。
- 更偏输出和执行型 Agent，而不是完整通信平台。
- 它的市场叙事很有参考价值：AI 不是工具，而像一个 hire。

来源：https://viktor.com/

### Lindy

Lindy 是 AI executive assistant，重点是 inbox、会议和日程管理。

值得关注：

- 与“每个用户有助理”的方向高度相关。
- 更偏个人生产力和行政助理，不是企业级协作 OS。
- 可以参考它在邮件、日程、会议准备和自动化方面的 UX。

来源：https://www.lindy.ai/

### Dust

Dust 定位为 human-agent collaboration 的多人 AI workspace，整合公司知识、工具、对话和通知。

值得关注：

- 强调人与 Agent 共同工作。
- 更像叠加在公司知识和工具上的 AI workspace。
- 不明显是消息平台替代品。

来源：

- https://dust.tt/
- https://dust.tt/home/slack/slack-integration

### Glean

Glean 从企业搜索扩展到 Work AI platform，提供 assistant、agent、治理、编排和企业上下文。

值得关注：

- 企业上下文和知识层很强。
- 会竞争企业 AI work platform 预算。
- 不是以替代 IM 或聊天为核心。

来源：https://www.glean.com/

### Relevance AI

Relevance AI 是 AI workforce platform，让业务专家构建运行 playbook 的 Agent，并提供 eval、dashboard、RBAC 和监控。

值得关注：

- 可以参考 Agent 治理、评估和运营机制。
- 更像 Agent 团队管理平台，而不是新的沟通平台。
- 对企业权限和控制面很有启发。

来源：

- https://relevanceai.com/
- https://relevanceai.com/docs/get-started/introduction

## 大平台正在进入

### Slack + Agentforce / Slackbot

Slack 正在把自己重新定位为 AI work platform。Salesforce/Slack 把 Agentforce 描述为让 Agent 成为 Slack 中的 teammate，Slackbot 也在向个人 AI agent 演进。

为什么重要：

- 如果产品想替代 Slack 类使用，这是直接竞争压力。
- Slack 的优势是分发、组织图谱和应用生态。
- Slack 的弱点是历史架构仍然是 human-first chat。

来源：

- https://slack.com/
- https://slack.com/blog/news/turn-agents-into-teammates-with-slack
- https://www.salesforce.com/slack/agentforce/

### Anthropic Claude Tag

Claude Tag 是 Slack 中的 Claude 使用形态，团队可以在频道和 thread 中 tag Claude 来委托任务和共享上下文。

为什么重要：

- 验证“AI 进入群组上下文”是明确需求。
- 但仍然偏 @Claude 触发，不是助理先接管工作。
- 说明团队共享 AI 正在成为主流模式。

来源：https://www.anthropic.com/news/introducing-claude-tag

### Microsoft 365 Copilot / Copilot Studio

Microsoft 支持组织创建 Agent 并发布到 Teams 和 Microsoft 365 Copilot 中。Agent 可以自动化任务，或在 Microsoft 365 中代表用户工作。

为什么重要：

- Microsoft 拥有邮件、日历、文档、身份和 Teams 的企业分发。
- 它是企业 AI 分发和合规能力最强的 incumbent 之一。
- 但其 Agent 模式仍然主要嵌入 Microsoft 生态，不是中立跨平台工作层。

来源：

- https://www.microsoft.com/en-us/microsoft-365-copilot
- https://learn.microsoft.com/en-us/microsoft-copilot-studio/publication-add-bot-to-microsoft-teams

### Atlassian Rovo

Rovo Agents 是 Atlassian 产品中的可配置 AI teammate。在 Jira work item 中，Agent 可以作为协作者出现、被分配工作、被提及，或参与 workflow transition。

为什么重要：

- 它提供了一个重要先例：Agent 可以进入 assignee、workflow transition 这类操作字段。
- 更偏工作管理，不是通用通信平台。
- 对“Agent 是工作图谱中的实体”很有参考价值。

来源：

- https://www.atlassian.com/software/rovo
- https://support.atlassian.com/rovo/docs/agents/
- https://support.atlassian.com/rovo/docs/collaborate-with-your-rovo-agent-on-work-items/

### Linear Agents

Linear 支持 Agent 和人类 teammate 一起工作。Issue 可以委托给 Agent，但人类 assignee 仍然保留责任。

为什么重要：

- 它给出一个很好的责任模型：Agent 执行，人类负责。
- 对我们的权限、审批和升级设计有参考价值。

来源：https://linear.app/docs/agents-in-linear

### Workday Sana

Workday Sana 是企业 AI experience platform，连接 Workday 和公司技术栈，并提供企业 workflow Agent。

为什么重要：

- Workday 正从 system of record 走向 AI work execution。
- 在 HR、财务和企业流程中有强势入口。
- 不是新的通信层，但会成为企业 AI workflow 竞争者。

来源：

- https://www.workday.com/en-us/artificial-intelligence/workday-sana.html
- https://www.workday.com/en-us/artificial-intelligence/ai-agents.html

## AI inbox 与助理优先产品

这些产品不是完整企业协作平台，但它们验证了“助理先处理消息”的方向。

### Inbox Zero

开源 AI 邮件助理，可以整理 inbox、预写回复、管理日历，并可通过 Slack 或 Telegram 管理。

来源：https://github.com/elie222/inbox-zero

### Shortwave

AI 原生邮件产品，提供 inbox 整理、搜索、日历支持，以及 Slack、Calendar、Notion、Asana、HubSpot 等集成。

来源：

- https://www.shortwave.com/
- https://www.shortwave.com/docs/guides/ai-assistant/

### Fyxer

Fyxer 是 AI email assistant，用于邮件优先级、回复草拟、会议纪要和排期。

来源：https://www.fyxer.com/

### Missive

Missive 是团队 inbox 和共享邮件协作产品，并加入 AI assistant 功能。它适合作为协作式 inbox workflow 的参考。

来源：

- https://missiveapp.com/
- https://missiveapp.com/ai-assistant

### this+that 和 NOX

这两个产品接近统一 AI inbox：

- this+that：跨邮件、聊天、日历和 DM 的 AI 原生统一 inbox，并带任务和 workflow。
- NOX：macOS 上的 AI 原生消息 inbox，统一 iMessage、WhatsApp、Slack、email 等。

来源：

- https://www.thisandthat.chat/
- https://www.heynox.com/

## 开源与开发者项目

### OpenClaw

OpenClaw 是自托管个人 AI assistant，可以在用户已有渠道中回复，并能运行在用户设备上。

为什么重要：

- 可参考 channel adapter、个人助理控制面和“AI 真正做事”的设计。
- 风险是 Agent 可能获得强大的系统和工具访问能力，安全边界必须严格。

来源：

- https://github.com/openclaw/openclaw
- https://openclaw.ai/

### Hermes Agent

Hermes Agent 是开源自我改进 Agent，带记忆、skills 和消息访问能力。

为什么重要：

- 可参考持久记忆和 skills 体系。
- 更像个人 Agent runtime，不是企业协作产品。

来源：

- https://github.com/nousresearch/hermes-agent
- https://hermes-agent.nousresearch.com/

### SlackAgents

Salesforce AI Research 的 SlackAgents 是在 Slack workspace 中部署多 Agent 的库。

为什么重要：

- 适合参考 Slack 原生多 Agent 协作模式。
- 它是库/框架，不是最终产品。

来源：https://github.com/SalesforceAIResearch/SlackAgents

### Mattermost Agents

Mattermost Agents 是 Mattermost 的 AI 插件，可将 AI 能力集成到 Mattermost workspace，并支持本地或云端 LLM。

为什么重要：

- Mattermost 是真实存在的开源 Slack alternative，因此这是更接近“开源企业协作平台 + AI”的路线。
- 但它仍然更像给既有聊天产品加 AI 插件，而不是从零做 AI 原生设计。

来源：

- https://github.com/mattermost/mattermost-plugin-agents
- https://mattermost.com/marketplace/mattermost-agents/

### Dify、FastGPT、MaxKB、Coze

这些更接近 AI app / Agent builder：

- Dify：开源 agentic workflow、RAG 和应用平台。
- FastGPT：AI Agent 开发平台，带知识库、可视化 workflow、Agent 编排和工具调用。
- MaxKB：面向企业 Agent 的开源平台，支持 RAG、workflow 和 MCP 工具。
- Coze：无代码 AI app 和 Agent 开发平台。

为什么重要：

- 它们可以成为更大工作平台里的 Agent 创建层。
- 但它们本身不解决 Agent 原生企业通信、身份和消息入口问题。

来源：

- https://dify.ai/
- https://github.com/langgenius/dify
- https://doc.fastgpt.io/en/guide/getting-started
- https://maxkb.pro/
- https://www.coze.com/

### MCP、Composio、Arcade、Pipedream

这些是集成和工具访问基础设施：

- MCP：连接 AI 应用与外部系统的开放标准。
- Composio：连接 Agent 与 Gmail、Slack、GitHub、Notion 等应用。
- Arcade：为 Agent 提供安全工具访问。
- Pipedream Connect：为应用和 Agent 提供集成 SDK。

为什么重要：

- 未来产品应该兼容或借鉴 MCP。
- 这些系统能降低集成成本，但不提供最终用户协作界面。

来源：

- https://modelcontextprotocol.io/docs/getting-started/intro
- https://composio.dev/
- https://www.arcade.dev/
- https://pipedream.com/docs/connect

## 中国企业协作平台观察

飞书、钉钉、企业微信等平台都提供机器人能力，但整体仍然更像 bot / integration primitive，而不是把 AI Agent 当成真正的一等工作成员。

观察到的模式：

- 飞书/Lark bot 可以接收消息，但标准群聊使用常围绕 @ 触发；更开放的群消息权限可能需要敏感权限。
- 钉钉有 AI assistant API 和主动发送模式，但仍然是平台特定、权限较重的模型。
- 企业微信支持群机器人和自建应用，但开发表面更接近“机器人集成”，而不是“AI 原生工作成员”。

来源：

- 飞书消息接收文档：https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/reference/im-v1/message/events/receive
- 飞书消息发送文档：https://open.feishu.cn/document/server-docs/im-v1/message/create
- 钉钉 AI assistant 主动发送文档：https://open.dingtalk.com/document/development/ai-assistant-active-sends-messages-mode
- 钉钉接收消息文档：https://open.dingtalk.com/document/group/receive-message
- 企业微信群机器人示例：https://cloud.tencent.com/document/product/1263/71731

## 对我们产品的含义

### 已验证趋势

- “Agent 作为 teammate”正在成为主流方向。
- “每个员工有一个助理”越来越可信。
- 工作会从点菜单、找功能，转向委托、审核、升级和审计。
- Agent 身份、权限、记忆、治理和成本控制是企业核心问题。
- 跨渠道消息入口是有价值的切口，因为邮件、聊天、日历、ticket 和文档仍然碎片化。

### 仍有空间的空白

- 中立的 AI 原生协作平台，不依赖 Slack、Teams、Microsoft 365、Salesforce 或 Atlassian。
- Agent 作为组织成员，拥有 presence、inbox、职责、权限和审计轨迹。
- assistant-first 消息路由：助理先处理常规工作，人只处理升级后的部分。
- 统一工作入口：连接邮件、IM、日历、任务系统和内部应用。
- 人类责任模型：Agent 可以行动，但人类可以审核、批准、覆盖并保留最终责任。

### 竞争风险

最大风险不是方向不成立，而是 incumbent 添加足够多 Agent 功能后降低用户迁移动机。新产品必须有锋利切口。

可能切口：

1. 面向个人和小团队的 AI 原生统一 inbox。
2. Agent 原生群组协作：Agent 驻留频道、观察、总结、跟进和执行。
3. 面向现有 IM/email 的 Agent 身份和权限层。
4. 垂直 AI coworker：销售运营、客服、招聘、IT/helpdesk、工程项目跟进等。

## 初步建议

不要一开始试图整体替代钉钉、飞书、企业微信或 Slack。替换面太大，incumbent 分发优势太强。

更强的第一切口是：

> 一个 AI 原生 work inbox 和 Agent layer：先连接现有 email、IM、calendar，让每个用户有个人助理，并让选定团队 Agent 以明确权限、记忆、审计和升级机制参与共享话题。

这能保留核心 thesis，同时避免第一天重建所有协作基础设施。
