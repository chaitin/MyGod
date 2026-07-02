# 服务端架构设计

日期：2026-07-01

## 当前阶段目标

先实现服务端最小闭环：

- 管理员登录。
- 管理员创建普通用户。
- 创建普通用户时自动生成随机初始密码。
- 普通用户登录。
- 服务端具备配置文件机制。
- 普通用户创建群聊会话。

当前只实现群聊会话创建，不实现消息发送、聊天历史、WebSocket、AI 助手回复、任务、Agent、外部 IM 接入和复杂权限。这些能力只在架构上预留边界。

## 技术选型

服务端使用 Go。

- HTTP 框架：Echo。
- 数据库：Postgres。
- ORM：GORM。
- Migration：Goose。
- 密码哈希：Argon2id。
- 日志：Go 标准库 `slog`。
- 配置：最小 YAML 配置文件，路径由 `CONFIG` 环境变量指定。
- 会话：服务端 session + HttpOnly Cookie。

选择理由：

- Echo 足够轻量，路由分组和 middleware 清晰，适合 `/api/admin` 与 `/api/client` 分区。
- GORM 能快速完成 MVP 数据访问，后续复杂查询可以局部改为手写 SQL。
- Goose 与 SQL migration 配合直接，数据库结构变化可审计、可回滚。
- 服务端 session 比纯 JWT 更适合企业后台，管理员禁用用户、重置密码或调整配置后更容易让会话失效。

## 目录结构

建议服务端放在仓库根目录的 `server/` 下：

```text
server/
  cmd/server/main.go
  config.example.yaml
  internal/config/
  internal/http/
    middleware/
    admin/
    client/
  internal/auth/
  internal/user/
  internal/session/
  internal/store/
    postgres/
  internal/random/
  migrations/
```

目录职责：

- `cmd/server`：启动入口，加载配置、连接数据库、执行依赖组装、启动 Echo。
- `internal/config`：读取 YAML 配置，未提供的非敏感配置使用默认值。
- `internal/http/admin`：管理员 API handler。
- `internal/http/client`：普通用户 API handler。
- `internal/http/middleware`：鉴权、请求日志、错误处理、CORS。
- `internal/auth`：登录、密码校验、密码哈希、session 创建。
- `internal/user`：普通用户创建、查询、状态管理。
- `internal/session`：session 存储和校验。
- `internal/store/postgres`：GORM 初始化、事务、数据库模型。
- `migrations`：Goose SQL migration 文件。

## API 分区

管理员 API 全部放在：

```text
/api/admin/...
```

普通用户 API 全部放在：

```text
/api/client/...
```

健康检查不属于业务 API，可以放在：

```text
/healthz
```

## API 文档

API 文档从服务端 Go 注释自动生成，生成产物放在仓库根目录：

```text
api-docs/swagger.json
api-docs/swagger.yaml
```

生成命令：

```bash
./scripts/generate-api-docs.sh
```

服务端启动后，如果能在当前目录或父级目录找到 `api-docs/swagger.json`，会自动启用：

```text
/api-docs/swagger.json
/api-docs/swagger.yaml
/swagger/index.html
```

仓库使用 `.githooks/pre-commit` 作为 Git hook。每次提交前会自动运行 API 文档生成脚本，并把 `api-docs/swagger.json` 和 `api-docs/swagger.yaml` 加入本次提交。当前仓库已通过以下配置启用：

```bash
git config core.hooksPath .githooks
```

后续新增接口时，需要在 handler 上补充 `swaggo/swag` 注释，提交前文档会自动刷新。

## 管理员模型

当前阶段只有一个默认管理员：

- 管理员账号固定为 `admin`。
- 管理员密码通过配置文件传入。
- 管理员不在管理端里创建，也不支持多个管理员。
- 管理员不作为普通用户出现在通讯录里。

建议 MVP 先采用“配置驱动的单管理员”：

- 管理员账号不写入 `users` 表。
- 登录时服务端读取配置中的管理员密码，验证通过后创建管理员 session。
- 管理员 session 记录在 `admin_sessions` 表。
- 修改管理员密码通过修改配置并重启服务完成。

这样可以避免第一版引入管理员用户管理、管理员密码重置、多管理员角色等额外复杂度。

## 普通用户模型

普通用户只能由管理员创建，不能自行注册。

创建普通用户时：

- 管理员提交邮箱和名称。
- 邮箱是登录账号，必须全局唯一。
- 服务端生成随机初始密码。
- 服务端保存初始密码的 Argon2id 哈希。
- API 响应只在创建成功时返回一次明文初始密码。
- 明文密码不写入数据库、不写入日志。

