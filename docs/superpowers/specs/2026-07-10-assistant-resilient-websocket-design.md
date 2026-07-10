# Assistant Resilient WebSocket Design

## 背景

assistant 当前把 WebSocket 连接、请求响应路由和 conversation agent runner 都创建在单次 `serveConnection` 生命周期内。连接发生读错误、网络中断或服务端重启时，`serveConnection` 返回并调用 `runner.CancelAll()`，正在执行及空闲待复用的 agent session 都会被取消。当前单条 WebSocket 消息上限还是 64 KiB，而 assistant 会一次读取最多 30 条完整历史消息，合法响应也可能超过该上限。

本设计将 agent 生命周期与 WebSocket 生命周期分离。连接可以断开和重建，但已有 session 不得重建、回放原始任务或被连接错误取消。

## 目标

- app WebSocket 单条消息上限统一提升到 1 MiB。
- 以 conversation ID 为键，由进程级 Session Manager 创建、复用和清理 agent session。
- 保留现有 1 小时空闲复用机制，且不再受 WebSocket 连接寿命影响。
- WebSocket Manager 独立负责连接、心跳、断线检测和自动重连。
- 所有依赖 app WebSocket 的请求通过统一可靠调用层执行。
- 可重试连接或请求错误在首次失败后最多重试 10 次，不无限等待。
- 重试复用原 request ID，服务端保证重复请求不会重复产生副作用。
- 重连后补投断线期间未确认的 app 事件。
- WebSocket 断线不能取消、删除或重建 agent session。

## 非目标

- 不把 agent session 落盘。
- 不支持 assistant 进程重启后恢复内存中的 LLM session。
- 不改变当前 1 小时 session 空闲清理时间。
- 不提高内联生成文件的 64 KiB 业务限制。
- 不保证 server 进程重启后恢复进程内的请求响应缓存；消息发送仍依赖现有数据库幂等约束。

## 总体架构

```text
Assistant Process
├── Message Router
│   └── 校验事件、判断是否需要处理、转换触发消息
├── Session Manager
│   ├── conversation ID -> Session Job
│   ├── 创建或追加指令
│   ├── 保持每个 conversation 的执行顺序
│   └── 1 小时空闲清理
├── Reliable App Requester
│   ├── request ID 与 pending response
│   ├── 最多 10 次重试
│   └── 跨连接重试
└── WebSocket Manager
    ├── 建连、心跳和读写
    ├── 当前连接 generation
    ├── 断线检测
    └── 自动重连
```

WebSocket Manager 不引用 Session Manager 的取消函数。Session Manager 的根 context 只从 assistant 进程 context 派生。

## Session Manager

Session Manager 延续当前 `conversationAgentRunner` 的核心语义，但生命周期提升到 `Client`/assistant 进程级。

收到需要处理的 `message.created` 后：

1. Message Router 完成协议解析和 `shouldHandleIncomingMessage` 判断。
2. 以 conversation ID 将触发消息提交给 Session Manager。
3. 如果 conversation 没有 session，创建 `agent.Session` 和 session job。
4. 如果 session 已存在，过滤已经消费的历史消息，然后调用 `Session.Append` 追加新指令。
5. 如果 session 正在执行，新指令保留在 pending 队列，当前 cycle 结束后继续处理。
6. 如果 session 空闲，停止空闲清理定时器并立即启动下一次 cycle。

每个 job 继续保存：

- `agent.Session`
- `lastSeenSeq`
- authorization store
- pending instructions
- output sink
- process-scoped job context
- running 状态和空闲定时器

指令完成且没有 pending instruction 时，session 进入空闲状态。1 小时内的新指令复用原 session；超过 1 小时才从 map 删除并取消 job context。

WebSocket 断线时不得执行 `CancelAll`。`CancelAll` 只用于 assistant 进程退出。

## WebSocket Manager

WebSocket Manager 是进程级传输组件，只负责连接状态和协议帧：

- 使用 app ID 和 secret 建立连接。
- 设置 1 MiB read limit。
- 维护 ping、pong、read deadline 和 write deadline。
- 为每条物理连接分配递增 generation。
- reader 将事件交给 Message Router，将 response 交给 Reliable App Requester。
- 任一 reader/writer 错误只关闭当前 generation，并启动重连。
- 保证任意时刻最多有一个可写 generation。

重连在首次失败后最多重试 10 次。基础序列为 `1s, 2s, 4s, 8s, 16s`，后续等待上限为 30 秒，并加入随机抖动。成功连接后重试计数和 backoff 立即重置。

401、403 等确定不可恢复的认证错误不参与瞬时错误重试，直接记录明确错误。10 次重试耗尽后，本轮连接进入失败状态；后续新的发送或显式连接需求可以开始新的 10 次重试周期。assistant 进程退出时立即终止重连。

## Reliable App Requester

所有依赖 app WebSocket 的操作必须经过同一个可靠请求入口，而不是在每个工具里分别实现重试。这包括：

- conversation history 加载
- temporary file URL 获取
- contacts、recent conversations 和 read history
- reply 和 send as user
- create group 和 add group members
- 最终 markdown 回复

一次逻辑请求只生成一个 request ID。请求对象和 response channel 在逻辑请求完成前保存在进程级 pending map 中，不属于某条物理连接。

请求流程：

