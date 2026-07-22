import { useEffect, useState } from 'react';
import {
  ArrowRight,
  Bot,
  Check,
  FolderKanban,
  Github,
  Image,
  ListTodo,
  MessageCircle,
  Mic,
  MoreHorizontal,
  Paperclip,
  Plus,
  Search,
  Send,
  Users,
} from 'lucide-react';

type DemoKey = 'chat' | 'agent' | 'project';

const demoTabs: Array<{ id: DemoKey; label: string }> = [
  { id: 'chat', label: '团队会话' },
  { id: 'agent', label: 'AI 应用' },
  { id: 'project', label: '项目任务' },
];

const productUrl = 'https://chat.chaitin.net/';
const repositoryUrl = 'https://github.com/duke-yeah/MagicChat';
const logoUrl = `${import.meta.env.BASE_URL}favicon.webp`;

const workflowSteps = [
  {
    number: '01',
    title: '团队在会话中提出目标',
    description:
      '从熟悉的私聊或群聊开始。讨论背景、文件、图片和语音都留在同一处，成为后续行动需要的上下文。',
  },
  {
    number: '02',
    title: 'AI 应用进入协作现场',
    description:
      '通过独立会话或加入群聊，AI 应用和成员共享当前语境。团队可以 @AI 获取帮助，也能继续追问与修正。',
  },
  {
    number: '03',
    title: '关键结论落到项目与任务',
    description:
      '群聊关联项目，任务明确负责人、优先级、状态和截止日期。下一步不再散落在消息历史里。',
  },
  {
    number: '04',
    title: '需要确认时，带着上下文发起请求',
    description:
      '理想的 AI 协作会先整理信息，再将原因、建议和所需动作交给对应负责人；确认结果继续回到任务，形成可追踪的闭环。',
  },
];

