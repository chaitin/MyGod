# Server Auth MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first Go service with admin login, admin-created users, generated initial passwords, and user login.

**Architecture:** A modular Go service under `server/` using Echo for HTTP, GORM for Postgres access, Goose SQL migrations, and server-side sessions in separate admin/user session tables. API responses use a unified `{ success, data/error }` envelope.

**Tech Stack:** Go, Echo, GORM, Postgres, Goose, Argon2id, YAML config.

---

### Task 1: Server Module And Skeleton

**Files:**
- Create: `server/go.mod`
- Create: `server/cmd/server/main.go`
- Create: `server/config.example.yaml`
- Create: `server/internal/config/config.go`
- Test: `server/internal/config/config_test.go`

- [ ] Write config tests for `CONFIG` path loading, required admin password, and default listen address.
- [ ] Run `go test ./internal/config` and verify the package fails before implementation.
- [ ] Implement minimal YAML config loading with default `:20080`.
- [ ] Run `go test ./internal/config` and verify it passes.

### Task 2: Database Migrations And Models

**Files:**
- Create: `server/migrations/00001_create_users.sql`
- Create: `server/migrations/00002_create_admin_sessions.sql`
- Create: `server/migrations/00003_create_user_sessions.sql`
- Create: `server/internal/store/models.go`
- Create: `server/internal/store/db.go`

- [ ] Write model/migration tests using SQLite in memory for GORM constraints where practical.
- [ ] Run the model tests and verify they fail before implementation.
- [ ] Implement GORM models for `users`, `admin_sessions`, and `user_sessions`.
- [ ] Add Goose SQL migrations for Postgres.
- [ ] Run model tests and verify they pass.

### Task 3: Auth Utilities

**Files:**
- Create: `server/internal/auth/password.go`
- Create: `server/internal/auth/session.go`
- Create: `server/internal/auth/random.go`
- Test: `server/internal/auth/auth_test.go`

- [ ] Write tests for Argon2id hash/verify, wrong password rejection, random initial password length, and session token hashing.
- [ ] Run `go test ./internal/auth` and verify it fails before implementation.
- [ ] Implement password hashing, password verification, secure random password generation, random session token generation, and token SHA-256 hashing.
- [ ] Run `go test ./internal/auth` and verify it passes.

### Task 4: HTTP API

**Files:**
- Create: `server/internal/httpserver/server.go`
- Create: `server/internal/httpserver/response.go`
- Create: `server/internal/httpserver/auth_handlers.go`
- Create: `server/internal/httpserver/middleware.go`
- Test: `server/internal/httpserver/server_test.go`

- [ ] Write API tests for `POST /api/admin/auth/login`, `POST /api/admin/users`, and `POST /api/client/auth/login`.
- [ ] Verify tests fail before implementation.
- [ ] Implement Echo routes, unified response envelope, admin session middleware, user creation, and user login.
- [ ] Verify API tests pass.

### Task 5: Final Verification

**Commands:**
- `cd server && go test ./...`
- `cd server && go vet ./...`
- `cd server && go build ./cmd/server`
- `cd client-web && pnpm test`
- `cd client-web && pnpm typecheck`

- [ ] Run all verification commands.
- [ ] Report any failures with exact command output.
- [ ] If all pass, report the implemented files and API behavior.