1. 获取当前可用 connection generation。
2. 写入带稳定 request ID 的 envelope。
3. 等待匹配 `reply_to` 的响应。
4. 如果 generation 断开、写失败或等待响应超时，将同一 envelope 放入下一次尝试。
5. 首次请求失败后使用指数退避，最多重试 10 次。
6. 成功响应后从 pending map 删除。
7. 不可重试协议错误立即返回。
8. 10 次重试耗尽后返回 `websocket_unavailable`/retry-exhausted 错误，但不取消 session。

请求 context 从 agent job 或 assistant process 派生，不能从单次连接 context 派生。

## 请求幂等与响应重放

TCP/WebSocket 写成功不表示 assistant 一定收到应用响应。server 可能已经执行请求，但响应在断线时丢失。Reliable App Requester 会使用相同 request ID 重发，因此 server 必须按 `(app_id, request_id)` 去重。

server 的 app connection manager 维护有界、带 TTL 的请求记录：

- request ID
- method
- payload digest
- running/completed 状态
- 完整 response envelope
- 创建和过期时间

第一次请求执行 handler 并保存响应。重复请求如果 method 和 payload digest 相同：

- running：等待第一次执行完成；
- completed：直接重放缓存响应。

同一 request ID 携带不同 method 或 payload 时返回 `request_id_conflict`。

缓存采用容量、字节数和 TTL 三重限制，避免无限增长：completed response 保留 10 分钟，最多 1,000 条且编码后总大小不超过 64 MiB，按最近最少使用顺序淘汰；正在执行的记录不参与淘汰。进程内缓存覆盖本设计的短线重连场景。已有 `message.send` 和 `message.send_as_user` 继续使用 request ID 作为 client message ID，由数据库唯一约束提供额外幂等保护。

## 事件补投

连接断开时 server 无法推送 `message.created`。为了避免新触发消息丢失，server 为 app 事件维护可确认游标：

- app 事件写入 durable outbox，具有全局递增 cursor 和 app ID。
- live connection 收到带 cursor 的事件。
- assistant 将事件成功提交给 Session Manager 后发送 ack。
- server 保存每个 app 的最后确认 cursor。
- app 重连后，server 按 cursor 顺序补投未确认事件，再切换到 live delivery。

Session Manager 使用 message ID 和 conversation seq 去重，因此 ack 丢失造成的重复投递不会重复追加指令。

本设计不持久化 assistant 内存 session，因此 assistant 进程在 ack 后崩溃仍可能丢失尚未完成的内存任务；进程重启恢复不属于本期范围。

## 1 MiB 消息上限

assistant 和 server 的 app WebSocket 统一采用：

```go
const maxMessageBytes = 1 << 20
```

限制应用于：

- assistant 入站 read limit
- assistant 出站 envelope 编码检查
- server 入站 read limit
- server 出站 envelope 编码检查

server 在写出前编码完整 envelope。超过 1 MiB 时，不发送超大 envelope，而发送小型 `response_too_large` 错误；事件超限则记录结构化错误并保持连接可用。历史、联系人等聚合接口仍保留数量上限，1 MiB 不是取消分页的替代品。

内联文件 64 KiB 是独立业务限制，本期保持不变，并修正文案使其不再被描述为 WebSocket 的全局限制。

## 错误语义

- 连接错误：关闭当前 generation，触发重连，不触碰 session。
- 请求瞬时错误：首次失败后最多进行 10 次指数退避重试。
- 请求不可恢复错误：立即作为工具错误返回 agent。
- 重试耗尽：返回明确工具错误，session 保留并可继续接收后续指令。
- 输出发送失败：按相同可靠请求策略重试；耗尽后记录包含 conversation ID 和 request ID 的错误。
- assistant 退出：取消 Session Manager、pending request 和 WebSocket Manager。

## 并发与顺序

- 每个 conversation 同一时刻最多运行一个 agent cycle。
- 同一 conversation 的触发消息按 seq 追加。
- 不同 conversation 可以并行执行。
- WebSocket writer 串行写 envelope 和 control frame。
- response 可以乱序到达，通过 `reply_to` 路由。
- 旧 generation 的 reader 不得关闭或覆盖新 generation。

## 测试要求

- 64 KiB 以上、1 MiB 以下的 envelope 可以双向传输。
- 超过 1 MiB 的入站消息被拒绝，出站消息返回明确错误且连接不被意外打断。
- agent 执行期间断线不会取消 model context 或 session context。
- 断线后原 session 对象继续运行，不重新创建初始任务。
- 同一 conversation 的新消息追加到原 session。
- session 空闲 1 小时内复用，超时后清理。
- 连接首次失败后最多重试 10 次，并验证指数退避和成功后重置。
- 请求断线重试保持同一 request ID。
- server 已执行但响应丢失时，重试不会重复创建消息、群聊或成员事件。
- 断线期间的 app 事件在重连后按 cursor 补投。
- 重复补投事件不会重复追加指令。
- 连续断线不会产生多个并行可写连接。
- assistant 退出能取消所有 goroutine 和 timer。
- assistant 和 server 相关包通过 `go test -race`。

## 成功标准

- 复现原 `websocket: read limit exceeded` 的历史响应在 1 MiB 内可正常处理。
- 任意短线重连都不会调用 `CancelAll`，也不会重建正在运行或空闲复用中的 session。
- 依赖 WebSocket 的工具在首次失败后最多重试 10 次，失败只影响该次工具结果。
- 断线期间产生的用户触发消息在重连后不会丢失。
- server 对相同 app request ID 的重试不会重复产生副作用。
