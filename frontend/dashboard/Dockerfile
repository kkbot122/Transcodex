FROM node:24-alpine AS build

WORKDIR /app

RUN corepack enable

COPY frontend/dashboard/package.json frontend/dashboard/pnpm-lock.yaml frontend/dashboard/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

COPY frontend/dashboard ./
RUN pnpm build

FROM nginx:1.27-alpine

COPY frontend/dashboard/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html

EXPOSE 80