普通用户登录时：

- 使用邮箱和密码登录 `/api/client/auth/login`。
- 用户必须处于启用状态。
- 登录成功后创建普通用户 session。
- 普通用户 session 记录在 `user_sessions` 表，并关联 `user_id`。

普通用户暂时不保存头像。前端可以先用固定头像、名称首字母或本地默认头像展示。

## Session 设计

管理员和普通用户的 session 分开存储：

- 管理员 session 存在 `admin_sessions`。
- 普通用户 session 存在 `user_sessions`。

分开存储可以让管理员登录态和普通用户登录态的生命周期、清理策略和后续权限扩展更清晰。管理员和普通用户使用不同的 cookie 名称，避免两个面板在同一浏览器中登录时互相覆盖。管理员 API 只查询 `admin_sessions`，普通用户 API 只查询 `user_sessions`。

Cookie 建议：

- 管理员名称：`admin_session`
- 普通用户名称：`user_session`
- `HttpOnly: true`
- `SameSite: Lax`
- `Secure` 可以先由运行环境判断或后续再配置
- 过期时间使用服务端默认值，当前默认 7 天

管理员 API 只接受能在 `admin_sessions` 中查到的有效 session。

普通用户 API 只接受能在 `user_sessions` 中查到的有效 session。

## 配置机制

服务端启动时读取配置文件，路径通过 `CONFIG` 环境变量指定：

```text
CONFIG=/path/to/config.yaml
```

如果没有设置 `CONFIG`，服务端可以默认读取当前工作目录下的 `config.yaml`。如果文件不存在或管理员密码为空，服务端拒绝启动。

配置先保持最小化，能不用配置的都先不用配置。第一版只要求：

- Postgres 连接串。
- 管理员密码。

示例配置：

```yaml
database:
  dsn: "postgres://app:app@localhost:5432/app?sslmode=disable"

admin:
  password: ""
```

固定默认值：

- HTTP 监听地址默认使用 `:20080`。
- 管理员账号固定为 `admin`。
- 管理员 Session cookie 名称固定为 `admin_session`。
- 普通用户 Session cookie 名称固定为 `user_session`。
- Session 过期时间固定为 7 天。
- 随机初始密码长度固定为 16 位。

## 数据表设计

### users

普通用户表，不包含默认管理员。

字段建议：

- `id`
- `email`
- `name`
- `password_hash`
- `status`
- `created_at`
- `updated_at`

约束：

- `email` 唯一，大小写不敏感。实现上可以保存规范化后的小写邮箱，或在 Postgres 中使用 `lower(email)` 唯一索引。
- `status` 可取 `active`、`disabled`。

### admin_sessions

管理员登录会话表。

字段建议：

- `id`
- `token_hash`
- `expires_at`
- `created_at`
- `last_seen_at`
- `user_agent`
- `ip`

### user_sessions

普通用户登录会话表。

字段建议：

- `id`
- `token_hash`
- `user_id`
- `expires_at`
- `created_at`
- `last_seen_at`
- `user_agent`
- `ip`

约束：

- `user_id` 指向 `users.id`。
- 普通用户被禁用后，对应的 user session 应被视为不可用。

### conversations

统一会话表，用于承载一对一、群聊和 AI 会话。当前先实现 `group`。

字段建议：

- `id`
- `kind`：`direct`、`group`、`assistant`
- `name`
- `created_by_user_id`
- `status`：`active`、`dissolved`
- `posting_policy`：`open`、`muted`
- `created_at`
- `updated_at`
- `dissolved_at`
- `last_message_id`
- `last_message_at`

约束：

- `created_by_user_id` 指向 `users.id`，表示创建者这个历史事实。
- 群聊创建时 `status = active`，`posting_policy = open`。
- 群主不放在 `conversations` 表中，而是通过 `conversation_members.role = owner` 表达。

### conversation_members

会话成员表，记录成员身份和该成员在会话里的角色。

字段建议：

- `conversation_id`
- `member_type`：`user`、`assistant`
- `member_id`
- `user_member_id`：数据库生成列，仅当 `member_type = user` 时等于 `member_id`
- `role`：`owner`、`admin`、`member`
- `joined_at`
- `left_at`
- `last_read_message_id`

约束：

- 当前群聊只写入 `member_type = user`。
- `user_member_id` 指向 `users.id`，用数据库约束保证 user 成员必须存在。
- 创建者成员角色为 `owner`，其他成员角色为 `member`。
- 通过部分唯一索引保证每个未退出的会话最多只有一个 `owner`。

