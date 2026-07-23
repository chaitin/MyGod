# 第三方应用开发指南

## 1. 总体介绍

第三方应用通过 WebSocket 接入即应。应用使用独立的应用身份接收事件和调用 RPC，不会继承创建者的用户权限，也不能伪装成用户。

开始开发前，需要从即应的“应用接入信息”中取得：

- App ID
- 连接密钥
- WebSocket 地址

应用应运行在自己的服务端。不要把连接密钥放入浏览器、移动端、客户端安装包或公开仓库。

基本工作流程：

1. 使用 App ID 和连接密钥建立 WebSocket 连接。
2. 接收 Server 推送的 `message.created` 等可靠事件。
3. 根据需要读取会话历史或其他资源。
4. 调用 `message.send` 等 RPC 完成业务操作。
5. 业务处理完成后调用 `events.ack` 确认事件。
6. 断线重连后继续处理 Server 重放的未确认事件。

普通应用只能访问自己有权使用的用户、会话、话题和文件。联系人目录、项目、任务、代用户发送消息、`runas` 和 `entity_card` 等能力不对普通应用开放。

## 2. 协议介绍

### 2.1 建立连接

连接地址：

```text
wss://<server-host>/api/app/ws
```

握手 Header：

```http
X-MagicChat-App-ID: <App ID>
Authorization: Bearer <连接密钥>
```

Node.js 示例：

```js
import WebSocket from "ws";

const ws = new WebSocket(process.env.MAGICCHAT_APP_WS_URL, {
  headers: {
    "X-MagicChat-App-ID": process.env.MAGICCHAT_APP_ID,
    Authorization: `Bearer ${process.env.MAGICCHAT_APP_SECRET}`,
  },
});
```

连接约束：

- Server 每 30 秒发送 Ping，60 秒内未收到 Pong 会关闭连接。
- 单条入站或出站 WebSocket 消息最大为 1 MiB。
- 同一个 App ID 可以同时建立多个连接。
- 应用被禁用、删除、重置密钥或收缩授权范围时，已有连接可能被主动关闭。

握手失败状态：

| HTTP 状态 | 含义 |
| --- | --- |
| `400` | App ID 格式错误。 |
| `401` | 应用不存在或连接密钥错误。 |
| `403` | 应用已禁用。 |

### 2.2 Envelope

当前协议版本为 `1`。应用发出 `request`，Server 返回 `response`，Server 主动推送使用 `event`。

请求：

```json
{
  "v": 1,
  "kind": "request",
  "id": "req-1",
  "method": "conversation.messages.list",
  "payload": {}
}
```

成功响应：

```json
{
  "v": 1,
  "kind": "response",
  "id": "server-response-id",
  "reply_to": "req-1",
  "ok": true,
  "payload": {}
}
```

失败响应：

```json
{
  "v": 1,
  "kind": "response",
  "id": "server-response-id",
  "reply_to": "req-1",
  "ok": false,
  "error": {
    "code": "forbidden",
    "message": "当前应用无权调用该方法"
  }
}
```

事件：

```json
{
  "v": 1,
  "kind": "event",
  "id": "event-id",
  "cursor": 1287,
  "event": "message.created",
  "payload": {}
}
```

应用应使用 `reply_to` 将响应关联到本地请求，不能假设响应顺序与请求顺序一致。

### 2.3 请求 ID 与幂等

请求 `id` 由应用生成，在同一个 App 下应保持唯一。Server 会短期缓存请求结果：

- 相同请求 ID、方法和 payload 会返回第一次执行的响应。
- 相同请求 ID 携带不同内容会返回 `request_id_conflict`。
- 同一次业务操作超时重试时应复用原请求 ID。
- 不同业务操作必须使用不同请求 ID。

例如发送 5 条消息时必须调用 5 次 `message.send`，并使用 5 个不同的请求 ID。

### 2.4 常见错误

| 错误码 | 处理建议 |
| --- | --- |
| `invalid_request` | 参数错误，修正后再调用。 |
| `forbidden` | 当前应用无权操作，不要原样重试。 |
| `not_found` | 目标不存在或无权查看。 |
| `conflict` | 状态已变化，重新读取后再决定是否重试。 |
| `internal_error` | 使用指数退避进行有限重试。 |
| `unsupported_version` | 升级协议实现。 |
| `request_id_conflict` | 请求 ID 被用于不同内容。 |
| `response_too_large` | 减小 `limit` 或缩小请求范围。 |

