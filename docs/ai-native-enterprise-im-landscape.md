# AI 原生企业 IM 赛道图谱

日期：2026-06-29

更深入的逐项目介绍见 [ai-native-enterprise-im-project-introductions.md](ai-native-enterprise-im-project-introductions.md)。

## 范围

这份文档追踪更接近“AI 时代企业级 IM”的产品，而不是泛 AI Agent builder。纳入标准是：产品是否改变通信界面本身，包括频道、群聊、消息上下文、共享 Agent 身份、inbox、记忆，以及 Agent 在工作对话中的执行能力。

## 最直接相关的产品

### Glue

Glue 是一个 AI 原生 work chat，定位为 Slack alternative。它的主要特点是结构化、目标导向的 thread，而不是嘈杂频道流，并把 AI 和 MCP 驱动的 workflow 嵌入对话。

值得关注：

- Agentic team chat。
- MCP 作为内置工作/行动层。
- 用结构化 thread 解决 Slack 频道噪音。
- David Sacks 和 Evan Owen 等创始/分发信号较强。

来源：

- https://glue.ai/
- https://www.businesswire.com/news/home/20240514116610/en/Introducing-Glue-Work-Chat-for-the-AI-Era
- https://www.producthunt.com/products/glue-ai

### Raft / Slock

Slock 看起来已经重定向或演进为 Raft。Raft 明确围绕人和 AI Agent 一起建设，Agent 被设计为有记忆、身份、频道、任务、提醒和独立工作上下文的持久参与者。

值得关注：

- Agent 是一等公民。
- 一个 Agent 是一个持久 session，而不是一次性 chatbot 调用。
- Agent 有自己的 inbox、任务和提醒。
- Agent 可通过 daemon 在用户控制的计算机上执行。
- Agent 被作为部分 seat 计费，这对商业模式有启发。

来源：

- https://raft.build/
- https://raft.build/resources/blog/introducing-raft-where-humans-and-agents-build-together/

### FloatIM / Floatboat

FloatIM 是 Floatboat 的 agent-native messaging layer，直接把自己定义为人和 AI Agent 共享群聊的 Agent 原生消息网络。

值得关注：

- Agent 是群聊中的一等成员。
- Agent 通过 Floatboat 本地运行。
- 群规则和权限被结构化为 Agent 可理解的上下文。
- 多 Agent 团队可以形成临时角色并协同工作。
- 有开放协议方向：IACT 用于 interactive agent chat text，Selfware 用于自包含/可演化文件。

来源：

- https://floatboat.ai/floatim
- https://floatboat.ai/blog/introducing-floatim

### Ano

Ano 是面向 AI 原生开发团队的 Slack/Teams/Discord/Mattermost 替代品。它看起来像普通团队聊天，但每个频道都有 Claude Code 类 Agent，并接入 CLI 和 MCP server。

值得关注：

- Developer-first 切口。
- “每个频道一个 Agent”的定位。
- 聊天像 collaborative shell。
- 很适合工程团队：Agent 需要终端、代码、PR 和 MCP 工具。

来源：

- https://ano.chat/
- https://ano.chat/slack-alternative
- https://ano.chat/about

### Den

YC 对 Den 的描述是 AI-native Slack/Notion alternative，面向知识工作者：团队可以创建和协作使用 AI Agent，并用自然语言操作 productivity stack。

值得关注：

- 结合聊天、文档和 AI Agent。
- “Cursor for knowledge workers” 的定位。
- Agent 创建是非工程用户的一等工作流。

来源：

- https://www.ycombinator.com/companies/den
- https://getden.io/

### Tanka

Tanka 是有长期记忆的 AI messenger，面向团队，强调智能回复、待办、洞察，以及跨 WhatsApp、Slack、Telegram、Gmail 的工具记忆。

值得关注：

- memory-first，而不是 agent-first。
- 跨渠道集成接近我们的 assistant-first inbox thesis。
- 不一定替代企业 IM，更像 AI memory messenger。

来源：

- https://www.tanka.ai/
- https://www-old.tanka.ai/
- https://www.producthunt.com/products/tanka
- https://www.prnewswire.com/news-releases/tanka-brings-ai-memory-to-workplace-chat-302377615.html

## 相邻 AI 原生 workspace

### Orchestra

Orchestra 是 chat-centric workspace，整合 chat、channel、call、project、task、doc、media 和 AI Agent。

来源：https://orch.so/

### Vokal

