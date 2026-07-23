FROM node:24-bookworm-slim AS client-build

WORKDIR /src/client-web

ARG CLIENT_BUILD_COMMIT=development
ENV VITE_CLIENT_BUILD_COMMIT=${CLIENT_BUILD_COMMIT}

RUN corepack enable && corepack prepare pnpm@10.32.1 --activate

COPY client-web/package.json client-web/pnpm-lock.yaml client-web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

COPY client-web/ ./
COPY admin-web/public/assets/avatars/builtin /src/admin-web/public/assets/avatars/builtin
RUN pnpm build

FROM node:24-bookworm-slim AS admin-build

WORKDIR /src/admin-web

RUN corepack enable && corepack prepare pnpm@10.32.1 --activate

COPY admin-web/package.json admin-web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY admin-web/ ./
RUN pnpm build

FROM alpine:3.22 AS bootstrap-certificate

RUN apk add --no-cache openssl \
  && mkdir -p /out \
  && openssl req -x509 -nodes -newkey rsa:2048 -sha256 -days 3650 \
    -subj "/CN=bootstrap.invalid" \
    -addext "subjectAltName=DNS:bootstrap.invalid" \
    -keyout /out/tls.key \
    -out /out/tls.crt \
  && chmod 600 /out/tls.key \
  && chmod 644 /out/tls.crt

FROM caddy:2-alpine

COPY deploy/caddy/Caddyfile /etc/caddy/Caddyfile
COPY --from=bootstrap-certificate /out /etc/caddy/bootstrap
COPY --from=client-build /src/client-web/dist /srv/client
COPY --from=admin-build /src/admin-web/dist /srv/admin

RUN caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile

EXPOSE 80 443 1443
