#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

fail() {
  echo "deploy config check failed: $*" >&2
  exit 1
}

assert_file() {
  local path="$1"
  [[ -f "${ROOT_DIR}/${path}" ]] || fail "missing ${path}"
}

assert_not_exists() {
  local path="$1"
  [[ ! -e "${ROOT_DIR}/${path}" ]] || fail "legacy path must be removed: ${path}"
}

assert_contains() {
  local path="$1"
  local expected="$2"

  grep -Fq -- "${expected}" "${ROOT_DIR}/${path}" ||
    fail "${path} does not contain: ${expected}"
}

assert_not_contains() {
  local path="$1"
  local unexpected="$2"

  if grep -Fq -- "${unexpected}" "${ROOT_DIR}/${path}"; then
    fail "${path} must not contain: ${unexpected}"
  fi
}

assert_file "compose.yml"
assert_file "server/Dockerfile"
assert_file "assistant/Dockerfile"
assert_file "deploy/caddy/Dockerfile"
assert_file "deploy/caddy/Caddyfile"
assert_not_exists "deploy/nginx"
assert_file ".env.example"
assert_file ".github/workflows/docker.yml"
assert_file ".dockerignore"

assert_contains "compose.yml" "ghcr.1ms.run"
assert_contains "compose.yml" "ghcr.1ms.run/chaitin/magicchat"
assert_contains "compose.yml" "name: magic-chat"
assert_contains "compose.yml" "assistant:"
assert_contains "compose.yml" "container_name: magic-chat-postgres"
assert_contains "compose.yml" "container_name: magic-chat-assistant"
assert_contains "compose.yml" "container_name: magic-chat-server"
assert_contains "compose.yml" "caddy:"
assert_contains "compose.yml" "container_name: magic-chat-caddy"
assert_not_contains "compose.yml" "name: mygod"
assert_not_contains "compose.yml" "container_name: mygod-"
assert_contains "compose.yml" 'POSTGRES_DB: ${POSTGRES_DB:-magic-chat}'
assert_contains "compose.yml" 'POSTGRES_USER: ${POSTGRES_USER:-magic-chat}'
assert_contains "compose.yml" '${IMAGE_REGISTRY:-ghcr.1ms.run/chaitin/magicchat}/assistant:${IMAGE_TAG:-latest}'
assert_contains "compose.yml" 'AWS_ACCESS_KEY_ID: ${AWS_ACCESS_KEY_ID:-change-me}'
assert_contains "compose.yml" 'AWS_SECRET_ACCESS_KEY: ${AWS_SECRET_ACCESS_KEY:-change-me}'
assert_contains "compose.yml" 'AWS_ENDPOINT_URL_S3: ${AWS_ENDPOINT_URL_S3:-https://s3.example.com}'
assert_contains "compose.yml" 'AWS_REGION: ${AWS_REGION:-us-east-1}'
assert_contains "compose.yml" 'S3_BOOTSTRAP_ENABLED: ${S3_BOOTSTRAP_ENABLED:-false}'
assert_contains "compose.yml" 'POSTGRES_HOST: ${POSTGRES_HOST:-postgres}'
assert_contains "compose.yml" 'ADMIN_PASSWORD: ${ADMIN_PASSWORD:-change-me}'
assert_contains "compose.yml" 'PUBLIC_HOSTNAME: ${PUBLIC_HOSTNAME:-localhost}'
assert_contains "compose.yml" 'CLIENT_HTTPS_PORT: ${CLIENT_HTTPS_PORT:-443}'
assert_contains "compose.yml" 'ADMIN_HTTPS_PORT: ${ADMIN_HTTPS_PORT:-1443}'
assert_contains "compose.yml" 'PUBLIC_ASSETS_HOSTNAME: ${PUBLIC_ASSETS_HOSTNAME:-public-assets.localhost}'
assert_contains "compose.yml" 'PRIVATE_ASSETS_HOSTNAME: ${PRIVATE_ASSETS_HOSTNAME:-private-assets.localhost}'
assert_contains "compose.yml" 'TEMPORARY_ASSETS_HOSTNAME: ${TEMPORARY_ASSETS_HOSTNAME:-temporary-assets.localhost}'
assert_contains "compose.yml" 'AI_ASSISTANT_SECRET: ${AI_ASSISTANT_SECRET:-change-me}'
assert_contains "compose.yml" 'ASSISTANT_WEBSOCKET_URL: ${ASSISTANT_WEBSOCKET_URL:-ws://server:20080/api/app/ws}'
assert_contains "compose.yml" 'AGENT_MAX_TURNS: ${AGENT_MAX_TURNS:-50}'
assert_contains "compose.yml" 'AGENT_MAX_SESSIONS: ${AGENT_MAX_SESSIONS:-1000}'
assert_contains "compose.yml" 'LLM_BASE_URL: ${LLM_BASE_URL:-https://api.example.com}'
assert_contains "compose.yml" 'LLM_API_KEY: ${LLM_API_KEY:-change-me}'
assert_contains "compose.yml" 'LLM_MODEL_NAME: ${LLM_MODEL_NAME:-change-me}'
assert_contains "compose.yml" 'MCP_GATEWAY_URL: ${MCP_GATEWAY_URL:-https://mcp.example.com/mcp}'
assert_contains "compose.yml" 'MCP_GATEWAY_KEY: ${MCP_GATEWAY_KEY:-change-me}'
assert_contains "compose.yml" 'S3_FORCE_PATH_STYLE: ${S3_FORCE_PATH_STYLE:-false}'
assert_contains "compose.yml" 'PUBLIC_ASSETS_BUCKET: ${PUBLIC_ASSETS_BUCKET:-magicchat-public}'
assert_contains "compose.yml" 'PRIVATE_ASSETS_BUCKET: ${PRIVATE_ASSETS_BUCKET:-magicchat-private}'
assert_contains "compose.yml" 'TEMPORARY_ASSETS_BUCKET: ${TEMPORARY_ASSETS_BUCKET:-magicchat-temporary}'
assert_contains "compose.yml" 'TEMPORARY_ASSETS_EXPIRE_DAYS: ${TEMPORARY_ASSETS_EXPIRE_DAYS:-180}'
assert_contains "compose.yml" 'S3_ABORT_MULTIPART_DAYS: ${S3_ABORT_MULTIPART_DAYS:-7}'
assert_not_contains "compose.yml" "MYGOD_AI_ASSISTANT_SECRET"
assert_not_contains "compose.yml" "MYGOD_APP_SECRET"
assert_contains "compose.yml" "80:80"
assert_contains "compose.yml" '${CLIENT_HTTPS_PORT:-443}:443'
assert_contains "compose.yml" '${ADMIN_HTTPS_PORT:-1443}:1443'
assert_contains "compose.yml" 'https://127.0.0.1:443/gateway-healthz'
assert_not_contains "compose.yml" '${CLIENT_HTTPS_PORT:-443}:${CLIENT_HTTPS_PORT:-443}'
assert_not_contains "compose.yml" '${ADMIN_HTTPS_PORT:-1443}:${ADMIN_HTTPS_PORT:-1443}'
assert_contains "compose.yml" "./data/postgres/data:/var/lib/postgresql/data"
assert_not_contains "compose.yml" "./data/assistant/config:/app/config:ro"
assert_not_contains "compose.yml" "./data/server/config:/app/config:ro"
assert_contains "compose.yml" "./data/server/log:/app/log"
assert_contains "compose.yml" "./data/caddy/data:/data"
assert_contains "compose.yml" "./data/caddy/config:/config"
assert_contains "compose.yml" "./data/caddy/logs:/var/log/caddy"
assert_contains "compose.yml" 'CLIENT_HTTPS_PORT: ${CLIENT_HTTPS_PORT:-443}'
assert_contains "compose.yml" 'driver: json-file'
assert_contains "compose.yml" 'max-size: "10m"'
assert_contains "compose.yml" 'max-file: "5"'
assert_not_contains "compose.yml" "data/nginx"