Vokal 是人和 Agent 共同工作的 operating layer，目标用户是已经使用 Codex、Claude Code、Hermes 等多个 Agent 的团队。它提供共享 channel、task、doc、tool、memory 和 knowledge base。

来源：https://vokal.team/

### Play

Play 是 AI-native workspace，帮助团队集中工作、部署 AI coworkers，并在没有工程瓶颈的情况下构建和调整 workflow。

来源：https://play.fast/

### Taskade

Taskade 是 AI-native workspace，包含 chat、task、AI Agent、automation 和 app generation。它更偏 productivity suite，而不是纯 IM。

来源：https://www.taskade.com/compare/free-slack-alternative

## 开源或基础设施型参考

### Pager

Pager 是开源 AI-native Slack alternative，强调 Slack history import、自托管、BYOK、智能搜索和从对话中学习的 AI。

来源：https://pager.team/

### OpenAgents Workspace

OpenAgents 是开源 workspace，人和多个 Agent 可以在共享 thread、文件和浏览器上下文中协作。它更像 “Slack for agents”，但对多 Agent 协作机制很有参考价值。

来源：

- https://github.com/openagents-org/openagents
- https://openagents.org/docs/en/getting-started/overview

### Open Claude Tag

Open Claude Tag 是自托管、channel-native 的 Slack AI teammate，模仿 Claude Tag。它不是新 IM，但捕捉到了“每个频道一个共享 Agent”的需求。

来源：https://github.com/Anil-matcha/open-claude-tag

### Mattermost Agents

Mattermost Agents 把 AI Agent 加入 Mattermost 这个开源 Slack alternative。它不是从零 AI 原生设计，但适合作为自托管企业基线。

来源：

- https://github.com/mattermost/mattermost-plugin-agents
- https://mattermost.com/marketplace/mattermost-agents/

### Integral

Integral 曾是面向高信号团队和社区的 AI 原生通信平台，但活跃开发已经结束。它适合作为一个提醒：AI 原生通信方向有吸引力，但分发、迁移和持续留存很难。

来源：https://integralhq.com/

## 大平台压力

### Slack

Slack 正在把自己重新定位为 AI work platform，包含 Slackbot、Agentforce 和第三方 Agent。

来源：

- https://slack.com/
- https://slack.com/ai-agents

### Claude Tag

Claude Tag 让 Claude 成为 Slack 中共享的频道 Agent。它验证了 channel-native teammate 模式，但仍然在 Slack 内部。

来源：https://www.anthropic.com/news/introducing-claude-tag

### ChatGPT Workspace Agents

OpenAI Workspace Agents 允许团队创建共享 Agent，运行 workflow，在企业控制下云端工作，并可部署到 Slack。

来源：

- https://openai.com/index/introducing-workspace-agents-in-chatgpt/
- https://chatgpt.com/features/workspace-agents/

## 模式总结

这个赛道正在分成几种产品哲学：

1. Agent 原生 IM：Bloome、FloatIM、Raft、Ano、Glue。
2. 带聊天的 AI 原生 workspace：Kylon、Den、Orchestra、Play、Taskade、Dust。
3. 记忆优先 messenger：Tanka。
4. 现有 IM 加 Agent：Slack/Agentforce、Claude Tag、Microsoft Teams agents、Mattermost Agents。
5. 开源协作基座：OpenAgents、Pager、Open Claude Tag，以及类似 Canopy 的 local-first 实验。
6. 边界参考：Teamily。它验证人和 AI 共存，但太社交化，不适合作为企业 IM 的主要对标。

## 战略结论

“Agent 应该像人一样加入群聊”已经不是独特点。更强切口是：

> 跨渠道的 assistant-first work intake，并把 Agent 原生群组协作作为执行场景之一。

多数直接竞品都聚焦共享聊天室。相对较少的产品把用户个人助理设计成默认接收、分流、回复、执行和升级的工作入口，并覆盖 email、外部 IM、日历、任务、文档和企业系统。

因此我们的差异化可以是：

> AI 原生企业 IM：每个人有助理 inbox，每个 Agent 有真实身份和 workspace，所有外部工作渠道都能进入一个有治理的人机协作层。

从竞品里已经浮现出一个重要产品结构：

- 借鉴 Glue：话题/thread 比巨大群时间流更适合作为工作容器。
- 借鉴 Raft：任务是人和 Agent 的持久执行单位。
- 组合方向：进入系统的消息应自动变成话题和任务。话题保存上下文和讨论；任务保存责任人、状态、截止时间、执行历史和 Agent 工作记录。