const pageStyles = String.raw`
  .im-home {
    --ink: #17211d;
    --soft-ink: #39463f;
    --muted: #6d7872;
    --paper: #f4f5ef;
    --surface: #fff;
    --line: rgba(23, 33, 29, .12);
    --green: #35d07f;
    --green-light: #79efb1;
    --green-pale: #ddf7e7;
    --green-dark: #176b46;
    --violet: #7765ec;
    --orange: #ff9d54;
    min-width: 320px;
    overflow-x: clip;
    background:
      radial-gradient(circle at 8% 3%, rgba(121, 239, 177, .22), transparent 27rem),
      radial-gradient(circle at 94% 18%, rgba(119, 101, 236, .1), transparent 30rem),
      var(--paper);
    color: var(--ink);
    font-family: Inter, 'SF Pro Display', 'PingFang SC', 'Microsoft YaHei', system-ui, sans-serif;
  }
  .im-home *, .im-home *::before, .im-home *::after { box-sizing: border-box; }
  .im-container { width: min(calc(100% - 48px), 1180px); margin-inline: auto; }
  .im-nav {
    position: sticky; top: 0; z-index: 50;
    border-bottom: 1px solid var(--line);
    background: rgba(244, 245, 239, .76);
    backdrop-filter: blur(18px) saturate(150%);
  }
  .im-nav-inner { min-height: 76px; display: flex; align-items: center; gap: 34px; }
  .im-brand { display: inline-flex; align-items: center; gap: 11px; text-decoration: none; font-size: 20px; font-weight: 800; letter-spacing: -.04em; }
  .im-logo { display: block; width: 35px; height: 35px; flex: 0 0 auto; border-radius: 11px; object-fit: cover; }
  .im-links { display: flex; align-items: center; gap: 30px; margin-left: auto; }
  .im-links a { color: var(--muted); font-size: 14px; font-weight: 650; text-decoration: none; transition: color .2s; }
  .im-links a:hover { color: var(--ink); }
  .im-button {
    display: inline-flex; min-height: 52px; align-items: center; justify-content: center; gap: 9px;
    padding: 0 23px; border: 1px solid transparent; border-radius: 999px;
    background: var(--ink); color: white; font-size: 14px; font-weight: 750; text-decoration: none;
    box-shadow: 0 14px 30px rgba(23, 33, 29, .15); transition: transform .2s, box-shadow .2s;
  }
  .im-button:hover { transform: translateY(-2px); box-shadow: 0 18px 36px rgba(23, 33, 29, .21); }
  .im-button.small { min-height: 42px; padding-inline: 18px; box-shadow: none; }
  .im-button.light { border-color: rgba(23, 33, 29, .2); background: rgba(255,255,255,.48); color: var(--ink); box-shadow: none; }
  .im-button-icon { display: grid; width: 23px; height: 23px; place-items: center; border-radius: 50%; background: var(--green); color: var(--ink); }
  .im-hero { position: relative; padding: 92px 0 86px; }
  .im-hero::after { position: absolute; top: 10%; left: 53%; z-index: 0; width: 520px; height: 520px; content: ''; border-radius: 50%; background: rgba(121,239,177,.18); filter: blur(80px); }
  .im-hero-grid { position: relative; z-index: 1; display: grid; grid-template-columns: .9fr 1.1fr; align-items: center; gap: 58px; }
  .im-hero-grid > * { min-width: 0; }
  .im-kicker { display: inline-flex; align-items: center; gap: 9px; margin-bottom: 24px; padding: 7px 12px 7px 9px; border: 1px solid var(--line); border-radius: 999px; background: rgba(255,255,255,.55); color: var(--soft-ink); font-size: 11px; font-weight: 800; letter-spacing: .1em; }
  .im-kicker-dot { width: 9px; height: 9px; border-radius: 50%; background: var(--green); box-shadow: 0 0 0 5px rgba(53,208,127,.14); }
  .im-title { max-width: 670px; margin: 0 0 26px; font-size: clamp(48px, 4.4vw, 64px); font-weight: 820; letter-spacing: -.067em; line-height: 1.04; }
  .im-title-line { display: block; white-space: nowrap; }
  .im-highlight { position: relative; z-index: 0; white-space: nowrap; }
  .im-highlight::after { position: absolute; right: -.02em; bottom: .08em; left: -.02em; z-index: -1; height: .27em; content: ''; border-radius: 999px 20px; background: var(--green-light); transform: rotate(-1.5deg); }
  .im-hero-copy { max-width: 580px; color: var(--muted); font-size: 17px; line-height: 1.9; }
  .im-hero-copy strong { color: var(--ink); }
  .im-actions { display: flex; flex-wrap: wrap; gap: 13px; margin-top: 34px; }
  .im-checks { display: flex; flex-wrap: wrap; gap: 18px; margin-top: 28px; color: var(--muted); font-size: 12px; }
  .im-checks span { display: inline-flex; align-items: center; gap: 7px; }
  .im-check-icon { display: grid; width: 18px; height: 18px; place-items: center; border-radius: 50%; background: var(--green-pale); color: var(--green-dark); }
  .im-app-wrap { position: relative; perspective: 1400px; }
  .im-app-wrap::before { position: absolute; right: -22px; bottom: -27px; width: 72%; height: 80%; content: ''; border-radius: 38px; background: rgba(53,208,127,.2); transform: rotate(5deg); }
  .im-app {
    position: relative; display: grid; grid-template-columns: 62px 170px minmax(300px,1fr); min-height: 500px;
    overflow: hidden; border: 1px solid var(--line); border-radius: 26px; background: white;
    box-shadow: 0 32px 90px rgba(29,48,38,.17); transform: rotateY(-4deg) rotateX(1.5deg);
  }
  .im-rail { display: flex; flex-direction: column; align-items: center; gap: 17px; padding: 15px 0; background: #1c2923; color: white; }
  .im-rail-logo { display: block; width: 34px; height: 34px; margin-bottom: 7px; border-radius: 10px; object-fit: cover; }
  .im-rail-item { display: grid; width: 35px; height: 35px; place-items: center; border-radius: 10px; color: rgba(255,255,255,.48); }
  .im-rail-item.active { position: relative; background: rgba(255,255,255,.11); color: var(--green-light); }
  .im-rail-item.active::before { position: absolute; left: -13px; width: 3px; height: 19px; content: ''; border-radius: 3px; background: var(--green); }
  .im-user { display: grid; width: 32px; height: 32px; margin-top: auto; place-items: center; border: 2px solid rgba(255,255,255,.2); border-radius: 50%; background: #ffe1c2; color: #7d4b26; font-size: 10px; font-weight: 850; }
  .im-side { padding: 17px 12px; border-right: 1px solid var(--line); background: #f7f8f4; }
  .im-team { padding: 0 7px 13px; font-size: 12px; font-weight: 800; }
  .im-search { display: flex; align-items: center; gap: 6px; margin-bottom: 14px; padding: 8px 9px; border: 1px solid var(--line); border-radius: 8px; background: white; color: #99a29d; font-size: 9px; }
  .im-side-label { display: flex; justify-content: space-between; margin: 15px 7px 6px; color: #8a958f; font-size: 8px; font-weight: 850; letter-spacing: .1em; }
  .im-conversation { display: flex; align-items: center; gap: 8px; margin-bottom: 3px; padding: 7px; border-radius: 9px; font-size: 9px; font-weight: 650; }
  .im-conversation.active { background: var(--green-pale); color: var(--green-dark); }
  .im-mini-avatar { position: relative; display: grid; flex: 0 0 auto; width: 24px; height: 24px; place-items: center; border-radius: 8px; background: #e8e3ff; color: #5844c6; font-size: 8px; font-weight: 850; }
  .im-mini-avatar.green { background: #d9f4e2; color: #27754b; }
  .im-mini-avatar.orange { background: #ffead7; color: #a35822; }
  .im-mini-avatar::after { position: absolute; right: -2px; bottom: -2px; width: 7px; height: 7px; content: ''; border: 2px solid #f7f8f4; border-radius: 50%; background: var(--green); }
  .im-chat { display: flex; min-width: 0; flex-direction: column; background: white; }
  .im-chat-head { display: flex; min-height: 59px; align-items: center; justify-content: space-between; padding: 11px 15px; border-bottom: 1px solid var(--line); }
  .im-chat-head h3 { margin: 0; font-size: 11px; }
  .im-chat-head p { margin: 1px 0 0; color: var(--muted); font-size: 7px; }
  .im-head-avatars { display: flex; }
  .im-head-avatars span { display: grid; width: 23px; height: 23px; margin-left: -6px; place-items: center; border: 2px solid white; border-radius: 50%; background: #eceeed; font-size: 6px; font-weight: 850; }
  .im-messages { display: flex; flex: 1; flex-direction: column; gap: 13px; padding: 17px 15px 12px; }
  .im-message { display: grid; grid-template-columns: 26px 1fr; gap: 8px; }
  .im-message-avatar { display: grid; width: 26px; height: 26px; place-items: center; border-radius: 8px; background: #ece8ff; color: #5b47c9; font-size: 7px; font-weight: 850; }
  .im-message-avatar.ai { background: var(--ink); color: var(--green-light); }
  .im-message-meta { display: flex; align-items: center; gap: 6px; margin-bottom: 3px; font-size: 7px; font-weight: 800; }
  .im-message-time { color: #a3aba7; font-weight: 500; }
  .im-ai-badge { padding: 1px 5px; border-radius: 4px; background: var(--green-pale); color: var(--green-dark); font-size: 5px; letter-spacing: .06em; }
  .im-message-copy { color: #4e5953; font-size: 8px; line-height: 1.55; }
  .im-task { margin-top: 8px; overflow: hidden; border: 1px solid var(--line); border-radius: 11px; background: #fbfcfa; box-shadow: 0 5px 14px rgba(31,50,40,.05); }
  .im-task-top { display: flex; align-items: center; justify-content: space-between; padding: 8px 9px 6px; }
  .im-task-kicker { color: var(--violet); font-size: 6px; font-weight: 850; letter-spacing: .08em; }
  .im-status { padding: 2px 6px; border-radius: 999px; background: #fff0df; color: #a2581d; font-size: 5px; font-weight: 850; }
  .im-task-title { padding: 0 9px 7px; font-size: 8px; font-weight: 800; }
  .im-task-info { display: flex; gap: 10px; padding: 7px 9px; border-top: 1px solid var(--line); color: #7b8580; font-size: 6px; }
  .im-compose { margin: 0 15px 13px; padding: 9px 10px; border: 1px solid var(--line); border-radius: 10px; background: #fafbf9; color: #9ba49f; font-size: 7px; }
  .im-compose-tools { display: flex; align-items: center; gap: 7px; margin-top: 8px; }
  .im-send { display: grid; width: 21px; height: 21px; margin-left: auto; place-items: center; border-radius: 7px; background: var(--ink); color: var(--green-light); }
  .im-float { position: absolute; right: -22px; bottom: 56px; display: flex; align-items: center; gap: 9px; padding: 10px 13px; border: 1px solid var(--line); border-radius: 13px; background: rgba(255,255,255,.94); box-shadow: 0 12px 30px rgba(31,50,40,.1); font-size: 9px; font-weight: 800; animation: im-float 4s ease-in-out infinite; }
  .im-pulse { width: 8px; height: 8px; border-radius: 50%; background: var(--green); box-shadow: 0 0 0 6px rgba(53,208,127,.13); }
  @keyframes im-float { 50% { transform: translateY(-8px); } }
  .im-strip { border-block: 1px solid var(--line); background: rgba(255,255,255,.35); }
  .im-strip-inner { display: flex; min-height: 72px; align-items: center; justify-content: space-between; gap: 24px; overflow: hidden; font-size: 12px; font-weight: 700; }
  .im-strip-label { color: var(--ink); font-weight: 850; white-space: nowrap; }
  .im-strip-items { display: flex; gap: 18px; color: var(--muted); white-space: nowrap; }
  .im-strip-items span { display: inline-flex; align-items: center; gap: 18px; }
  .im-strip-items span:not(:last-child)::after { width: 5px; height: 5px; content: ''; border-radius: 50%; background: var(--green); }
  .im-section { padding: 118px 0; }
  .im-section-kicker { margin-bottom: 16px; color: var(--green-dark); font-size: 11px; font-weight: 850; letter-spacing: .14em; }
  .im-heading { max-width: 780px; margin: 0; font-size: clamp(36px,4.6vw,58px); font-weight: 820; letter-spacing: -.055em; line-height: 1.12; }
  .im-heading.im-statement-title, .im-heading.im-capability-title { max-width: none; font-size: clamp(32px,2.8vw,40px); font-weight: 780; letter-spacing: -.05em; line-height: 1.15; }
  .im-heading.im-statement-title { white-space: nowrap; }
  .im-statement-title span { position: relative; z-index: 0; color: var(--green-dark); }
  .im-statement-title span::after { position: absolute; right: -.08em; bottom: -.02em; left: -.08em; z-index: -1; height: .28em; content: ''; border-radius: 999px; background: var(--green-light); opacity: .72; }
  .im-intro { max-width: 650px; margin: 19px 0 0; color: var(--muted); font-size: 16px; line-height: 1.85; }
  .im-statement { display: grid; grid-template-columns: .9fr 1.1fr; align-items: end; gap: 78px; }
  .im-quote { position: relative; margin: 0; padding-left: 27px; color: var(--soft-ink); font-size: 21px; font-weight: 650; line-height: 1.65; }
  .im-quote::before { position: absolute; top: 7px; bottom: 7px; left: 0; width: 4px; content: ''; border-radius: 4px; background: var(--green); }
  .im-demo { margin-top: 60px; overflow: hidden; border-radius: 36px; background: var(--ink); box-shadow: 0 32px 80px rgba(29,48,38,.16); }
  .im-demo-tabs { display: flex; gap: 7px; padding: 18px; border-bottom: 1px solid rgba(255,255,255,.1); }
  .im-demo-tab { padding: 10px 15px; border: 0; border-radius: 999px; background: transparent; color: rgba(255,255,255,.53); font-size: 12px; font-weight: 750; cursor: pointer; }
  .im-demo-tab.active { background: var(--green); color: var(--ink); }
  .im-demo-panel { display: grid; grid-template-columns: .78fr 1.22fr; min-height: 430px; }
  .im-demo-copy { display: flex; flex-direction: column; justify-content: center; padding: 56px; color: white; }
  .im-demo-index { margin-bottom: 22px; color: var(--green-light); font-size: 11px; font-weight: 850; letter-spacing: .1em; }
  .im-demo-copy h3 { max-width: 400px; margin: 0 0 17px; font-size: 33px; letter-spacing: -.045em; line-height: 1.17; }
  .im-demo-copy p { max-width: 420px; margin: 0; color: rgba(255,255,255,.6); font-size: 14px; line-height: 1.85; }
  .im-pills { display: flex; flex-wrap: wrap; gap: 7px; margin-top: 25px; }
  .im-pills span { padding: 6px 9px; border: 1px solid rgba(255,255,255,.13); border-radius: 999px; color: rgba(255,255,255,.72); font-size: 10px; }
  .im-demo-visual { display: flex; align-items: center; justify-content: center; padding: 44px; background: linear-gradient(rgba(255,255,255,.04) 1px,transparent 1px), linear-gradient(90deg,rgba(255,255,255,.04) 1px,transparent 1px), #223129; background-size: 32px 32px; }
  .im-visual-card { width: min(100%,520px); overflow: hidden; border: 1px solid rgba(255,255,255,.16); border-radius: 20px; background: rgba(255,255,255,.96); box-shadow: 0 22px 55px rgba(0,0,0,.24); color: var(--ink); }
  .im-visual-head { display: flex; align-items: center; justify-content: space-between; padding: 16px 19px; border-bottom: 1px solid var(--line); font-size: 11px; font-weight: 800; }
  .im-visual-body { padding: 19px; }
  .im-topic { display: flex; gap: 10px; margin-bottom: 13px; }
  .im-topic-avatar { display: grid; flex: 0 0 auto; width: 31px; height: 31px; place-items: center; border-radius: 10px; background: var(--green-pale); color: var(--green-dark); font-size: 10px; font-weight: 850; }
  .im-topic-copy { flex: 1; padding: 10px 12px; border-radius: 0 12px 12px; background: #f1f3ee; color: #526058; font-size: 10px; line-height: 1.65; }
  .im-topic-copy b { display: block; margin-bottom: 2px; color: var(--ink); }
  .im-agent-grid { display: grid; grid-template-columns: repeat(2,1fr); gap: 11px; }
  .im-agent-card { padding: 14px; border: 1px solid var(--line); border-radius: 14px; background: #fafbf9; }
  .im-agent-head { display: flex; align-items: center; gap: 8px; margin-bottom: 10px; }
  .im-agent-face { display: grid; width: 33px; height: 33px; place-items: center; border-radius: 10px; background: var(--ink); color: var(--green-light); font-size: 9px; font-weight: 850; }
  .im-agent-card h4 { margin: 0; font-size: 10px; }
  .im-agent-card small { color: var(--muted); font-size: 7px; }
  .im-agent-card p { margin: 0; color: var(--muted); font-size: 8px; line-height: 1.6; }
  .im-board { display: grid; grid-template-columns: repeat(3,1fr); gap: 8px; }
  .im-board-col { padding: 9px; border-radius: 12px; background: #f2f4ef; }
  .im-board-title { display: flex; justify-content: space-between; margin-bottom: 9px; color: var(--muted); font-size: 7px; font-weight: 850; }
  .im-board-task { margin-bottom: 7px; padding: 9px; border: 1px solid var(--line); border-radius: 9px; background: white; font-size: 8px; font-weight: 750; }
  .im-board-task i { display: block; width: 33px; height: 3px; margin: 7px 0; border-radius: 9px; background: var(--orange); }
  .im-board-meta { display: flex; justify-content: space-between; color: var(--muted); font-size: 6px; font-weight: 500; }
  .im-capabilities { background: rgba(255,255,255,.45); }
  .im-section-head { display: flex; align-items: end; justify-content: space-between; gap: 48px; margin-bottom: 52px; }
  .im-section-head .im-intro { max-width: 440px; margin: 0; }
  .im-bento { display: grid; grid-template-columns: repeat(12,1fr); gap: 17px; }
  .im-card { position: relative; min-height: 220px; overflow: hidden; padding: 30px; border: 1px solid var(--line); border-radius: 23px; background: rgba(255,255,255,.7); transition: transform .25s, box-shadow .25s; }
  .im-card:hover { transform: translateY(-4px); box-shadow: 0 15px 36px rgba(31,50,40,.08); }
  .im-card.wide { grid-column: span 7; }
  .im-card.medium { grid-column: span 5; }
  .im-card.third { grid-column: span 4; }
  .im-card.dark { border-color: transparent; background: var(--ink); color: white; }
  .im-card.green { border-color: transparent; background: var(--green-pale); }
  .im-card-number { display: inline-flex; margin-bottom: 40px; padding: 5px 8px; border: 1px solid var(--line); border-radius: 999px; color: var(--muted); font-size: 9px; font-weight: 850; letter-spacing: .09em; }
  .im-card.dark .im-card-number { border-color: rgba(255,255,255,.14); color: rgba(255,255,255,.55); }
  .im-card h3 { max-width: 520px; margin: 0 0 11px; font-size: 24px; letter-spacing: -.04em; line-height: 1.25; }
  .im-card p { max-width: 510px; margin: 0; color: var(--muted); font-size: 13px; line-height: 1.8; }
  .im-card.dark p { color: rgba(255,255,255,.57); }
  .im-card-pills { display: flex; flex-wrap: wrap; gap: 7px; margin-top: 22px; }
  .im-card-pills span { padding: 6px 9px; border: 1px solid var(--line); border-radius: 8px; background: rgba(255,255,255,.45); color: var(--soft-ink); font-size: 10px; font-weight: 650; }
  .im-card.dark .im-card-pills span { border-color: rgba(255,255,255,.12); background: rgba(255,255,255,.05); color: rgba(255,255,255,.7); }
  .im-reveal { opacity: 0; transform: translateY(24px); transition: opacity .72s cubic-bezier(.2,.7,.2,1), transform .72s cubic-bezier(.2,.7,.2,1); }
  .im-reveal.is-visible { opacity: 1; transform: translateY(0); }
  .im-workflow-grid { display: grid; grid-template-columns: .72fr 1.28fr; gap: 90px; margin-top: 62px; }
  .im-workflow-lead { position: sticky; top: 115px; align-self: start; padding: 32px; border: 1px solid var(--line); border-radius: 22px; background: rgba(255,255,255,.58); }
  .im-workflow-lead.im-reveal.is-visible { transform: translate3d(0,var(--workflow-shift,0),0); transition: opacity .72s cubic-bezier(.2,.7,.2,1), transform .16s linear, box-shadow .25s ease; will-change: transform; }
  .im-workflow-lead.im-reveal.is-visible:hover { box-shadow: 0 18px 42px rgba(31,50,40,.09); }
  .im-workflow-lead h3 { margin: 0 0 15px; font-size: 26px; letter-spacing: -.04em; }
  .im-workflow-lead p { margin: 0; color: var(--muted); font-size: 13px; line-height: 1.8; }
  .im-logic { display: flex; flex-wrap: wrap; align-items: center; gap: 7px; margin-top: 23px; font-size: 10px; font-weight: 800; }
  .im-logic span { padding: 6px 8px; border-radius: 7px; background: var(--green-pale); }
  .im-steps { position: relative; }
  .im-steps::before { position: absolute; top: 26px; bottom: 26px; left: 26px; width: 1px; content: ''; background: linear-gradient(var(--green),var(--violet),var(--orange)); }
  .im-step { position: relative; display: grid; grid-template-columns: 54px 1fr; gap: 21px; padding-bottom: 50px; }
  .im-step:last-child { padding-bottom: 0; }
  .im-step-number { z-index: 1; display: grid; width: 54px; height: 54px; place-items: center; border: 1px solid var(--line); border-radius: 17px; background: white; box-shadow: 0 10px 25px rgba(31,50,40,.07); color: var(--green-dark); font-size: 11px; font-weight: 850; }
  .im-step-copy { padding-top: 5px; }
  .im-step-copy h3 { margin: 0 0 8px; font-size: 20px; letter-spacing: -.03em; }
  .im-step-copy p { max-width: 620px; margin: 0; color: var(--muted); font-size: 13px; line-height: 1.8; }
  .im-principle { display: grid; grid-template-columns: 1fr .9fr; gap: 68px; overflow: hidden; padding: 68px; border-radius: 36px; background: var(--ink); color: white; }
  .im-principle .im-section-kicker { color: var(--green-light); }
  .im-principle h2 { max-width: 620px; margin: 0 0 22px; font-size: clamp(32px,3.6vw,44px); letter-spacing: -.05em; line-height: 1.14; }
  .im-principle > div > p { max-width: 560px; margin: 0; color: rgba(255,255,255,.6); font-size: 15px; line-height: 1.85; }
  .im-decision { align-self: center; padding: 21px; border: 1px solid rgba(255,255,255,.15); border-radius: 21px; background: rgba(255,255,255,.08); }
  .im-decision-head { display: flex; justify-content: space-between; margin-bottom: 13px; color: rgba(255,255,255,.58); font-size: 9px; font-weight: 800; }
  .im-decision-badge { padding: 4px 7px; border-radius: 999px; background: rgba(255,157,84,.15); color: #ffb77e; }
  .im-decision h3 { margin: 0 0 18px; font-size: 19px; }
  .im-question { display: grid; grid-template-columns: 22px 1fr; gap: 8px; margin-top: 8px; padding: 9px 10px; border-radius: 10px; background: rgba(255,255,255,.06); color: rgba(255,255,255,.68); font-size: 10px; line-height: 1.55; }
  .im-question > span { display: grid; width: 22px; height: 22px; place-items: center; border-radius: 7px; background: rgba(121,239,177,.12); color: var(--green-light); font-size: 8px; font-weight: 850; }
  .im-footer { background: var(--ink); color: white; }
  .im-footer-closing { display: grid; grid-template-columns: 1fr auto; align-items: center; gap: 46px; padding: 54px 0 42px; border-bottom: 1px solid rgba(255,255,255,.1); }
  .im-footer-closing h2 { max-width: 660px; margin: 0; font-size: clamp(27px,3vw,36px); font-weight: 760; letter-spacing: -.045em; line-height: 1.18; }
  .im-footer-closing p { max-width: 620px; margin: 12px 0 0; color: rgba(255,255,255,.52); font-size: 13px; line-height: 1.75; }
  .im-footer-closing .im-button { min-height: 44px; padding-inline: 18px; background: var(--green); color: var(--ink); box-shadow: none; font-size: 12px; }
  .im-footer-closing .im-button-icon { width: 20px; height: 20px; background: var(--ink); color: var(--green-light); }
  .im-footer-inner { display: flex; min-height: 112px; align-items: center; justify-content: space-between; gap: 28px; }
  .im-footer-brand { display: flex; align-items: center; gap: 10px; font-weight: 800; }
  .im-footer-note { margin: 0; color: rgba(255,255,255,.48); font-size: 11px; }
  @media (max-width: 980px) {
    .im-links { display: none; }
    .im-button.small { margin-left: auto; }
    .im-hero-grid, .im-statement, .im-workflow-grid, .im-principle, .im-footer-closing { grid-template-columns: minmax(0, 1fr); }
    .im-hero-grid { gap: 60px; }
    .im-app { transform: none; }
    .im-statement { gap: 34px; }
    .im-demo-panel { grid-template-columns: 1fr; }
    .im-demo-visual { min-height: 410px; }
    .im-section-head { display: block; }
    .im-section-head .im-intro { max-width: 650px; margin-top: 20px; }
    .im-card.wide, .im-card.medium, .im-card.third { grid-column: span 6; }
    .im-workflow-grid { gap: 48px; }
    .im-workflow-lead { position: static; }
    .im-workflow-lead.im-reveal.is-visible { transform: none; }
    .im-principle { gap: 45px; }
    .im-decision { max-width: 560px; }
    .im-footer-closing { align-items: start; }
  }
  @media (max-width: 680px) {
    .im-container { width: min(calc(100% - 30px),1180px); }
    .im-nav-inner { min-height: 66px; }
    .im-brand { font-size: 18px; }
    .im-logo { width: 31px; height: 31px; border-radius: 9px; }
    .im-button.small { min-height: 38px; padding-inline: 14px; font-size: 11px; }
    .im-button.small .im-button-icon { display: none; }
    .im-hero { padding: 52px 0 65px; }
    .im-title { font-size: clamp(39px,11vw,43px); letter-spacing: -.06em; }
    .im-hero-copy { font-size: 15px; }
    .im-actions { flex-direction: column; }
    .im-actions .im-button { width: 100%; }
    .im-checks { gap: 10px 15px; }
    .im-app-wrap { height: 300px; margin-inline: 0; }
    .im-app-wrap::before, .im-float { display: none; }
    .im-app { position: absolute; top: 0; left: 50%; width: 588px; min-height: 450px; grid-template-columns: 48px 118px minmax(260px,1fr); margin-left: -179px; border-radius: 18px; transform: scale(.61); transform-origin: top left; }
    .im-strip-inner { min-height: 62px; }
    .im-strip-label { display: none; }
    .im-strip-items { overflow-x: auto; }
    .im-section { padding: 86px 0; }
    .im-heading { font-size: 37px; }
    .im-heading.im-statement-title, .im-heading.im-capability-title { font-size: clamp(23px,7.4vw,29px); }
    .im-intro { font-size: 14px; }
    .im-quote { font-size: 18px; }
    .im-demo { margin-top: 38px; border-radius: 24px; }
    .im-demo-tabs { overflow-x: auto; }
    .im-demo-tab { white-space: nowrap; }
    .im-demo-copy { padding: 37px 25px; }
    .im-demo-copy h3 { font-size: 28px; }
    .im-demo-visual { min-height: 340px; padding: 22px 14px; }
    .im-bento { display: block; }
    .im-card { min-height: 235px; margin-bottom: 14px; padding: 26px; }
    .im-card-number { margin-bottom: 33px; }
    .im-workflow-grid { margin-top: 44px; }
    .im-workflow-lead { padding: 26px; }
    .im-step { grid-template-columns: 46px 1fr; gap: 16px; }
    .im-steps::before { left: 22px; }
    .im-step-number { width: 46px; height: 46px; border-radius: 14px; }
    .im-principle { padding: 40px 24px; border-radius: 25px; }
    .im-principle h2 { font-size: 32px; }
    .im-footer-closing { gap: 26px; padding: 44px 0 34px; }
    .im-footer-closing h2 { font-size: 28px; }
    .im-footer-closing .im-button { width: 100%; }
    .im-footer-inner { align-items: flex-start; flex-direction: column; justify-content: center; gap: 7px; }
  }
  @media (prefers-reduced-motion: reduce) {
    .im-home *, .im-home *::before, .im-home *::after { scroll-behavior: auto !important; animation-duration: .01ms !important; transition-duration: .01ms !important; }
    .im-reveal { opacity: 1; transform: none; }
  }
`;