## 3. 如何接收消息

### 3.1 `message.created`

用户向应用所在会话发送消息后，Server 推送：

```json
{
  "v": 1,
  "kind": "event",
  "id": "event-id",
  "cursor": 1287,
  "event": "message.created",
  "payload": {
    "conversation": {
      "id": "f967369f-9fd3-4058-92f0-b1960b5ea783",
      "name": "报表机器人",
      "type": "app"
    },
    "sender": {
      "id": "用户 ID",
      "type": "user",
      "name": "Alice",
      "nickname": "Alice",
      "email": "alice@example.com"
    },
    "message": {
      "id": "消息 ID",
      "seq": 42,
      "body": {"type": "text", "content": "请生成本周报表"},
      "summary": "请生成本周报表",
      "created_at": "2026-07-20T06:05:00Z"
    }
  }
}
```

推送规则：

- 应用单聊：用户消息会推送给会话中的应用。
- 群聊：只有消息明确 `@` 当前应用时才推送；普通消息和 `@all` 不会推送。
- 话题：话题内的用户消息会推送给仍参与该话题的应用。

话题消息的 `conversation` 还包含父会话和来源消息位置：

```json
{
  "id": "话题会话 ID",
  "name": "发布计划",
  "type": "topic",
  "parent": {
    "id": "父会话 ID",
    "name": "产品讨论组",
    "type": "group"
  },
  "source_message": {
    "id": "来源消息 ID",
    "seq": 42
  }
}
```

### 3.2 `topic.closed`

应用参与的话题被关闭后，Server 推送：

```json
{
  "v": 1,
  "kind": "event",
  "id": "event-id",
  "cursor": 1288,
  "event": "topic.closed",
  "payload": {
    "archived": true,
    "conversation_id": "话题会话 ID",
    "parent_conversation_id": "父会话 ID",
    "source_message_id": "来源消息 ID"
  }
}
```

收到后应停止向该话题发送新消息，并结束与该话题关联的本地任务或 Session。

### 3.3 `choice.response_created`

用户回答由当前应用发送的选择消息后，Server 会向该应用推送可靠事件：

```json
{
  "v": 1,
  "kind": "event",
  "id": "event-id",
  "cursor": 1289,
  "event": "choice.response_created",
  "payload": {
    "conversation": {
      "id": "会话 ID",
      "name": "项目讨论组",
      "type": "group"
    },
    "choice_message": {
      "id": "选择消息 ID",
      "seq": 43,
      "body": {
        "type": "choice",
        "content_type": "markdown",
        "content": "**请选择部署时间**",
        "selection": "single",
        "options": [
          {"id": "today", "label": "今天"},
          {"id": "tomorrow", "label": "明天"}
        ]
      },
      "summary": "[选择] 请选择部署时间",
      "created_at": "2026-07-24T08:00:00Z"
    },
    "sender": {
      "id": "用户 ID",
      "type": "user",
      "name": "Alice",
      "nickname": "Alice",
      "email": "alice@example.com"
    },
    "response": {
      "id": "回答 ID",
      "option_ids": ["tomorrow"],
      "created_at": "2026-07-24T08:01:00Z"
    }
  }
}
```

`response.option_ids` 使用发送选择消息时定义的选项 ID；`single` 只会包含一个 ID，`multiple` 可以包含多个。每名用户只能回答同一条选择消息一次。只有原始选择消息的发送应用会收到该事件；选择消息被撤回或删除后不能再回答。话题中的事件会在 `conversation` 中附带与 `message.created` 相同的 `parent` 和 `source_message` 信息。

### 3.4 可靠投递与 ACK

`message.created`、`choice.response_created` 和 `topic.closed` 会先写入 Server outbox。未确认事件会在重连后按 cursor 升序重放。

推荐处理顺序：

1. 按 `(app_id, cursor)` 检查是否已处理。
2. 执行业务逻辑并持久化结果。
3. 完成所有 RPC 调用。
4. 调用 `events.ack` 确认 cursor。

ACK 某个 cursor 会同时确认不大于它的所有事件。因此某个事件处理失败后，不要继续 ACK 更大的 cursor。

同一个 App ID 的所有在线连接都会收到事件；任意连接 ACK 后会影响整个应用。多实例部署必须使用共享存储去重。

最小处理示例：

