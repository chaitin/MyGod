# MagicChat Mobile

MagicChat 的 Expo / React Native 手机客户端，包含：

- 多服务器管理与 Cookie 会话登录
- 左侧抽屉导航
- 共享页面 Header
- 消息、通讯录、项目三个底部 Tab
- 会话列表、联系人目录和文本消息收发
- WebSocket 实时消息、断线增量同步和后台本地通知
- 官方服务器与自定义 HTTP/HTTPS 服务器
- Tamagui 2 默认组件与明暗主题

## 本地开发

```bash
pnpm install
pnpm start
```

也可以直接启动指定平台：

```bash
pnpm android
pnpm ios
pnpm web
```

## 检查

```bash
pnpm typecheck
pnpm lint
```

Android 13 及以上系统首次登录后会申请通知权限。后台 WebSocket 与本地通知依赖应用进程存活；进程被系统终止后，可靠通知仍需服务端推送通道。
