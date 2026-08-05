# syntax=docker/dockerfile:1.7
FROM node:24.14.0-alpine AS dependencies

RUN npm install --global pnpm@11.9.0
WORKDIR /app
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

FROM dependencies AS development
COPY --chown=node:node frontend/ ./
USER node
EXPOSE 5173
CMD ["pnpm", "dev"]

FROM dependencies AS build
COPY frontend/ ./
RUN pnpm build

FROM nginxinc/nginx-unprivileged:1.29-alpine AS runtime
COPY frontend/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build --chown=101:101 /app/dist /usr/share/nginx/html
USER 101:101
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=5 CMD ["wget", "-q", "-O", "/dev/null", "http://127.0.0.1:8080/healthz"]