function ProductPreview() {
  return (
    <div className="im-app-wrap" aria-label="即应产品界面示意图">
      <div className="im-app">
        <aside className="im-rail" aria-hidden="true">
          <img className="im-rail-logo" src={logoUrl} alt="" />
          <div className="im-rail-item active">
            <MessageCircle size={15} />
          </div>
          <div className="im-rail-item">
            <Users size={15} />
          </div>
          <div className="im-rail-item">
            <FolderKanban size={15} />
          </div>
          <div className="im-user">YL</div>
        </aside>

        <aside className="im-side" aria-hidden="true">
          <div className="im-team">即应 · 产品团队</div>
          <div className="im-search">
            <Search size={10} /> 搜索消息与联系人
          </div>
          <div className="im-side-label">
            <span>会话</span>
            <Plus size={10} />
          </div>
          <div className="im-conversation active">
            <span className="im-mini-avatar green">版</span>版本发布协作
          </div>
          <div className="im-conversation">
            <span className="im-mini-avatar orange">体</span>产品体验优化
          </div>
          <div className="im-conversation">
            <span className="im-mini-avatar">研</span>协作体验调研组
          </div>
          <div className="im-side-label">
            <span>AI 应用</span>
            <Plus size={10} />
          </div>
          <div className="im-conversation">
            <span className="im-mini-avatar green">AI</span>项目助理
          </div>
          <div className="im-conversation">
            <span className="im-mini-avatar orange">AI</span>知识助手
          </div>
        </aside>

        <div className="im-chat" aria-hidden="true">
          <div className="im-chat-head">
            <div>
              <h3># 版本发布协作</h3>
              <p>已关联项目 · 即应 Web 2.6</p>
            </div>
            <div className="im-head-avatars">
              <span>唐</span>
              <span>周</span>
              <span>AI</span>
            </div>
          </div>
          <div className="im-messages">
            <div className="im-message">
              <span className="im-message-avatar">唐</span>
              <div>
                <div className="im-message-meta">
                  唐宁 <span className="im-message-time">09:42</span>
                </div>
                <div className="im-message-copy">
                  本周五准备发布新版本，大家把剩余事项同步到这里。
                </div>
              </div>
            </div>
            <div className="im-message">
              <span className="im-message-avatar ai">AI</span>
              <div>
                <div className="im-message-meta">
                  项目助理 <span className="im-ai-badge">AI 应用</span>
                  <span className="im-message-time">09:43</span>
                </div>
                <div className="im-message-copy">
                  我已根据当前讨论整理发布前检查项，并创建一项待确认任务。
                </div>
                <div className="im-task">
                  <div className="im-task-top">
                    <span className="im-task-kicker">TASK · RELEASE</span>
                    <span className="im-status">待确认</span>
                  </div>
                  <div className="im-task-title">
                    完成发布前回归测试与风险确认
                  </div>
                  <div className="im-task-info">
                    <span>负责人 周乔</span>
                    <span>高优先级</span>
                    <span>周五截止</span>
                  </div>
                </div>
              </div>
            </div>
            <div className="im-message">
              <span
                className="im-message-avatar"
                style={{ background: '#ffe6d0', color: '#9b541f' }}
              >
                周
              </span>
              <div>
                <div className="im-message-meta">
                  周乔 <span className="im-message-time">09:48</span>
                </div>
                <div className="im-message-copy">
                  收到，我来负责，今天下班前先回传第一轮结果。
                </div>
              </div>
            </div>
          </div>
          <div className="im-compose">
            输入消息，或 @AI 应用一起推进…
            <div className="im-compose-tools">
              <Paperclip size={9} />
              <Image size={9} />
              <Mic size={9} />
              <span className="im-send">
                <Send size={9} />
              </span>
            </div>
          </div>
        </div>
      </div>
      <div className="im-float">
        <span className="im-pulse" />
        任务卡片已同步到项目
      </div>
    </div>
  );
}