assert_not_contains "compose.yml" "rustfs"
assert_not_contains "compose.yml" "RUSTFS_"
assert_not_contains "compose.yml" "CLIENT_HOSTNAME"
assert_not_contains "compose.yml" "ADMIN_HOSTNAME"
assert_not_contains "compose.yml" "CONFIG:"
assert_not_contains "compose.yml" "/etc/nginx"
if grep -Fq -- "your-org" "${ROOT_DIR}/compose.yml"; then
  fail "compose.yml should not contain placeholder image namespace"
fi
assert_not_contains "compose.yml" "MYGOD_LLM_"
assert_not_contains "compose.yml" "APP_ID"
old_ai_assistant_name="god""dess"
if grep -Fqi -- "${old_ai_assistant_name}" "${ROOT_DIR}/compose.yml"; then
  fail "compose.yml should not contain old AI assistant naming"
fi

assert_contains ".dockerignore" "data"
assert_contains ".dockerignore" "**/node_modules"
assert_contains ".dockerignore" "**/dist"

assert_contains ".env.example" "PUBLIC_HOSTNAME=localhost"
assert_contains ".env.example" "CLIENT_HTTPS_PORT=443"
assert_contains ".env.example" "ADMIN_HTTPS_PORT=1443"
assert_contains ".env.example" "POSTGRES_DB=magic-chat"
assert_contains ".env.example" "POSTGRES_USER=magic-chat"
assert_contains ".env.example" "POSTGRES_HOST=postgres"
assert_contains ".env.example" "ADMIN_PASSWORD=change-me"
assert_contains ".env.example" "AWS_ENDPOINT_URL_S3=https://s3.example.com"
assert_contains ".env.example" "AWS_REGION=us-east-1"
assert_contains ".env.example" "S3_BOOTSTRAP_ENABLED=false"
assert_contains ".env.example" "S3_FORCE_PATH_STYLE=false"
assert_contains ".env.example" "PUBLIC_ASSETS_BUCKET=magicchat-public"
assert_contains ".env.example" "PRIVATE_ASSETS_BUCKET=magicchat-private"
assert_contains ".env.example" "TEMPORARY_ASSETS_BUCKET=magicchat-temporary"
assert_contains ".env.example" "TEMPORARY_ASSETS_EXPIRE_DAYS=180"
assert_contains ".env.example" "S3_ABORT_MULTIPART_DAYS=7"
assert_contains ".env.example" "PUBLIC_ASSETS_HOSTNAME=public-assets.localhost"
assert_contains ".env.example" "PRIVATE_ASSETS_HOSTNAME=private-assets.localhost"
assert_contains ".env.example" "TEMPORARY_ASSETS_HOSTNAME=temporary-assets.localhost"
assert_contains ".env.example" "ASSISTANT_WEBSOCKET_URL=ws://server:20080/api/app/ws"
assert_contains ".env.example" "AGENT_MAX_TURNS=50"
assert_contains ".env.example" "AGENT_MAX_SESSIONS=1000"
assert_contains ".env.example" "LLM_BASE_URL=https://api.example.com"
assert_contains ".env.example" "LLM_API_KEY=change-me"
assert_contains ".env.example" "LLM_MODEL_NAME=change-me"
assert_contains ".env.example" "MCP_GATEWAY_URL=https://mcp.example.com/mcp"
assert_contains ".env.example" "MCP_GATEWAY_KEY=change-me"

