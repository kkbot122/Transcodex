FROM node:24-alpine AS build

WORKDIR /app

RUN corepack enable

COPY frontend/demo/package.json frontend/demo/pnpm-lock.yaml frontend/demo/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

COPY frontend/demo ./
RUN pnpm build

FROM nginx:1.27-alpine

COPY frontend/demo/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html

EXPOSE 80