function ChatDemo() {
  return (
    <div className="im-visual-card">
      <div className="im-visual-head">
        <span># 产品体验优化 · 话题</span>
        <MoreHorizontal size={16} />
      </div>
      <div className="im-visual-body">
        <div className="im-topic">
          <span className="im-topic-avatar">陈</span>
          <div className="im-topic-copy">
            <b>陈墨 · 10:18</b>
            移动端消息列表的层级还可以再清晰一些，我补了一张标注图。
          </div>
        </div>
        <div className="im-topic">
          <span
            className="im-topic-avatar"
            style={{ background: '#e9e4ff', color: '#5746b6' }}
          >
            林
          </span>
          <div className="im-topic-copy">
            <b>林夏 · 10:23</b>
            收到。我们就在这条消息下继续讨论，避免打断主会话。
          </div>
        </div>
        <div className="im-topic-copy" style={{ marginTop: 18 }}>
          回复当前话题…
        </div>
      </div>
    </div>
  );
}

function AgentDemo() {
  const agents = [
    [
      '项目助理',
      '团队可见 · 在线',
      '整理讨论、生成任务卡片，帮助项目保持同步。',
    ],
    ['知识助手', '全员可见 · 在线', '在会话上下文中检索并归纳团队知识。'],
    ['运营助手', '运营组可见', '参与运营群聊，协助整理活动执行信息。'],
    ['接入新应用', '自定义名称与范围', '让更多企业能力，以成员身份进入协作。'],
  ];

  return (
    <div className="im-visual-card">
      <div className="im-visual-head">
        <span>团队 AI 应用</span>
        <span style={{ color: '#176b46' }}>＋ 创建应用</span>
      </div>
      <div className="im-visual-body im-agent-grid">
        {agents.map(([name, meta, description], index) => (
          <div className="im-agent-card" key={name}>
            <div className="im-agent-head">
              <span
                className="im-agent-face"
                style={
                  index === 1
                    ? { background: '#5b49ca', color: 'white' }
                    : index === 2
                      ? { background: '#b65d20', color: 'white' }
                      : undefined
                }
              >
                {index === 3 ? '+' : 'AI'}
              </span>
              <span>
                <h4>{name}</h4>
                <small>{meta}</small>
              </span>
            </div>
            <p>{description}</p>
          </div>
        ))}
      </div>
    </div>
  );
}