assert_contains "deploy/caddy/Dockerfile" "FROM caddy:2-alpine"
assert_contains "deploy/caddy/Dockerfile" "COPY deploy/caddy/Caddyfile /etc/caddy/Caddyfile"
assert_contains "deploy/caddy/Dockerfile" "openssl req -x509"
assert_contains "deploy/caddy/Dockerfile" "caddy validate"
assert_contains "deploy/caddy/Dockerfile" "EXPOSE 80 443 1443"
assert_not_contains "deploy/caddy/Dockerfile" "ghcr.1ms.run"
assert_not_contains "deploy/caddy/Dockerfile" "docker.1ms.run"

assert_contains "deploy/caddy/Caddyfile" "fallback_sni bootstrap.invalid"
assert_contains "deploy/caddy/Caddyfile" "auto_https disable_redirects"
assert_contains "deploy/caddy/Caddyfile" 'http://{$PUBLIC_HOSTNAME:localhost}'
assert_contains "deploy/caddy/Caddyfile" 'redir https://{$PUBLIC_HOSTNAME:localhost}:{$CLIENT_HTTPS_PORT:443}{uri} 301'
assert_contains "deploy/caddy/Caddyfile" 'https://{$PUBLIC_HOSTNAME:localhost}'
assert_contains "deploy/caddy/Caddyfile" 'https://{$PUBLIC_HOSTNAME:localhost}:1443'
assert_contains "deploy/caddy/Caddyfile" "protocols h1 h2"
assert_contains "deploy/caddy/Caddyfile" "handle /gateway-healthz"
assert_contains "deploy/caddy/Caddyfile" "@client_api path /api/client/* /api/app/*"
assert_contains "deploy/caddy/Caddyfile" "handle /api/*"
assert_contains "deploy/caddy/Caddyfile" "reverse_proxy server:20080"
assert_contains "deploy/caddy/Caddyfile" "max_size 201MiB"
assert_contains "deploy/caddy/Caddyfile" "root * /srv/client"
assert_contains "deploy/caddy/Caddyfile" "root * /srv/admin"
assert_contains "deploy/caddy/Caddyfile" "try_files {path} /index.html"
assert_contains "deploy/caddy/Caddyfile" "encode zstd gzip"
assert_contains "deploy/caddy/Caddyfile" "output file /var/log/caddy/caddy.log"
assert_contains "deploy/caddy/Caddyfile" "output file /var/log/caddy/client-access.log"
assert_contains "deploy/caddy/Caddyfile" "output file /var/log/caddy/admin-access.log"
assert_contains "deploy/caddy/Caddyfile" "roll_size 10MiB"
assert_contains "deploy/caddy/Caddyfile" "roll_keep 5"
assert_contains "deploy/caddy/Caddyfile" "roll_keep_for 720h"
assert_not_contains "deploy/caddy/Caddyfile" "output stdout"
assert_contains "deploy/caddy/Caddyfile" "tls /etc/caddy/bootstrap/tls.crt /etc/caddy/bootstrap/tls.key"
assert_not_contains "deploy/caddy/Caddyfile" '${ADMIN_HTTPS_PORT}'
assert_not_contains "deploy/caddy/Caddyfile" "mygod"

