# AI 时代企业级 IM 与人机协作平台项目介绍

日期：2026-06-29

## 研究范围

这篇文档介绍一批接近“AI 时代的新企业级 IM”的产品和项目。纳入标准不是“用了 AI”这么宽，而是至少触及以下某个核心问题：

- AI Agent 能否作为协作空间里的成员，而不是被动机器人。
- 群聊、频道、线程、任务、文档、工具调用是否围绕 AI 重新设计。
- 每个人是否拥有可代理工作的个人助理。
- 企业消息、邮件、日程、任务和业务系统能否进入统一的 AI 工作入口。
- 是否提供 Agent 的身份、权限、记忆、工具、审计、后台执行和人类升级机制。

这批项目大致分成五类：

- Agent-native IM：Bloome、Glue、Raft、FloatIM、Ano。
- AI-native workspace：Kylon、Den、Orchestra、Vokal、Play、Taskade。
- Assistant / memory-first messenger：Tanka。
- Boundary reference：Teamily。它验证了人和 AI 共存的产品叙事，但过于社交化，不作为我们的核心企业 IM 参考对象。
- 开源或自托管参考：Pager、OpenAgents、Open Claude Tag、Mattermost Agents、Integral。
- 大平台压力：Slack / Agentforce、Claude Tag、ChatGPT Workspace Agents。

## 总体判断

“让 Agent 进入群聊”已经是一个明确赛道，不再是空白机会。Bloome、FloatIM、Raft、Ano、Glue 都在用不同方式重做工作聊天协作；Kylon、Den、Vokal、Orchestra、Play 则从 workspace 和业务执行切入；Slack、OpenAI、Anthropic、Microsoft、Atlassian 这类平台也在把 Agent 加进原有工作流。Teamily 虽然也有人机共存和 AI 原生 IM 叙事，但它明显偏社交网络，不应作为我们设计企业级 IM 的核心对标。

更值得继续探索的差异化，不是再做一个“AI 群聊”，而是：

> Assistant-first enterprise IM：每个用户都有自己的工作助理，外部邮件、IM、日程、任务和企业系统先进入助理工作层；助理能处理、路由、跟进、升级；团队 Agent 可以像成员一样在共享空间中执行任务，并留下权限和审计记录。

换句话说，群聊是执行场景之一，但真正的产品重心应该是“工作入口”和“代理处理权”。

## 横向地图

| 项目 | 最接近的定位 | 核心亮点 | 对我们的启发 |
| --- | --- | --- | --- |
| Teamily | 人 + AI 的社交网络 / AI 原生 IM | 多人多 Agent、长期记忆、社交图谱 | 边界参考：不要走成消费社交网络 |
| Kylon | 企业 Agent 工作台 | Agent 执行业务、工具代理、结构化资源 | Agent 必须能读写工作对象，不只是聊天 |
| Bloome | 多 Agent 群聊 IM | Agent 成员、个人 Agent、Cloud Agent、Agent/Skill 市场 | “Agent 进群”已有强直接竞品 |
| Glue | AI 原生 work chat | 结构化线程、AI 工作流、MCP | 频道噪音问题可以用结构化线程解决 |
| Raft | 人和 Agent 共建 workspace | Agent 有身份、任务、记忆、后台运行 | Agent seat 和人类 seat 可形成新商业模型 |
| FloatIM | Agent-native messaging network | 本地 Agent、群规则、开放协议 | IM 协议层可能需要为 Agent 重新设计 |
| Ano | 开发者团队 AI chat | 每个频道有 coding agent 和 MCP 工具 | 工程团队是很强的早期切口 |
| Den | AI-native Slack/Notion alternative | 聊天 + 文档 + Agent | 知识工作者需要 chat 和 docs 融合 |
| Tanka | 有长期记忆的 AI messenger | 跨 WhatsApp/Slack/Telegram/Gmail 记忆 | 跨渠道记忆接近 assistant-first 入口 |
| Orchestra | chat-centric workspace | 聊天、任务、项目、文档、Agent | IM 可与项目管理深度合并 |
| Vokal | 人和多 Agent 的 operating layer | 面向 Codex/Claude Code 等 Agent 团队 | 多 Agent 协作需要共享任务和知识层 |
| Play | AI-native workspace | AI coworkers + no-code workflow | 非工程团队需要可配置的 AI 工作流 |
| Taskade | AI productivity workspace | 任务、聊天、Agent、自动化 | Suite 化容易，但差异化难 |
| Pager | 开源 AI-native Slack alternative | Slack 导入、自托管、BYOK | 开源/自托管可服务安全敏感团队 |
| OpenAgents | 开源多人多 Agent workspace | 共享线程、文件、浏览器上下文 | 多 Agent 协作机制可借鉴 |
| Open Claude Tag | Slack 频道 AI teammate | 自托管 Claude Tag 替代 | 频道级共享 Agent 是明确需求 |
| Mattermost Agents | Mattermost AI 插件 | 自托管企业 IM + Agent | 插件化是保守企业的迁移方式 |
| Integral | AI-native communication experiment | 高信号社区通信 | 说明“AI 通信”有吸引力但留存难 |
| Slack / Agentforce | 现有 IM 加 Agent | 分发、工作图谱、生态 | 最大压力来自 incumbent |
| Claude Tag | Slack 内共享 Claude | @Claude 共享上下文协作 | 验证 channel-native AI，但仍偏 @ 触发 |
| ChatGPT Workspace Agents | ChatGPT 内企业共享 Agent | Agent 工作流、企业控制、可接 Slack | 通用 Agent 平台会吃掉一部分需求 |

## Teamily（边界参考，不作为核心对标）