function ProjectDemo() {
  return (
    <div className="im-visual-card">
      <div className="im-visual-head">
        <span>即应 Web 2.6 · 任务视图</span>
        <span style={{ color: '#176b46' }}>＋ 新建任务</span>
      </div>
      <div className="im-visual-body im-board">
        <div className="im-board-col">
          <div className="im-board-title">
            <span>待处理</span>
            <span>2</span>
          </div>
          <div className="im-board-task">
            补齐上线说明
            <i />
            <span className="im-board-meta">
              <b>中</b>
              <span>周三</span>
            </span>
          </div>
          <div className="im-board-task">
            检查多端下载页
            <i style={{ background: '#7765ec' }} />
            <span className="im-board-meta">
              <b>低</b>
              <span>周四</span>
            </span>
          </div>
        </div>
        <div className="im-board-col">
          <div className="im-board-title">
            <span>进行中</span>
            <span>1</span>
          </div>
          <div className="im-board-task">
            发布前回归测试
            <i style={{ background: '#e8665d' }} />
            <span className="im-board-meta">
              <b>高</b>
              <span>周五</span>
            </span>
          </div>
        </div>
        <div className="im-board-col">
          <div className="im-board-title">
            <span>已完成</span>
            <span>2</span>
          </div>
          <div className="im-board-task">
            确认版本范围
            <i style={{ background: '#35d07f' }} />
            <span className="im-board-meta">
              <b>完成</b>
              <span>今天</span>
            </span>
          </div>
        </div>
      </div>
    </div>
  );
}