assert_contains "server/Dockerfile" "go build"
assert_contains "server/Dockerfile" "COPY server/migrations"
assert_contains "server/Dockerfile" "COPY api-docs"
assert_not_contains "server/Dockerfile" "config.example.yaml"
assert_contains "assistant/Dockerfile" "go build"
assert_contains "assistant/internal/config/config.go" 'AIAssistantAppID        = "00000000-0000-0000-0000-000000000001"'
assert_contains "assistant/internal/config/config.go" "DefaultAgentMaxTurns    = 50"
assert_contains "assistant/internal/config/config.go" "DefaultAgentMaxSessions = 1000"
assert_contains "server/internal/appregistry/ai_assistant.go" 'AIAssistantAppID          = "00000000-0000-0000-0000-000000000001"'

assert_contains "deploy/caddy/Dockerfile" "pnpm build"
assert_contains "deploy/caddy/Dockerfile" "ARG CLIENT_BUILD_COMMIT=development"
assert_contains "deploy/caddy/Dockerfile" 'ENV VITE_CLIENT_BUILD_COMMIT=${CLIENT_BUILD_COMMIT}'
assert_contains "deploy/caddy/Dockerfile" "COPY --from=client-build /src/client-web/dist /srv/client"
assert_contains "deploy/caddy/Dockerfile" "COPY --from=admin-build /src/admin-web/dist /srv/admin"
assert_contains "deploy/caddy/Dockerfile" "COPY admin-web/public/assets/avatars/builtin"

assert_contains ".github/workflows/docker.yml" "ghcr.io"
assert_contains ".github/workflows/docker.yml" "server/Dockerfile"
assert_contains ".github/workflows/docker.yml" "assistant/Dockerfile"
assert_contains ".github/workflows/docker.yml" "image: caddy"
assert_contains ".github/workflows/docker.yml" "deploy/caddy/Dockerfile"
assert_not_contains ".github/workflows/docker.yml" "deploy/nginx/Dockerfile"
assert_contains ".github/workflows/docker.yml" "docker/build-push-action@v7"
assert_contains ".github/workflows/docker.yml" 'build_args: CLIENT_BUILD_COMMIT=${{ github.sha }}'
assert_contains ".github/workflows/docker.yml" 'build-args: ${{ matrix.build_args }}'

assert_contains "deploy/caddy/Caddyfile" "@client_version path /version.json"
assert_contains "deploy/caddy/Caddyfile" 'Cache-Control "no-store, no-cache, must-revalidate"'
assert_contains "deploy/caddy/Caddyfile" "@client_document"
assert_contains "deploy/caddy/Caddyfile" 'not path /assets/* /version.json'

echo "deploy config check passed"