### 一句话定位

Teamily 是一个人和 AI Agent 共存的 AI 原生社交网络 / 即时通讯产品，试图把个人、朋友、家庭、社区和工作团队都放进一个 human-AI social network。它不再作为我们的核心对标，因为它的产品气质和增长逻辑明显偏社交化。

### 产品形态

它不是传统意义上的企业 IM，而是更宽的“AI 社交平台”。公开资料显示，Teamily 强调 AI-native instant messaging、personal and social AI Agent OS、human-AI social network。用户可以创建自己的 Agent，把 Agent 加入联系人或群聊，让 Agent 在对话中参与、记忆、总结、建议和执行任务。

这和我们的方向存在关键差异：我们要研究的是企业工作入口、组织权限、任务执行、审计、个人助理接管工作，而不是面向朋友、家庭、社区和内容发现的社交网络。

### 核心能力

- 多人多 Agent 群聊：人和 AI Agent 在同一对话里协作。
- 长期记忆：公开资料强调跨会话、跨群、跨 Agent 的记忆。
- Agent 社交图谱：Agent 不只是工具，而是社交网络中的节点。
- 个人 Agent 队伍：每个人可以拥有多个 Agent，而不是一个通用助手。
- 外部账号连接：公开信息提到 Gmail、X、Slack、GitHub 等连接方向。
- 主动 AI：Agent 可以在上下文中主动建议和推进任务。

### 特色与优势

Teamily 最强的是叙事和野心。它不是在 Slack 上加 AI，而是试图重建“人和 AI 共同生活、共同工作”的网络层。如果它的长期记忆、社交图谱和 Agent 关系链跑通，壁垒会比较深，因为这不是简单功能复制，而是数据、关系和记忆的复合网络效应。

但这些优势更适合消费社交和泛社群产品，不直接服务我们的企业级 IM 目标。

另一个优势是它覆盖消费和工作双场景。消费场景可以带来使用频率，工作场景可以带来付费空间。公开报道也显示它有较强融资和创始团队背景。

### 局限与风险

Teamily 的问题也来自它的宽度。个人社交、家庭、社区、企业工作是完全不同的增长模型和信任模型。企业客户会关心权限、审计、隔离、合规和数据归属；消费用户会关心好玩、易用、隐私和关系密度。两者同时做，产品焦点会很难收敛。

对于我们的方向，Teamily 还不一定是“企业工作入口”。它更像 human-AI social layer，而不是一个明确把邮件、IM、日程、任务和企业系统纳入代理处理流的 work intake 产品。

### 对我们的启发

- 可以借鉴：长期记忆、多 Agent 共存、Agent 身份这些底层概念。
- 不应借鉴：社交 feed、朋友/家庭/社区场景、泛内容发现、消费社交增长模型。
- 结论：Teamily 是边界参考和反例提醒，不是核心竞品。我们的产品必须比 Teamily 更聚焦工作流、权限、审计、责任归属和企业 ROI。

### 来源

- https://teamily.ai/
- https://teamily.ai/pricing
- https://play.google.com/store/apps/details?id=ai.teamily.mobile
- https://eu.36kr.com/en/p/3678646020940422
- https://www.prnewswire.com/apac/news-releases/teamily-ai-officially-enters-the-singapore-market-launching-its-personal-ai-agent-os--redefining-personal-agi-through-self-improving-aihuman-social-network-302781009.html

## Kylon

### 一句话定位

Kylon 是一个企业 Agent 工作台，定位接近“让 AI Agent 团队运行你的业务”。

### 产品形态

Kylon 是明确的 B2B 产品。它把问题定义为：公司的上下文分散在 GPT、Codex、Claude、Slack、Gmail、Notion、GitHub、Figma 等工具里，导致 Agent 无法共享上下文、持续执行和跨系统推进工作。Kylon 的方向是把 Agent、频道、任务、文件、表格、工作流、工具和 API 放进一个共享 workspace。

### 核心能力

- Agent 作为 workspace 成员加入频道。
- Agent 可以拥有任务、处理请求、生成报告、更新工作对象。
- 共享公司上下文和频道记忆。
- 工作区内有结构化资源：文件、表格、workflow、apps、issues 等。
- 大量第三方集成：公开页面列出 GitHub、Slack、Gmail、Salesforce、HubSpot、Linear、Jira、Figma、Stripe、Zoom、Notion、Google Sheets、Calendar、Asana、Shopify、Zendesk、Teams、Outlook、LinkedIn、Dropbox、ClickUp、Airtable、Confluence、Sentry、PostHog 等。
- 开发者接口：docs 中能看到 workspace CLI、gateway、proxy API、tools API、model proxy API、agent/workspace scoped API keys。
- 企业控制：权限、人类审批、审计、日志、角色、数据不训练声明等。

### 特色与优势

Kylon 的优势是它不只做“AI 会说话”，而是让 Agent 进入业务执行层。它理解企业 Agent 的关键不是聊天体验，而是能否读取上下文、调用工具、修改系统、创建任务、更新数据、留下记录，并在权限边界内工作。

它的工具代理和 CLI gateway 也值得注意。如果企业已经有 Codex CLI、Claude Code、内部脚本、API 和业务系统，Kylon 类产品可以成为 Agent runtime 和企业工具之间的中间层。

### 局限与风险

Kylon 的挑战是产品复杂度很高。集成越多，权限、异常处理、可靠性和售后成本越重。它也更像“Agent workspace / execution layer”，未必自然拥有每个普通员工的日常消息入口。

如果用户仍然每天在飞书、钉钉、Slack、微信、Gmail、Outlook 里收消息，Kylon 需要证明自己能成为入口，而不是另一个需要打开的新工作台。

