FROM node:24-bookworm-slim AS client-build

WORKDIR /src/client-web

RUN corepack enable && corepack prepare pnpm@10.32.1 --activate

COPY client-web/package.json client-web/pnpm-lock.yaml client-web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

COPY client-web/ ./
RUN pnpm build

FROM node:24-bookworm-slim AS admin-build

WORKDIR /src/admin-web

RUN corepack enable && corepack prepare pnpm@10.32.1 --activate

COPY admin-web/package.json admin-web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY admin-web/ ./
RUN pnpm build

FROM nginx:stable-alpine

COPY deploy/nginx/nginx.conf /etc/nginx/nginx.conf
COPY --from=client-build /src/client-web/dist /usr/share/nginx/client
COPY --from=admin-build /src/admin-web/dist /usr/share/nginx/admin

EXPOSE 443 8443