## Migration 策略

使用 Goose 管理 migration。

目录：

```text
server/migrations/
```

命名：

```text
00001_create_users.sql
00002_create_admin_sessions.sql
00003_create_user_sessions.sql
00004_create_app_settings.sql
00005_create_conversations.sql
```

服务端启动时不自动执行 destructive migration。开发环境可以通过命令执行：

```bash
goose -dir server/migrations postgres "$DATABASE_DSN" up
```

后续 Docker Compose 可以提供单独的 migration 命令或启动脚本。

## API 响应格式

所有 API 响应统一包一层。

成功响应：

```json
{
  "success": true,
  "data": {}
}
```

失败响应：

```json
{
  "success": false,
  "error": {
    "code": "invalid_credentials",
    "message": "邮箱或密码错误"
  }
}
```

约定：

- 成功响应必须有 `success: true` 和 `data`。
- 失败响应必须有 `success: false` 和 `error`。
- `data` 内部再放具体业务对象，例如 `admin`、`user`、`initial_password`。
- HTTP 状态码仍表达请求结果，例如登录失败返回 401，参数错误返回 400，邮箱冲突返回 409。

## API 设计

### 管理员登录

```text
POST /api/admin/auth/login
```

请求：

```json
{
  "email": "admin",
  "password": "configured-password"
}
```

响应：

```json
{
  "success": true,
  "data": {
    "admin": {
      "email": "admin"
    }
  }
}
```

行为：

- 管理员登录标识固定为 `admin`。为保持登录表单统一，请求字段名仍使用 `email`。
- 密码必须匹配配置中的管理员密码。
- 登录成功后写入 HttpOnly session cookie。
- 登录失败返回统一错误，避免泄露账号或密码是否正确。

### 创建普通用户

```text
POST /api/admin/users
```

请求：

```json
{
  "email": "wenlei@example.com",
  "name": "Wenlei Zhu"
}
```

响应：

```json
{
  "success": true,
  "data": {
    "user": {
      "id": "uuid",
      "email": "wenlei@example.com",
      "name": "Wenlei Zhu",
      "status": "active"
    },
    "initial_password": "random-generated-password"
  }
}
```

行为：

- 只有管理员 session 可以调用。
- `email` 必须全局唯一。
- 服务端生成随机密码。
- 明文初始密码只在本次响应中返回。

### 普通用户登录

```text
POST /api/client/auth/login
```

请求：

```json
{
  "email": "wenlei@example.com",
  "password": "initial-or-current-password"
}
```

响应：

```json
{
  "success": true,
  "data": {
    "user": {
      "id": "uuid",
      "email": "wenlei@example.com",
      "name": "Wenlei Zhu",
      "status": "active"
    }
  }
}
```

行为：

- 只允许普通用户登录。
- 禁用用户不能登录。
- 登录成功后写入 HttpOnly session cookie。

## 错误码

错误响应也使用统一 envelope：

```json
{
  "success": false,
  "error": {
    "code": "invalid_credentials",
    "message": "邮箱或密码错误"
  }
}
```

第一版错误码：

- `invalid_request`
- `invalid_credentials`
- `unauthorized`
- `forbidden`
- `not_found`
- `conflict`
- `internal_error`

登录失败统一使用 `invalid_credentials`，不区分邮箱不存在、密码错误或用户被禁用。

## 安全要求

- 管理员密码必须通过配置传入，未配置时服务端拒绝启动。
- 普通用户密码只保存 Argon2id 哈希。
- 随机初始密码使用安全随机数生成。
- 明文初始密码只返回一次，不进入日志。
- Session token 只保存哈希，不保存明文 token。
- 管理员 API 与普通用户 API 必须使用不同 middleware。
- 管理员 middleware 只查询 `admin_sessions`。
- 普通用户 middleware 只查询 `user_sessions`。

## 后续扩展预留

当前设计先支持登录和用户创建，但保留以下扩展方向：

- 管理员禁用用户。
- 管理员重置普通用户密码。
- 管理员查看普通用户列表。
- 管理员退出登录。
- 普通用户退出登录。
- 获取当前登录身份。
- 普通用户修改密码。
- 通讯录 API。
- 会话和消息 API。
- AI 助手 API。
- WebSocket 实时消息。
- 审计日志。
- Cookie `Secure` 策略配置。
- 多管理员。
- 角色权限。
- 企业 SSO。

这些能力不进入当前实现范围。