### 对我们的启发

- Agent 必须能读写工作对象，不能只参与聊天。
- 权限、审计、工具调用、人类审批是企业级 AI IM 的核心，不是附加项。
- 我们如果做 assistant-first work intake，需要比 Kylon 更重视“人每天收到的工作请求如何先进入助理”。

### 来源

- https://kylon.io/
- https://docs.kylon.io/introduction
- https://docs.kylon.io/concepts
- https://docs.kylon.io/cli/workspace
- https://docs.kylon.io/proxy/overview
- https://docs.kylon.io/proxy/tools-api

## Bloome

### 一句话定位

Bloome 是一个面向多人多 Agent 协作的 IM 平台，强调人和 AI Agent 在同一个聊天空间里工作。

### 产品形态

Bloome 是最直接接近“Agent 应该像人一样在群里工作”的产品之一。它提供 group chat、DM、threads、replies、@mentions、Agent profile、Agent marketplace、skill marketplace、personal agent、cloud agent 等能力。

### 核心能力

- Agent 是聊天成员，有 profile 和成员列表存在感。
- 多个 Agent 可以加入同一个群聊，互相协作和分工。
- 每个账号有 personal agent。
- Agent 可以在 sandbox 中执行代码、读写文件。
- Cloud Agent 支持远程持续运行。
- 支持连接 Claude Code、Codex、Gemini CLI、OpenCode 等外部 coding agents。
- Agent marketplace：Agent 可浏览、克隆、定制、发布。
- Skill marketplace：通过 SKILL.md 等方式给 Agent 安装能力。
- Blog 中讨论 Agent Collaboration Protocol 和多人多 Agent 记忆设计。

### 特色与优势

Bloome 的优势是产品焦点非常清楚：它就是 agent-first group chat。相比 Kylon，它更像 IM 产品；相比传统 Slack bot，它更重视 Agent 成员身份、多 Agent 协作、技能复用和开发者 Agent 接入。

它对 coding agents 的支持是一个很强的早期切口。工程团队已经习惯让 Codex、Claude Code 这类工具做实际工作，把这些 Agent 带入群聊和任务上下文，需求比较自然。

### 局限与风险

Bloome 仍然可能过于 chat-centric。公开页面中很多交互仍围绕 @mention、群聊和 thread。它很强地解决“Agent 进入聊天”，但不一定完整解决“所有工作先由个人助理接管，助理处理不了再找本人”。

另外，Agent/Skill marketplace 会带来质量、信任、安全和维护问题。企业用户会问：这些 Agent 谁能用，能看什么，能改什么，出错如何追责。

### 对我们的启发

- “Agent 作为群成员”已经有直接竞品。
- 个人 Agent、Cloud Agent、Skill marketplace 都是可借鉴模块。
- 我们要差异化，应该把入口放在 assistant-first intake，而不是只做更强群聊。

### 来源

- https://bloome.im/about
- https://bloome.im/features/ai-agent-platform
- https://bloome.im/features/ai-agents-in-group-chat
- https://bloome.im/features/personal-ai-agent
- https://bloome.im/features/cloud-agent
- https://bloome.im/features/agent-marketplace
- https://bloome.im/features/agent-skill-marketplace
- https://bloome.im/blog/agent-collaboration-protocol
- https://bloome.im/zh-CN/blog/designing-agent-memory-for-multiplayer

## Glue

### 一句话定位

Glue 是一个 AI 原生 work chat，定位为 Slack 替代品，试图用结构化线程和 AI workflow 解决传统团队聊天的噪音问题。

### 产品形态

Glue 的核心不是再造一个频道流，而是把 work chat 改成更结构化的 thread 组织方式。公开资料强调 work chat for the AI era、agentic team chat、AI teammate，以及 MCP-powered workflow。它把聊天、上下文、AI 和工具调用放在同一个工作流里。

### 核心能力

- 团队聊天和结构化线程。
- AI 助手参与对话和工作流。
- MCP 作为连接工具和执行动作的基础。
- 面向团队协作的消息整理、上下文处理和行动推进。

### 特色与优势

Glue 的产品假设很务实：传统 Slack 式频道太吵，真正的工作应该以目标、问题、任务或 thread 为中心。这个角度适合企业，因为很多团队并不缺聊天工具，缺的是“把聊天变成行动，并且不被噪音淹没”。

它的另一个优势是创始和市场声量。公开报道中 Glue 与 David Sacks、Evan Owen 等名字绑定，这会帮助它进入硅谷团队和 AI-native startup 早期市场。

对我们的方向来说，Glue 最值得参考的是“话题作为工作容器”。传统群组只是把一堆人放在一起，消息按时间流动，重要信息很容易被淹没。更合理的结构是：群组表达成员和权限，话题表达具体事务。每个话题可以承载讨论、文件、决策、待办、Agent 行动、状态和总结。

### 局限与风险

Glue 看起来更像“AI 时代的 Slack 优化版”，而不是从个人助理代理入口出发。它可能很好地解决团队讨论结构化，但不一定接管邮件、外部 IM、日程和个人消息流。

如果它仍然要求用户主动进入 Glue workspace 工作，那么它面对的迁移阻力仍然是 Slack/Teams/飞书/钉钉替换难题。

### 对我们的启发

- 传统频道模型确实可能不适合 Agent 协作，结构化 thread 值得重点研究。
- MCP 可能成为企业 AI IM 调用工具的事实标准之一。
- 我们的产品如果做统一入口，也需要解决“消息如何结构化为工作对象”。
- 群组不应是默认工作单元；话题、请求、任务、事件、决策这类可追踪对象才应该是默认工作单元。