function DemoPanel({ active }: { active: DemoKey }) {
  const content = {
    chat: {
      index: '01 · CONVERSATION',
      title: '让每一种沟通，都有合适的容器',
      description:
        '从一对一沟通到公开群组，从主会话到独立话题；文字、Markdown、文件、图片、语音和业务卡片，都在熟悉的消息体验里自然流动。',
      pills: ['私聊与群聊', '公开群', '话题讨论', '多类型消息'],
      visual: <ChatDemo />,
    },
    agent: {
      index: '02 · AI APPLICATION',
      title: 'AI 有身份，也有自己的协作位置',
      description:
        '创建企业 AI 应用，为它设置名称、头像、说明与可见范围，再把它加入私聊或群聊。团队不用离开沟通现场，就能和 AI 一起处理信息。',
      pills: ['独立应用身份', '可见范围', '群聊成员', '接入凭据'],
      visual: <AgentDemo />,
    },
    project: {
      index: '03 · PROJECT & TASK',
      title: '让讨论有去处，让进展有状态',
      description:
        '会话可以关联协作项目，群成员获得一致的项目上下文；任务进一步明确负责人、优先级、标签、状态、截止日期与提醒。',
      pills: ['项目关联群聊', '成员授权', '任务状态', '到期提醒'],
      visual: <ProjectDemo />,
    },
  }[active];

  return (
    <div className="im-demo-panel" role="tabpanel">
      <div className="im-demo-copy">
        <div className="im-demo-index">{content.index}</div>
        <h3>{content.title}</h3>
        <p>{content.description}</p>
        <div className="im-pills">
          {content.pills.map((pill) => (
            <span key={pill}>{pill}</span>
          ))}
        </div>
      </div>
      <div className="im-demo-visual">{content.visual}</div>
    </div>
  );
}

