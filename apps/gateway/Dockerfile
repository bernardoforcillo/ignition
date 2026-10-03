# syntax=docker/dockerfile:1

# BUILD CONTEXT IS THE REPOSITORY ROOT, not apps/gateway:
#
#   docker build -f apps/gateway/Dockerfile -t gateway .
#
# The gateway depends on the sibling Go modules under go-packages/ (wired
# with relative `replace` directives in apps/gateway/go.mod), so the image
# build needs them in the context. go.work is deliberately not copied: the
# replace directives alone resolve everything (GOWORK=off), which is also
# what proves the module builds without the workspace file.
#
# The generated protobuf code (go-packages/proto/gen) is checked into the repo,
# so this stage only needs the Go toolchain — no buf/protoc to produce an image.
# Email templates: the React/react.email sources are exported to the static HTML the Go mailer
# embeds. The output is generated here and never committed.
FROM node:26-alpine AS emails
WORKDIR /src
# Node 25+ no longer ships corepack; keep in step with "packageManager" in package.json.
ARG PNPM_VERSION=11.5.0
RUN npm install --global "pnpm@${PNPM_VERSION}"
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./
COPY apps/web/package.json apps/web/
COPY packages packages
RUN pnpm install --frozen-lockfile --filter @ignition/mailer... \
	&& pnpm --filter @ignition/mailer export

FROM golang:1.27-alpine AS build
WORKDIR /src
ENV GOWORK=off CGO_ENABLED=0 GOOS=linux

# Module manifests first, so `go mod download` is cached until a
# dependency actually changes. billing, mailer and storage are stdlib-only and
# have no go.sum.
COPY go-packages/billing/go.mod go-packages/billing/go.mod
COPY go-packages/database/go.mod go-packages/database/go.sum go-packages/database/
COPY go-packages/features/go.mod go-packages/features/go.sum go-packages/features/
COPY go-packages/identity/go.mod go-packages/identity/go.sum go-packages/identity/
COPY go-packages/jobs/go.mod go-packages/jobs/go.sum go-packages/jobs/
COPY go-packages/mailer/go.mod go-packages/mailer/go.mod
COPY go-packages/proto/go.mod go-packages/proto/go.sum go-packages/proto/
COPY go-packages/storage/go.mod go-packages/storage/go.mod
COPY go-packages/telemetry/go.mod go-packages/telemetry/go.sum go-packages/telemetry/
COPY apps/gateway/go.mod apps/gateway/go.sum apps/gateway/
RUN go -C apps/gateway mod download

COPY go-packages go-packages
COPY --from=emails /src/go-packages/mailer/templates go-packages/mailer/templates
COPY apps/gateway apps/gateway
RUN go -C apps/gateway build -trimpath -ldflags="-s -w" -o /out/gateway .

# Runtime stage: static binary on a distroless base, running as the
# image's built-in non-root user. No shell, no package manager — the
# smallest attack surface reasonable for a template default.
FROM gcr.io/distroless/static-debian12:nonroot
USER nonroot:nonroot
COPY --from=build /out/gateway /gateway

EXPOSE 8080
ENTRYPOINT ["/gateway"]