### 来源

- https://glue.ai/
- https://www.businesswire.com/news/home/20240514116610/en/Introducing-Glue-Work-Chat-for-the-AI-Era
- https://www.producthunt.com/products/glue-ai

## Raft / Slock

### 一句话定位

Raft 是一个人和 AI Agent 一起工作的 workspace，公开资料显示它延续或替代了早期 Slock 方向，核心是让 Agent 成为持久工作成员。

### 产品形态

Raft 的方向是“where humans and agents build together”。它强调 Agent 不是一次性的 chatbot session，而是有身份、记忆、频道、任务、提醒和工作上下文的长期协作者。

### 核心能力

- Agent 作为 workspace 中的成员存在。
- Agent 有自己的上下文、任务、提醒和消息入口。
- 人和 Agent 在频道里共事。
- Agent 可以通过 daemon 在用户控制的计算环境中执行任务。
- 定价中把 Agent 作为一种 seat 或部分 seat 处理，这对商业模型很有参考价值。

### 特色与优势

Raft 的优势是它把 Agent 当成长期工作主体，而不是把 LLM 调用包装成聊天按钮。这个设定更接近真正的“AI coworker”：Agent 需要持续记忆任务、接收指令、等待事件、主动跟进、在后台运行。

它也较早面对了一个关键问题：Agent 工作到底运行在哪里。通过本地或用户控制环境运行 Agent，可以缓解安全和数据控制问题，也能访问更真实的工具链。

对我们的方向来说，Raft 最值得参考的是 task 模式。AI 原生 IM 不应该只让消息更容易阅读，而应该从消息中提取任务、承诺、截止时间、待跟进事项和风险，再把这些对象放入一个可管理的任务面板。任务随后可以成为 Agent 执行的基本单位：Agent 可以认领、推进、补充上下文、创建草稿、更新系统，或者在需要判断时升级给人。

### 局限与风险

这种模式对用户心智要求较高。普通企业用户可能并不关心 Agent daemon、运行环境和持久 session，他们关心的是工作是否少了、结果是否可靠。Raft 需要把技术强点包装成非常清晰的工作收益。

此外，如果 Agent 作为 seat 收费，客户会自然追问：这个 Agent seat 的产出能否稳定超过人类 seat 的一部分成本。

### 对我们的启发

- Agent 需要持久身份和持久上下文。
- 后台任务能力是 AI 原生 IM 与传统 bot 的关键区别。
- Agent seat / assistant seat 可能是未来企业软件的新计费单位。
- 任务应该是 Agent 执行单位，而不只是人类待办清单。
- 消息自动提取任务是 assistant-first work intake 的核心能力。

### 来源

- https://raft.build/
- https://raft.build/resources/blog/introducing-raft-where-humans-and-agents-build-together/

## FloatIM / Floatboat

### 一句话定位

FloatIM 是 Floatboat 的 agent-native messaging layer，强调人和本地 AI Agent 在同一个消息网络中协作。

### 产品形态

FloatIM 的出发点偏协议和本地 Agent。它不是简单在现有聊天工具里加 bot，而是提出一个更适合 Agent 的 messaging layer：群聊、规则、权限、Agent 角色、临时团队、多 Agent 协作和开放协议。

### 核心能力

- 人和 Agent 在群聊中共同工作。
- Agent 作为一等成员参与消息网络。
- Agent 可通过 Floatboat 在本地运行。
- 群规则和权限被设计为 Agent 可以理解的上下文。
- 多 Agent 可以形成临时团队、分工协调。
- 提出 IACT、Selfware 等协议/文件方向，用于更适合 Agent 的交互和可演化文件。

### 特色与优势

FloatIM 的优势在于它没有停留在产品 UI，而是思考“IM 协议本身是否适合 Agent”。传统 IM 面向人类阅读和回复设计，而 Agent 需要结构化上下文、可执行状态、权限边界、任务状态和可验证输出。

本地 Agent 也是重要方向。很多企业和开发者不愿意把所有上下文送到云端；本地运行和开放协议可以吸引技术型用户。

### 局限与风险

协议和本地化路线对早期市场教育要求高。企业客户通常先购买结果，不会先购买协议。FloatIM 需要证明它的 agent-native messaging 能在实际工作中明显优于 Slack/Discord/飞书 + Agent 插件。

它也可能更适合开发者和 AI power users，进入普通企业协作场景需要更多产品化。

### 对我们的启发

- AI 原生 IM 可能需要新的消息语义，不只是新的 UI。
- 群规则、权限、任务状态应该成为 Agent 可读结构。
- 本地 Agent / 用户控制环境是一个可信任切入点。

### 来源

- https://floatboat.ai/floatim
- https://floatboat.ai/blog/introducing-floatim

## Ano

### 一句话定位

Ano 是面向 AI-native developer teams 的 Slack / Teams / Discord / Mattermost 替代品，主打每个频道都有 coding agent。

### 产品形态

Ano 看起来像团队聊天工具，但它把 Claude Code 类 Agent、CLI、MCP server 和工程工作流放到频道里。它的目标用户不是泛企业，而是开发者团队。

### 核心能力

- 团队频道、聊天和协作。
- 每个频道内置 AI coding agent。
- 支持 MCP 和开发工具连接。
- 面向代码、PR、terminal、repo、issue 等工程工作流。

### 特色与优势

Ano 的优势是切口很清楚。工程团队是最容易接受 AI Agent 的群体，因为他们已经使用 Codex、Claude Code、Cursor、GitHub Copilot、Linear、GitHub Actions 等工具。把 coding agent 放进频道，可以直接解决“代码 Agent 在哪里接需求、交付结果、和人对齐上下文”的问题。