```js
ws.on("message", (raw) => {
  const envelope = JSON.parse(raw.toString());

  if (envelope.kind === "response") {
    resolvePendingRequest(envelope.reply_to, envelope);
    return;
  }

  if (envelope.kind === "event") {
    enqueueSerially(async () => {
      await handleEvent(envelope);
      await request("events.ack", { cursor: envelope.cursor });
    });
  }
});
```

`resolvePendingRequest`、`enqueueSerially` 和 `request` 由应用自行实现；关键是按 cursor 串行处理，并在业务成功后 ACK。

## 4. 如何发起调用

### 4.1 能力列表

普通第三方应用当前可以调用以下 18 个 RPC：

| 分类 | 方法 | 能力 |
| --- | --- | --- |
| 消息 | `message.send` | 以应用身份发送消息。 |
| 查询 | `users.get` | 查询应用可见用户。 |
| 查询 | `apps.get` | 查询自己、公开或同群应用。 |
| 查询 | `conversations.list` | 列出应用参与的会话。 |
| 查询 | `conversation.messages.list` | 读取应用可见的会话历史。 |
| 话题 | `conversation.topic.create` | 创建或复用话题。 |
| 话题 | `conversation.topic.get` | 查询话题状态。 |
| 话题 | `conversation.topic.close` | 关闭当前应用创建的话题。 |
| 群聊 | `group_conversations.create` | 创建应用担任群主的群聊。 |
| 群聊 | `group_conversations.get` | 查询群聊详情。 |
| 群聊 | `group_conversations.update` | 修改群名称。 |
| 群聊 | `group_conversations.dissolve` | 解散群聊。 |
| 群聊 | `group_conversations.members.list` | 查询群成员。 |
| 群聊 | `group_conversations.members.add` | 添加群成员。 |
| 群聊 | `group_conversations.members.remove` | 移除群成员。 |
| 群聊 | `group_conversations.members.set_role` | 设置或取消管理员。 |
| 文件 | `temporary_files.read_urls` | 获取消息引用文件的签名 URL。 |
| 事件 | `events.ack` | 确认可靠事件。 |

不在该列表中的方法会返回 `forbidden`。普通应用不能使用 `runas`、`message.send_as_user`、联系人目录、项目或任务 RPC，也不能发送 `entity_card`。

### 4.2 通用调用方式

```js
import { randomUUID } from "node:crypto";

const pending = new Map();

function request(method, payload) {
  const id = randomUUID();

  return new Promise((resolve, reject) => {
    pending.set(id, { resolve, reject });
    ws.send(JSON.stringify({ v: 1, kind: "request", id, method, payload }));
  });
}

function resolvePendingRequest(replyTo, envelope) {
  const current = pending.get(replyTo);
  if (!current) return;
  pending.delete(replyTo);

  if (envelope.ok) current.resolve(envelope.payload);
  else current.reject(envelope.error);
}
```

生产实现还应增加请求超时、断线清理、原请求 ID 重试和待处理请求数量限制。

### 4.3 `message.send`

以应用身份发送消息。

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `target` | 是 | 消息目标对象。 |
| `target.type` | 是 | `user`、`app`、`group`、`topic` 或 `conversation`。 |
| `target.user_id` | 条件必填 | `target.type=user` 时使用。 |
| `target.conversation_id` | 条件必填 | 其他目标类型使用。 |
| `message` | 是 | 消息 body。 |

示例：

```json
{
  "target": {
    "type": "group",
    "conversation_id": "群聊 ID"
  },
  "message": {
    "type": "markdown",
    "content": "## 处理结果\n\n已生成 5 项统计。"
  }
}
```

`target.type=user` 会创建或复用用户与应用的一对一会话，目标用户必须处于应用可见范围内。其他目标要求当前应用仍是会话成员且会话可发言。

可发送的消息类型包括 `text`、`markdown`、`choice`、`link`、`card`、`chart`、`image` 和 `file`。不能发送 `entity_card`。图片和文件应提供可由 Server 拉取的 URL，整个 Envelope 仍受 1 MiB 限制。

发送选择消息仍使用 `message.send`，把 `message` 设置为 `choice` body：

```json
{
  "target": {
    "type": "group",
    "conversation_id": "群聊 ID"
  },
  "message": {
    "type": "choice",
    "content_type": "markdown",
    "content": "**请选择部署时间**",
    "selection": "single",
    "options": [
      {"id": "today", "label": "今天"},
      {"id": "tomorrow", "label": "明天"},
      {"id": "next_week", "label": "下周"}
    ]
  }
}
```