export default function HomePage() {
  const [activeDemo, setActiveDemo] = useState<DemoKey>('chat');

  useEffect(() => {
    const previousTitle = document.title;
    document.title = '即应 · 企业协作空间';

    return () => {
      document.title = previousTitle;
    };
  }, []);

  useEffect(() => {
    const elements = Array.from(
      document.querySelectorAll<HTMLElement>('.im-reveal')
    );

    if (!('IntersectionObserver' in window)) {
      elements.forEach((element) => element.classList.add('is-visible'));
      return undefined;
    }

    const observer = new IntersectionObserver(
      (entries) => {
        entries.forEach((entry) => {
          if (entry.isIntersecting) {
            entry.target.classList.add('is-visible');
            observer.unobserve(entry.target);
          }
        });
      },
      { threshold: 0.12, rootMargin: '0px 0px -30px' }
    );

    elements.forEach((element) => observer.observe(element));
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    const section = document.querySelector<HTMLElement>('#workflow');
    const lead = section?.querySelector<HTMLElement>('.im-workflow-lead');
    const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)');

    if (!section || !lead) return undefined;

    let frame = 0;
    const updatePosition = () => {
      frame = 0;

      if (window.innerWidth <= 980 || reduceMotion.matches) {
        lead.style.removeProperty('--workflow-shift');
        return;
      }

      const bounds = section.getBoundingClientRect();
      const distance = Math.max(bounds.height - window.innerHeight * 0.35, 1);
      const progress = Math.min(
        1,
        Math.max(0, (window.innerHeight * 0.62 - bounds.top) / distance)
      );
      const shift = (progress - 0.5) * 24;

      lead.style.setProperty('--workflow-shift', `${shift.toFixed(2)}px`);
    };

    const requestUpdate = () => {
      if (!frame) frame = window.requestAnimationFrame(updatePosition);
    };

    updatePosition();
    window.addEventListener('scroll', requestUpdate, { passive: true });
    window.addEventListener('resize', requestUpdate);
    reduceMotion.addEventListener('change', requestUpdate);

    return () => {
      if (frame) window.cancelAnimationFrame(frame);
      window.removeEventListener('scroll', requestUpdate);
      window.removeEventListener('resize', requestUpdate);
      reduceMotion.removeEventListener('change', requestUpdate);
      lead.style.removeProperty('--workflow-shift');
    };
  }, []);

  return (
    <div className="im-home">
      <style>{pageStyles}</style>

      <nav className="im-nav" aria-label="主导航">
        <div className="im-container im-nav-inner">
          <a className="im-brand" href="#top" aria-label="即应首页">
            <img className="im-logo" src={logoUrl} alt="" />
            <span>即应</span>
          </a>
          <div className="im-links">
            <a href="#product">产品体验</a>
            <a href="#capabilities">核心能力</a>
            <a href="#workflow">协作方式</a>
          </div>
          <a
            className="im-button small"
            href={repositoryUrl}
            target="_blank"
            rel="noreferrer"
          >
            GitHub
            <span className="im-button-icon">
              <Github size={13} />
            </span>
          </a>
        </div>
      </nav>

      <header className="im-hero" id="top">
        <div className="im-container im-hero-grid">
          <div>
            <div className="im-kicker">
              <span className="im-kicker-dot" /> A BETTER WAY TO WORK
            </div>
            <h1 className="im-title">
              <span className="im-title-line">从沟通到行动</span>
              <span className="im-highlight im-title-line">让协作持续向前</span>
            </h1>
            <p className="im-hero-copy">
              即应是一款面向企业团队的沟通与协作平台。它把
              <strong>聊天、AI 应用、项目与任务</strong>
              放进同一个上下文，让沟通不止被看见，更能继续向前。
            </p>
            <div className="im-actions">
              <a
                className="im-button"
                href={productUrl}
                target="_blank"
                rel="noreferrer"
              >
                在线体验
                <span className="im-button-icon">
                  <ArrowRight size={14} />
                </span>
              </a>
              <a
                className="im-button light"
                href={repositoryUrl}
                target="_blank"
                rel="noreferrer"
              >
                查看源码 <Github size={15} />
              </a>
            </div>
            <div className="im-checks">
              {['人与 AI 同场沟通', '项目任务自然衔接', '开源与多端覆盖'].map(
                (item) => (
                  <span key={item}>
                    <i className="im-check-icon">
                      <Check size={11} strokeWidth={3} />
                    </i>
                    {item}
                  </span>
                )
              )}
            </div>
          </div>
          <ProductPreview />
        </div>
      </header>

      <div className="im-strip" aria-label="产品能力概览">
        <div className="im-container im-strip-inner">
          <span className="im-strip-label">从沟通，到协同，再到推进</span>
          <div className="im-strip-items">
            {[
              '企业聊天',
              'AI 应用',
              '通讯录',
              '项目协作',
              '任务管理',
              '多端通知',
            ].map((item) => (
              <span key={item}>{item}</span>
            ))}
          </div>
        </div>
      </div>

      <section className="im-section" id="product">
        <div className="im-container">
          <div className="im-statement">
            <div>
              <div className="im-section-kicker">WHY JIYING</div>
              <h2 className="im-heading im-statement-title">
                不只是一个有 <span>AI</span> 的聊天框
              </h2>
            </div>
            <p className="im-quote">
              即应让 AI 以<strong>协作成员</strong>
              的身份参与会话，并由项目与任务承接讨论中形成的共识。沟通有上下文，行动有负责人，进展也始终清晰可见。
            </p>
          </div>

          <div className="im-demo">
            <div
              className="im-demo-tabs"
              role="tablist"
              aria-label="产品能力演示"
            >
              {demoTabs.map((tab) => (
                <button
                  className={`im-demo-tab ${activeDemo === tab.id ? 'active' : ''}`}
                  key={tab.id}
                  type="button"
                  role="tab"
                  aria-selected={activeDemo === tab.id}
                  onClick={() => setActiveDemo(tab.id)}
                >
                  {tab.label}
                </button>
              ))}
            </div>
            <DemoPanel active={activeDemo} />
          </div>
        </div>
      </section>

      <section className="im-section im-capabilities" id="capabilities">
        <div className="im-container">
          <div className="im-section-head">
            <div>
              <div className="im-section-kicker">CAPABILITIES</div>
              <h2 className="im-heading im-capability-title">
                从找到协作对象，到把事情持续推进
              </h2>
            </div>
            <p className="im-intro">
              在通讯录中找到团队成员、应用或公开群，进入会话后邀请 AI
              参与，再将讨论关联到项目并落实为任务。每一步自然衔接，也始终共享同一份上下文。
            </p>
          </div>

          <div className="im-bento">
            <article className="im-card medium green">
              <span className="im-card-number">01 · CONTACT</span>
              <h3>先找到一起协作的成员与应用</h3>
              <p>
                通讯录统一展示联系人、内置应用、团队应用和公开群组。可以发起私聊或应用会话，也可以直接加入公开群。
              </p>
              <Users size={38} color="#176b46" style={{ marginTop: 24 }} />
            </article>

            <article className="im-card wide dark">
              <span className="im-card-number">02 · CHAT</span>
              <h3>从会话开始，让讨论保持聚焦</h3>
              <p>
                支持私聊、群聊、公开群与话题。通过
                Markdown、链接、卡片、文件、图片和语音共享信息，重要讨论还可独立成话题继续展开。
              </p>
              <div className="im-card-pills">
                {['私聊与群聊', '公开群', '话题讨论', '多类型消息'].map(
                  (item) => (
                    <span key={item}>{item}</span>
                  )
                )}
              </div>
            </article>

            <article className="im-card third">
              <span className="im-card-number">03 · AI</span>
              <h3>邀请 AI 应用参与会话</h3>
              <p>
                AI
                应用拥有独立的名称、头像、说明和访问范围，可单独对话，也能作为成员加入群聊，参与当前讨论。
              </p>
              <Bot size={34} color="#7765ec" style={{ marginTop: 22 }} />
            </article>

            <article className="im-card third">
              <span className="im-card-number">04 · PROJECT</span>
              <h3>把相关会话归入同一项目</h3>
              <p>
                创建项目并关联一个或多个群聊，群成员随之获得项目访问权限，让讨论、成员与后续任务共享同一项目背景。
              </p>
              <FolderKanban
                size={34}
                color="#176b46"
                style={{ marginTop: 22 }}
              />
            </article>

            <article className="im-card third">
              <span className="im-card-number">05 · TASK</span>
              <h3>把讨论落实为明确任务</h3>
              <p>
                为任务设置负责人、优先级、标签、状态和截止时间；需要推进时，可向会话发送任务卡片，并按时提醒当前负责人。
              </p>
              <ListTodo size={34} color="#ff9d54" style={{ marginTop: 22 }} />
            </article>
          </div>
        </div>
      </section>

      <section className="im-section" id="workflow">
        <div className="im-container">
          <div className="im-reveal">
            <div className="im-section-kicker">HOW IT FLOWS</div>
            <h2 className="im-heading">从一句话开始，到一件事完成</h2>
            <p className="im-intro">
              真正有效的 AI
              协作，不是制造更多消息，而是理解上下文、参与推进，并在需要确认或决策时给出明确请求。
            </p>
          </div>

          <div className="im-workflow-grid">
            <aside className="im-workflow-lead im-reveal">
              <h3>沟通不再是终点</h3>
              <p>
                聊天保留团队沟通中的信任与讨论；AI
                应用帮助处理信息；项目与任务把关键结论沉淀为持续可追踪的工作。
              </p>
              <div className="im-logic">
                <span>消息</span>→<span>协同</span>→<span>任务</span>→
                <span>结果</span>
              </div>
            </aside>

            <div className="im-steps">
              {workflowSteps.map((step, index) => (
                <article
                  className="im-step im-reveal"
                  key={step.number}
                  style={{ transitionDelay: `${index * 70}ms` }}
                >
                  <span className="im-step-number">{step.number}</span>
                  <div className="im-step-copy">
                    <h3>{step.title}</h3>
                    <p>{step.description}</p>
                  </div>
                </article>
              ))}
            </div>
          </div>
        </div>
      </section>

      <section className="im-section" style={{ paddingTop: 10 }}>
        <div className="im-container">
          <div className="im-principle">
            <div>
              <div className="im-section-kicker">A BETTER INTERRUPTION</div>
              <h2>好的 AI 协作，知道什么时候不打扰你</h2>
              <p>
                未来的企业协作不需要更多通知，而需要更有准备的通知。每次 AI
                请求人类介入，都应该带着足够上下文，让人快速判断并继续推进。
              </p>
            </div>
            <aside className="im-decision" aria-label="AI 人类介入请求示例">
              <div className="im-decision-head">
                <span>AI 介入请求</span>
                <span className="im-decision-badge">需要确认</span>
              </div>
              <h3>发布范围存在一项待确认风险</h3>
              {[
                ['为什么现在找我？', '回归测试发现权限策略与发布范围不一致。'],
                ['为什么是我？', '你是当前版本的项目负责人。'],
                ['我需要做什么？', '确认缩小范围，或授权按原计划继续。'],
                ['不处理会怎样？', '发布任务将保持待确认状态。'],
              ].map(([question, answer], index) => (
                <div className="im-question" key={question}>
                  <span>{String(index + 1).padStart(2, '0')}</span>
                  <div>
                    <strong>{question}</strong>
                    <br />
                    {answer}
                  </div>
                </div>
              ))}
            </aside>
          </div>
        </div>
      </section>

      <footer className="im-footer">
        <div className="im-container im-footer-closing">
          <div>
            <h2>下一条消息，也可以是工作的下一步</h2>
            <p>
              打开即应，让团队成员、AI 应用、项目和任务在同一个协作空间里相遇。
            </p>
          </div>
          <a
            className="im-button"
            href={repositoryUrl}
            target="_blank"
            rel="noreferrer"
          >
            查看开源项目
            <span className="im-button-icon">
              <ArrowRight size={14} />
            </span>
          </a>
        </div>
        <div className="im-container im-footer-inner">
          <div className="im-footer-brand">
            <img className="im-logo" src={logoUrl} alt="" />
            <span>即应 · 企业协作空间</span>
          </div>
          <p className="im-footer-note">面向企业团队的沟通与协作平台</p>
        </div>
      </footer>
    </div>
  );
}