相比泛企业 IM，开发者团队对新工具迁移的阻力也相对低，只要它能提高实际开发效率。

### 局限与风险

Ano 的局限也在于垂直。它很适合 engineering org，但不一定自然覆盖销售、财务、人事、客服、法务等团队。它也可能被 GitHub、Linear、Slack、OpenAI、Anthropic 的原生集成挤压。

### 对我们的启发

- 如果要找早期种子用户，工程团队是强选择。
- “频道 + coding agent + MCP tools”是一个高频、高价值、可验证的场景。
- 但我们的长期方向如果是企业级 IM，需要从工程场景扩展到通用工作入口。

### 来源

- https://ano.chat/
- https://ano.chat/slack-alternative
- https://ano.chat/about

## Den

### 一句话定位

Den 是 YC 项目，定位为 AI-native Slack / Notion alternative，面向知识工作者。

### 产品形态

Den 把聊天、文档和 Agent 放在同一个 workspace 里。公开资料把它描述为一种自然语言接口，让团队可以创建并协作使用 AI Agent。

### 核心能力

- 团队聊天和协作空间。
- 文档/知识工作空间。
- Agent 创建和协作。
- 面向知识工作者的自然语言生产力入口。

### 特色与优势

Den 的关键是把 Slack 和 Notion 两个工作面合并，再用 AI 作为操作层。这个思路合理，因为企业协作中很多信息在聊天里生成，但最后需要沉淀到文档、任务和知识库里。AI 可以承担“从对话到结构化输出”的转换。

YC 背景也说明这个方向在创业生态中有关注度。

### 局限与风险

Den 面对的是两个强 incumbent：Slack/Teams/飞书的消息网络，以及 Notion/Confluence/Google Docs 的文档网络。要同时替换两者很难。

它需要一个极强的楔子，比如某个角色或团队在 Den 里完成一整类工作，而不是只因为“AI-native”就迁移。

### 对我们的启发

- AI 原生 IM 不能只停留在消息，必须把消息转为文档、任务和知识。
- Chat + docs + Agent 是自然组合。
- 但替换套件太难，初期更适合做跨工具入口或垂直工作流。

### 来源

- https://www.ycombinator.com/companies/den
- https://getden.io/

## Tanka

### 一句话定位

Tanka 是一个强调长期记忆的 AI messenger，关注团队聊天中的记忆、回复、待办和跨渠道上下文。

### 产品形态

Tanka 的重点不是多 Agent 群聊，而是“消息应用有记忆”。公开资料显示它支持或强调 WhatsApp、Slack、Telegram、Gmail 等渠道的跨工具记忆，让 AI 能理解长期关系和历史上下文。

### 核心能力

- 团队消息和 AI assistant。
- 长期记忆。
- 智能回复和消息摘要。
- todo / follow-up / insight。
- 跨 WhatsApp、Slack、Telegram、Gmail 等渠道的上下文。

### 特色与优势

Tanka 与我们的 assistant-first 方向很接近，因为它关注的不是替换某个 IM，而是让 AI 理解用户跨渠道的真实沟通历史。对个人助理来说，长期记忆和跨渠道上下文是基础能力。

很多 AI 助理失败，不是模型不会写回复，而是它不知道这个人是谁、这件事之前怎么聊过、什么语气合适、哪些承诺不能忘。Tanka 把记忆作为核心卖点，是正确方向。

### 局限与风险

记忆型产品最难的是信任和边界。用户会担心：哪些消息被记住，谁能读，能否删除，是否会误用，企业能否审计。长期记忆也容易变成噪音，如果召回不准，用户会失去信任。

Tanka 如果只做智能回复和记忆，可能无法覆盖 Agent 后台执行、工具调用和业务对象读写。

### 对我们的启发

- Assistant-first 产品必须有长期记忆，但要从第一天设计权限和可删除性。
- 跨渠道记忆比单一 IM 替代更接近真实用户需求。
- 记忆应该服务于处理工作，而不是只做聊天增强。

### 来源

- https://www.tanka.ai/
- https://www-old.tanka.ai/
- https://www.producthunt.com/products/tanka
- https://www.prnewswire.com/news-releases/tanka-brings-ai-memory-to-workplace-chat-302377615.html

## Orchestra

### 一句话定位

Orchestra 是一个 chat-centric workspace，把聊天、频道、通话、项目、任务、文档、媒体和 AI Agent 放在一起。

### 产品形态

Orchestra 更像新一代 team workspace，而不是纯 IM。它保留聊天作为中心，但把项目管理、任务、文档和 AI 协作纳入同一个工作空间。

### 核心能力

- 团队聊天和频道。
- 项目、任务和文档。
- 通话和媒体协作。
- AI agents 辅助团队工作。

### 特色与优势

Orchestra 的优势是承认企业协作不是只有聊天。实际工作经常从聊天开始，但需要落到项目、任务、文件、会议和交付物。一个 chat-centric workspace 可以减少工具切换。

如果 AI 能在这个 workspace 中理解项目和任务，它就能比单纯聊天机器人更有效。

### 局限与风险

这种产品会天然走向 suite 化，与 Notion、ClickUp、Slack、Teams、飞书等重叠。Suite 化的问题是边界大、替换难、初期很难在每个模块都比现有工具好。

### 对我们的启发

- IM 产品要真正影响工作，必须连接任务和项目。
- 但初期不宜做全套 workspace，应该优先做“消息到任务/行动”的核心链路。

### 来源

- https://orch.so/

## Vokal

### 一句话定位

Vokal 是面向人和多 Agent 的 operating layer，尤其适合已经使用 Codex、Claude Code、Hermes 等 Agent 的团队。

