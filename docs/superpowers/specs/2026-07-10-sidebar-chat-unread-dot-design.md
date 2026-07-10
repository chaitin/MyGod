# 侧边栏聊天未读红点设计

日期：2026-07-10

## 目标

当任一会话存在未读消息时，在主侧边栏的“聊天”导航按钮右上角显示一个红色小圆点。全部会话已读后圆点自动消失。圆点不显示具体数量，当前是否位于聊天页面不影响它展示真实的全局未读状态。

## 现有能力

客户端的 `ClientDataContext` 已全局提供 `conversations`，每个会话都有 `unreadCount`。实时消息和标记已读流程已经负责更新该字段，因此本功能不新增服务端接口、实时事件或未读状态。

全局未读状态直接派生为：

```ts
conversations.some((conversation) => conversation.unreadCount > 0)
```

## NotificationDot 组件

新增 `client-web/src/components/ui/notification-dot.tsx`，导出通用的 `NotificationDot` 视觉原语。

组件职责：

- 渲染一个默认约 10px 的红色圆点。
- 提供背景分隔环，保证圆点在深浅背景上都清晰。
- 默认不响应指针事件并对辅助技术隐藏。
- 接受普通 `span` 属性和 `className`，允许使用方覆盖尺寸、颜色和分隔环。

组件不负责绝对定位。使用方需要提供相对定位容器，并通过 `className` 决定圆点显示在右上角、右下角或其他位置。这样导航按钮、头像、列表项都可以复用该组件。

## 侧边栏集成

`AppLayout` 从 `useClientData()` 读取 `conversations` 并派生 `hasUnreadMessages`。渲染导航项时，仅向 `/chat` 对应的 `MainNavItem` 传递未读提示状态。

`MainNavItem` 的按钮作为相对定位容器。聊天入口存在未读时，在按钮右上角渲染 `NotificationDot`，并使用 sidebar 背景色作为分隔环颜色。

红点只反映会话数据：

- 任一 `unreadCount > 0`：显示。
- 所有 `unreadCount === 0`：隐藏。
- 当前位于 `/chat`，但仍有其他未读会话：继续显示。
- 会话数据尚未加载或为空：隐藏。

## 可访问性

`NotificationDot` 本身是装饰元素，默认设置 `aria-hidden="true"`。导航链接承担语义：无未读时使用“聊天”，有未读时使用“聊天，有未读消息”。标题仍保持简洁的“聊天”。

## 测试

在 `app-layout.test.tsx` 增加组件级回归测试：

- 任一会话未读时，聊天导航显示 `NotificationDot`，并更新无障碍名称。
- 全部会话已读时，不显示 `NotificationDot`。
- 当前路由为 `/chat` 时，未读圆点仍由 `unreadCount` 决定。

实现后运行客户端完整测试和 TypeScript 类型检查。

## 非目标

- 不在侧边栏显示未读数量或 `99+`。
- 不改变会话列表已有的未读数字徽标。
- 不新增服务端、数据库或实时同步逻辑。
- 不把 `AvatarBadge` 改造成通用组件。

## 验收标准

- 其他会话收到新消息后，聊天导航按钮出现红点。
- 用户读完最后一个未读会话后，红点自动消失。
- 红点在聊天入口激活和未激活样式下都清晰可见。
- 键盘和屏幕阅读器用户能获知聊天入口存在未读消息。