选择消息字段约束：

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `message.type` | 是 | 固定为 `choice`。 |
| `message.content_type` | 是 | `text` 或 `markdown`，决定问题正文的渲染方式。 |
| `message.content` | 是 | 问题正文，去除首尾空白后不能为空，最多 5000 个字符。 |
| `message.selection` | 是 | `single` 表示单选，`multiple` 表示多选。 |
| `message.options` | 是 | 2 到 20 个选项。 |
| `message.options[].id` | 是 | 选项唯一 ID，1 到 64 个字符，只允许字母、数字、`-` 和 `_`。 |
| `message.options[].label` | 是 | 展示给用户的单行纯文本，最多 200 个字符，不能包含换行或控制字符。 |

用户提交后，应用通过可靠的 `choice.response_created` 事件获取所选 ID。该事件必须与其他可靠事件一样在业务处理成功后调用 `events.ack`。

响应包含 `conversation`、`message` 和 `created`。同一请求 ID 重试不会重复发送。

### 4.4 `users.get`

查询应用可见范围内的有效用户。

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `user_id` | 是 | 用户 UUID。 |

```json
{"user_id":"用户 ID"}
```

响应的 `user` 包含 `id`、`name`、`nickname`、`email` 和 `avatar`。无权查询和用户不存在都返回 `not_found`。

### 4.5 `apps.get`

查询应用基本资料。

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `app_id` | 是 | 应用 UUID。 |

```json
{"app_id":"应用 ID"}
```

只能查询当前应用自身、`public` 应用或与当前应用处于同一有效群聊的应用。响应包含 `id`、`name`、`description`、`avatar`、`enabled`、`online` 和 `visibility`，不包含连接密钥。

### 4.6 `conversations.list`

列出当前应用参与的应用单聊、群聊和话题。

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `keyword` | 否 | 按会话名称过滤。 |
| `limit` | 否 | 默认 20，最大 100。 |

```json
{"keyword":"报表","limit":20}
```

响应的 `conversations` 元素包含 `conversation_id`、`name`、`type`、`member_count` 和 `last_active_at`，按最后活跃时间倒序排列。普通应用不得携带代用户查询参数。

### 4.7 `conversation.messages.list`

读取当前应用可见的会话历史。

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `conversation_id` | 是 | 会话 UUID。 |
| `before_or_equal_seq` | 是 | 正整数，读取不大于该 seq 的消息。 |
| `limit` | 否 | 默认 30，最大 100。 |

```json
{
  "conversation_id": "会话 ID",
  "before_or_equal_seq": 42,
  "limit": 30
}
```

消息按 seq 升序返回，并遵守应用的 `history_visible_from_seq`。已撤回消息不返回原始 body。读取话题时，响应还包含父会话和来源消息快照。普通应用不得携带 `runas`。

### 4.8 `conversation.topic.create`

基于一条可见历史消息创建或复用话题。

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `conversation_id` | 是 | 父会话 UUID。 |
| `source_message_id` | 是 | 来源消息 UUID。 |

```json
{
  "conversation_id": "父会话 ID",
  "source_message_id": "来源消息 ID"
}
```

同一来源消息只能对应一个话题，重复创建会返回同一话题。应用必须是父会话成员，来源消息必须可见，话题不能嵌套。

响应包含 `conversation`、`parent_conversation_id`、`source_message_id`、`last_message_seq`、`created` 和 `archived`。

### 4.9 `conversation.topic.get`

查询当前应用已参与的话题状态。

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `conversation_id` | 是 | 话题会话 UUID。 |

```json
{"conversation_id":"话题会话 ID"}
```

响应结构与话题创建响应相同，可通过 `last_message_seq` 和 `archived` 判断最新状态。

### 4.10 `conversation.topic.close`

关闭由当前应用创建的话题。

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `conversation_id` | 是 | 话题会话 UUID。 |
| `expected_last_message_seq` | 是 | 调用 `conversation.topic.get` 获得的最新 seq，可为 0。 |

```json
{
  "conversation_id": "话题会话 ID",
  "expected_last_message_seq": 12
}
```

如果话题在读取后又产生新消息，返回 `conflict`。调用方应重新读取后再决定是否关闭。关闭后不能继续发言，并会产生可靠的 `topic.closed` 事件。

### 4.11 `group_conversations.create`

创建由当前应用担任群主的群聊。

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `name` | 是 | 群名称，最多 120 个字符。 |
| `member_ids` | 是 | 用户 UUID 数组，至少包含一名有效用户。 |
| `app_ids` | 否 | 其他应用 UUID 数组。 |