### 产品形态

Vokal 偏向 AI-native team workspace。它将 channels、tasks、docs、tools、memory、knowledge base 等能力放到一起，让人类和多个 Agent 可以共享上下文和交付物。

### 核心能力

- 人和 Agent 的共享频道。
- 任务、文档和知识库。
- 多 Agent 协作和工具使用。
- 支持现有 coding/work agents 的团队协同。

### 特色与优势

Vokal 的优势是抓住了一个新现实：团队已经不只使用一个 AI 助理，而是同时使用 Codex、Claude Code、ChatGPT、Cursor、内部 Agent 等多个系统。问题从“有没有 AI”变成“这些 AI 如何和人共享上下文、分工、交付、避免重复劳动”。

这和我们讨论的“Agent 是组织成员”高度相关。

### 局限与风险

Vokal 可能更适合 AI power users 和工程团队。普通企业还没有到多 Agent 泛滥的阶段，教育成本较高。它也需要处理外部 Agent 的能力差异、权限差异和工作结果归因。

### 对我们的启发

- 多 Agent 协作不是远期问题，已经在工程团队出现。
- 工作空间需要统一任务状态、文件、记忆和工具调用记录。
- 我们的产品可以先支持少量高价值 Agent，而不是一开始开放无限 Agent。

### 来源

- https://vokal.team/

## Play

### 一句话定位

Play 是一个 AI-native workspace，帮助团队集中工作、部署 AI coworkers，并用无代码方式构建/调整 workflow。

### 产品形态

Play 更偏工作流和 AI coworker 平台，而不是纯聊天产品。它的核心是让非工程团队也能用 AI coworkers 和自动化流程推进工作。

### 核心能力

- AI coworkers。
- 工作流构建和自动化。
- 团队协作空间。
- 面向业务团队的无代码配置。

### 特色与优势

Play 的优势是面向非工程团队。很多企业 AI 产品容易从开发者或技术团队开始，但企业预算还大量在销售、运营、财务、HR、客服等团队。无代码 workflow + AI coworker 是这些团队更容易理解的形态。

### 局限与风险

无代码 workflow 平台的竞争非常激烈，容易与 Zapier、Make、Pipedream、Airtable、Notion、Monday、ClickUp、飞书多维表格、钉钉宜搭等方向重叠。Play 需要证明 AI coworker 不是普通 workflow automation 的换皮。

### 对我们的启发

- 企业级 AI IM 如果只服务工程团队，市场会变窄。
- 给普通业务用户的 Agent 配置、审批和监控界面非常重要。
- Assistant-first 入口可以和 workflow builder 结合，但不要一开始做成复杂自动化平台。

### 来源

- https://play.fast/

## Taskade

### 一句话定位

Taskade 是一个 AI productivity workspace，包含任务、项目、文档、聊天、Agent 和自动化。

### 产品形态

Taskade 原本就是团队生产力工具，后来强化 AI agents、AI automation 和 AI workspace。它不是新诞生的 AI IM，但它代表一种 suite 化路线：把团队协作、任务管理和 AI 放进一个产品。

### 核心能力

- 团队任务和项目管理。
- 聊天和协作。
- AI agents。
- 自动化和生成应用。
- 模板和工作流。

### 特色与优势

Taskade 的优势是成熟度和完整度。相比很多早期 AI-native IM，它已经有更多传统生产力功能，适合用户直接管理项目和任务。

### 局限与风险

它的挑战是 AI 原生感可能不够彻底。很多成熟生产力工具都在添加 AI，如果只是 suite + AI，很难形成“AI 时代全新工作方式”的心智。

### 对我们的启发

- Suite 化可以提高留存，但会拉大产品边界。
- 我们要避免一开始做太多模块，先抓住一个 AI 原生链路。

### 来源

- https://www.taskade.com/
- https://www.taskade.com/compare/free-slack-alternative

## Pager

### 一句话定位

Pager 是开源 AI-native Slack alternative，强调自托管、BYOK、Slack 历史导入和 AI 搜索/学习。

### 产品形态

Pager 的方向是给团队一个可迁移、自托管、AI 原生的团队聊天工具。它面向对数据控制敏感、愿意部署开源产品的团队。

### 核心能力

- Slack alternative。
- 开源和自托管。
- Slack 历史导入。
- BYOK。
- AI 搜索和从对话中学习。

### 特色与优势

Pager 的优势是开源和自托管，这对安全敏感团队很重要。AI 原生企业 IM 如果要进入一些技术团队、金融、政府、医疗或大型企业，数据控制会是核心购买理由。

Slack history import 也很关键。任何 IM 替代产品都要面对迁移成本，导入历史是降低迁移阻力的重要工具。

### 局限与风险

开源 Slack alternative 的难点是产品体验和生态。企业 IM 不是只要能聊天，还要移动端、通知、搜索、权限、文件、合规、管理后台、稳定性、第三方集成和迁移工具。

如果 AI 能力不够强，Pager 会被看作“另一个开源 Slack”。

### 对我们的启发

- 自托管和 BYOK 可以成为企业信任卖点。
- 历史消息导入和跨平台迁移能力很重要。
- 开源路线适合技术团队，但需要强产品体验才能扩散。

### 来源

- https://pager.team/

## OpenAgents

### 一句话定位

OpenAgents 是开源多人多 Agent workspace，面向人类和 Agent 在共享线程、文件和浏览器上下文中协作。

### 产品形态

OpenAgents 更像“Slack for agents”或多 Agent 协作实验平台。它不是完整企业 IM，但对研究多 Agent 协作机制很有价值。

### 核心能力

