# syntax=docker/dockerfile:1
# Build from the repo root: docker build -f apps/web/Dockerfile .

FROM node:26-alpine AS build
WORKDIR /src
# Node 25+ no longer ships corepack; keep in step with "packageManager" in package.json.
ARG PNPM_VERSION=11.5.0
RUN npm install --global "pnpm@${PNPM_VERSION}"
COPY . .
RUN pnpm install --frozen-lockfile --ignore-scripts \
	&& pnpm --filter @ignition/web... build

FROM nginxinc/nginx-unprivileged:alpine
USER nginx
COPY apps/web/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/apps/web/dist /usr/share/nginx/html
EXPOSE 8080