```json
{
  "name": "项目讨论组",
  "member_ids": ["用户 ID"],
  "app_ids": ["其他应用 ID"]
}
```

调用应用自动加入并成为群主。用户必须处于应用可见范围，其他应用必须为有效的 `public` 应用，总成员数最多 500。

响应包含 `conversation` 和本次创建产生的系统 `message`。

### 4.12 `group_conversations.get`

查询当前应用已加入的群聊详情。

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `conversation_id` | 是 | 群聊 UUID。 |

```json
{"conversation_id":"群聊 ID"}
```

响应的 `conversation` 包含群名称、状态、群主、创建者、成员数量、当前应用角色和创建时间等信息。

### 4.13 `group_conversations.update`

修改群名称。

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `conversation_id` | 是 | 群聊 UUID。 |
| `name` | 是 | 新名称，最多 120 个字符。 |

```json
{"conversation_id":"群聊 ID","name":"新版项目讨论组"}
```

应用必须是群主或管理员。响应包含更新后的 `conversation` 和可能产生的系统 `message`。

### 4.14 `group_conversations.dissolve`

解散群聊。

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `conversation_id` | 是 | 群聊 UUID。 |

只有应用群主可以调用。成功响应：

```json
{"conversation_id":"群聊 ID"}
```

### 4.15 `group_conversations.members.list`

分页查询群成员。

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `conversation_id` | 是 | 群聊 UUID。 |
| `page` | 否 | 默认 1。 |
| `page_size` | 否 | 默认 100，最大 500。 |

```json
{"conversation_id":"群聊 ID","page":1,"page_size":100}
```

响应包含 `members`、`page`、`page_size` 和 `total`。成员包含身份资料、`role` 和 `joined_at`。

### 4.16 `group_conversations.members.add`

向群聊添加用户或应用。

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `conversation_id` | 是 | 群聊 UUID。 |
| `member_ids` | 条件必填 | 用户 UUID 数组。 |
| `app_ids` | 条件必填 | 应用 UUID 数组。 |

`member_ids` 和 `app_ids` 至少有一个非空。调用应用必须是群主或管理员。用户必须处于当前应用可见范围，其他应用必须为有效的 `public` 应用。

```json
{
  "conversation_id": "群聊 ID",
  "member_ids": ["用户 ID"],
  "app_ids": ["应用 ID"]
}
```

### 4.17 `group_conversations.members.remove`

移除群成员。

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `conversation_id` | 是 | 群聊 UUID。 |
| `member_type` | 是 | `user` 或 `app`。 |
| `member_id` | 是 | 目标成员 UUID。 |

管理员只能移除普通成员；群主可以移除管理员和普通成员。应用不能移除自己、群主或群内最后一名有效用户。

```json
{
  "conversation_id": "群聊 ID",
  "member_type": "user",
  "member_id": "用户 ID"
}
```

### 4.18 `group_conversations.members.set_role`

设置或取消群管理员。

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `conversation_id` | 是 | 群聊 UUID。 |
| `member_type` | 是 | `user` 或 `app`。 |
| `member_id` | 是 | 目标成员 UUID。 |
| `role` | 是 | `admin` 表示设置管理员，`member` 表示取消管理员。 |

只有应用群主可以调用，不能修改群主角色。

```json
{
  "conversation_id": "群聊 ID",
  "member_type": "user",
  "member_id": "用户 ID",
  "role": "admin"
}
```

### 4.19 `temporary_files.read_urls`

获取可见消息所引用临时文件的签名 URL。

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `conversation_id` | 是 | 文件所在消息的会话 UUID。 |
| `message_id` | 是 | 引用文件的消息 UUID。 |
| `file_ids` | 是 | 临时文件 UUID 数组。 |

```json
{
  "conversation_id": "会话 ID",
  "message_id": "消息 ID",
  "file_ids": ["临时文件 ID"]
}
```

Server 会校验应用会话权限、消息历史范围以及文件是否确实被该消息引用。签名 URL 最长有效 24 小时，且不会超过临时文件剩余生命周期。

### 4.20 `events.ack`

确认已经完成处理的可靠事件。

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `cursor` | 是 | 正整数事件 cursor。 |

```json
{"cursor":1287}
```

成功响应同样返回 `cursor`。ACK 会同时确认不大于该 cursor 的所有事件，必须在业务处理和相关 RPC 全部成功后调用。