- 多人多 Agent 共享 workspace。
- 共享 threads。
- 共享文件。
- 共享 browser context。
- 面向开发者的开源框架。

### 特色与优势

OpenAgents 的优势是它把多 Agent 协作中的上下文共享问题摆在中心。多个 Agent 如果只通过自然语言聊天协作，很容易出现重复劳动、上下文丢失、状态不一致和不可追踪。共享线程、文件和浏览器上下文是更实际的协作基座。

### 局限与风险

OpenAgents 更像技术框架或实验环境，不是面向普通企业用户的完整产品。企业 IM 需要大量产品化能力：账号、权限、通知、组织结构、移动端、审计、安全、外部集成、管理后台。

### 对我们的启发

- 多 Agent 协作需要共享 artifacts，不只是共享聊天记录。
- 浏览器上下文和文件上下文可能是 Agent 工作空间的重要对象。
- 开源项目可作为机制参考，而不是直接竞品。

### 来源

- https://github.com/openagents-org/openagents
- https://openagents.org/docs/en/getting-started/overview

## Open Claude Tag

### 一句话定位

Open Claude Tag 是一个自托管、频道原生的 Slack AI teammate，模仿 Claude Tag 的使用形态。

### 产品形态

它不是新 IM，而是在 Slack 中提供 channel-native AI teammate。团队可以在频道里和共享 Claude 类 Agent 互动，让 Agent 读取上下文、回答问题、协助任务。

### 核心能力

- Slack 频道内 AI teammate。
- 自托管。
- 共享频道上下文。
- 面向 Claude Tag 类需求的开源替代。

### 特色与优势

它证明了一个需求：团队不是只想每个人单独和 AI 聊，而是想在共享频道里有一个共同的 AI 同事。自托管也适合对数据控制有要求的团队。

### 局限与风险

它仍然是 Slack 插件式方案，受限于 Slack 的平台能力、权限模型和消息形态。它不能从底层重做 IM，也不自然拥有外部邮件、日程和多工具代理入口。

### 对我们的启发

- 频道级共享 Agent 是刚需。
- 自托管版本可以吸引技术团队。
- 但插件方案不是最终形态，平台级身份、权限、记忆和执行层仍然重要。

### 来源

- https://github.com/Anil-matcha/open-claude-tag
- https://www.anthropic.com/news/introducing-claude-tag

## Mattermost Agents

### 一句话定位

Mattermost Agents 是 Mattermost 的 AI Agent 插件，把 AI 能力加入开源企业 IM。

### 产品形态

Mattermost 是成熟的开源 Slack alternative，常见于安全敏感、自托管和 DevOps 场景。Mattermost Agents 在这个基础上提供 AI integration。

### 核心能力

- 在 Mattermost workspace 中使用 AI agents。
- 支持本地或云端 LLM。
- 与既有频道和消息流集成。
- 适合自托管企业环境。

### 特色与优势

Mattermost 的优势不是最新颖，而是企业成熟度和自托管基础。很多新 AI IM 需要从零做组织、权限、移动端、通知、频道、文件、搜索，Mattermost 已经有这些基础。

对保守企业来说，“在现有自托管 IM 上加 AI”可能比迁移到全新 AI-native IM 更容易接受。

### 局限与风险

插件路线很难彻底重构工作方式。它可以把 AI 加进频道，但不一定能让 Agent 成为拥有完整身份、后台任务、跨系统入口和长期记忆的组织成员。

### 对我们的启发

- 自托管企业市场需要认真看 Mattermost。
- 如果我们不想从零做 IM，可以考虑先作为已有 IM 的 AI-native layer。
- 但最终差异化仍在 agent identity、assistant-first intake 和 tool/action layer。

### 来源

- https://github.com/mattermost/mattermost-plugin-agents
- https://mattermost.com/marketplace/mattermost-agents/

## Integral

### 一句话定位

Integral 是一个已停止活跃发展的 AI-native communication platform，曾面向高信号团队和社区通信。

### 产品形态

Integral 试图重做团队/社区沟通，让 AI 帮助提升信号、整理信息和改善协作。它不是当前活跃竞品，但作为失败或停滞案例很值得看。

### 核心能力

- AI-native communication。
- 面向团队和社区。
- 强调高信号交流和信息组织。

### 特色与优势

Integral 的价值在于证明“AI 改造通信”这个方向很早就有人尝试，而且痛点真实：群聊噪音、信息丢失、上下文难找、异步协作效率低。

### 局限与风险

它停止活跃发展说明这个方向很难。通信产品的核心不是功能，而是网络、习惯、迁移成本和日常留存。AI 能力如果不能形成压倒性效率提升，很难让团队离开现有 IM。

### 对我们的启发

- 新 IM 不能只靠“更聪明的信息组织”，必须切入强痛点。
- 要避免泛泛做“高信号沟通”，而要明确节省什么时间、替代什么流程、创造什么可量化收益。

### 来源

- https://integralhq.com/

## Slack / Agentforce

### 一句话定位

Slack 正在从团队聊天工具转向 AI work platform，通过 Slackbot、Agentforce 和第三方 agents 把 AI 加入现有工作网络。

### 产品形态

Slack 的优势是它已经拥有团队频道、组织关系、应用生态、历史上下文和日常使用习惯。Salesforce 的 Agentforce 则把业务 Agent 和 Slack 工作空间结合起来。

### 核心能力

- Slack AI 和 Slackbot。
- Agentforce agents in Slack。
- 第三方 agents 和应用生态。
- 频道、thread、workflow、canvas、huddle、apps 等既有协作能力。

### 特色与优势

Slack 是最大压力之一。它不需要说服用户迁移到新 IM，只要把 AI 加进用户已经工作的地方。对企业来说，这比采用一个全新 AI-native IM 风险更低。

Salesforce 还能把 CRM、销售、客服、营销等业务数据和 Agentforce 连接起来，形成业务闭环。

### 局限与风险

Slack 的弱点是架构历史包袱。频道、thread、bot、workflow 都是 human-first chat 时代的设计。AI Agent 可以被加进去，但未必能从底层获得最自然的身份、权限、记忆、任务和后台执行模型。

### 对我们的启发

- 不能低估 incumbent 分发优势。
- 新产品必须创造“现有 Slack 加 AI 也很难做到”的体验。
- Assistant-first cross-channel intake 可能比单纯替换 Slack 更有机会。

### 来源

- https://slack.com/
- https://slack.com/ai-agents
- https://slack.com/blog/news/turn-agents-into-teammates-with-slack
- https://www.salesforce.com/slack/agentforce/

## Claude Tag

### 一句话定位

Claude Tag 是 Anthropic 的 Slack-native Claude 体验，让团队在 Slack 频道和 thread 中 tag Claude 来共享上下文、委托任务和协作。

### 产品形态

Claude Tag 是典型的“现有 IM + 共享 AI teammate”方案。它不替换 Slack，而是把 Claude 带到 Slack 中，让团队在已有频道里使用同一个 AI。

### 核心能力

- 在 Slack 中 @Claude。
- 基于频道/thread 上下文回答和协作。
- 帮助团队研究、总结、写作和推进任务。
- 面向团队共享，而不是每个人各用各的 Claude。

### 特色与优势

Claude Tag 验证了“AI 应该进入群聊上下文”这个方向。团队共享 AI 的价值在于：所有人能看到同一个问题、同一个上下文、同一个输出，也能一起纠正和推进。

### 局限与风险

它仍然高度依赖 @mention，AI 大多是被动触发。它没有从底层解决 Agent 身份、个人助理先处理、跨渠道消息入口、后台任务和企业级行动权限。

### 对我们的启发

- 共享频道 AI 是确定需求。
- 但下一步应该是从 @AI 走向“AI 主动处理和升级”。
- 我们要设计 AI 何时观察、何时沉默、何时主动发言、何时需要批准。

### 来源

- https://www.anthropic.com/news/introducing-claude-tag

## ChatGPT Workspace Agents

### 一句话定位

ChatGPT Workspace Agents 是 OpenAI 面向团队和企业的共享 Agent 能力，允许组织创建、运行和管理工作流 Agent，并可部署到 Slack 等环境。

### 产品形态

它不是企业 IM，但会成为企业 AI 工作流的强平台。团队可以在 ChatGPT 内创建共享 Agent，让 Agent 在云端运行任务，连接工具，并在企业控制下协作。

### 核心能力

- 企业 workspace 内共享 Agent。
- Agent 可运行 workflow。
- 云端执行。
- 企业级控制和管理。
- 可与 Slack 等现有工作环境连接。

### 特色与优势

OpenAI 的优势是模型能力、用户心智和企业入口。很多团队已经在 ChatGPT 中工作，如果 Workspace Agents 能连接工具和 Slack，它会覆盖一部分“企业 Agent 平台”的需求。

### 局限与风险

它不是通信产品本身。ChatGPT 作为中心工作台很强，但用户的真实工作入口仍分散在邮件、IM、日程、任务和业务系统里。它能否成为 daily work inbox，还需要看产品形态。

### 对我们的启发

- 通用 Agent 平台会持续变强，不能把壁垒建立在“能创建 Agent”本身。
- 我们要把重点放在工作入口、消息代理、团队协作语义和企业权限。
- 与 ChatGPT / OpenAI Agent 生态兼容可能比对抗更现实。

### 来源

- https://openai.com/index/introducing-workspace-agents-in-chatgpt/
- https://chatgpt.com/features/workspace-agents/

## 结论：这些项目留下的机会

### 已经拥挤的方向

以下方向已经有明显竞品，不能作为唯一卖点：

- Agent 可以进群聊。
- 在 Slack/Teams 中 @AI。
- 用 AI 总结频道和 thread。
- 创建自定义 Agent。
- Agent marketplace / skill marketplace。
- AI workspace with tasks and docs。
- 开源 Slack alternative with AI。

### 仍然有空间的方向

更值得探索的是这些组合能力：

- 每个用户有个人工作助理，默认先接收和处理消息。
- 助理可以跨 email、IM、calendar、tasks、docs、CRM、ticket 等渠道工作。
- 助理能判断：直接处理、草拟给用户确认、转交团队 Agent、升级给本人。
- Agent 和人一样有组织身份、职责、权限、记忆、收件箱和审计记录。
- 群聊不是中心，而是 Agent 和人协作处理工作的一个场景。
- 工作对象是结构化的：请求、任务、承诺、风险、决策、待审批、外部联系人。
- 所有出站动作有可配置批准策略：自动、需确认、只草稿、禁止。

### 推荐定位

推荐把产品定义为：

> AI-native enterprise work inbox：接管企业工作消息入口，让每个员工的个人助理先处理跨渠道工作请求；团队 Agent 作为真实成员在共享空间中执行任务；所有行动都有权限、记忆、审计和人类升级机制。

更短的表达：

> 让工作先找你的 AI 助理，而不是先打断你。

这个定位避开了单纯 AI 群聊的拥挤竞争，也避开了直接替换飞书、钉钉、Slack 的高迁移成本。它保留了我们的核心 thesis：AI 时代，企业 IM 不应该只是人找人，而应该是工作先进入智能代理层，再由代理决定是否需要人。
