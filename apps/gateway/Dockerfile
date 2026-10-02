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
# internal/gen is checked into the repo (see README), so this stage only
# needs the Go toolchain — no buf/protoc required to produce an image.
FROM golang:1.27-alpine AS build
WORKDIR /src
ENV GOWORK=off CGO_ENABLED=0 GOOS=linux

# Module manifests first, so `go mod download` is cached until a
# dependency actually changes. billing and mailer are stdlib-only and
# have no go.sum.
COPY go-packages/billing/go.mod go-packages/billing/go.mod
COPY go-packages/database/go.mod go-packages/database/go.sum go-packages/database/
COPY go-packages/features/go.mod go-packages/features/go.sum go-packages/features/
COPY go-packages/identity/go.mod go-packages/identity/go.sum go-packages/identity/
COPY go-packages/mailer/go.mod go-packages/mailer/go.mod
COPY apps/gateway/go.mod apps/gateway/go.sum apps/gateway/
RUN go -C apps/gateway mod download

COPY go-packages go-packages
COPY apps/gateway apps/gateway
RUN go -C apps/gateway build -trimpath -ldflags="-s -w" -o /out/gateway .

# Runtime stage: static binary on a distroless base, running as the
# image's built-in non-root user. No shell, no package manager — the
# smallest attack surface reasonable for a template default.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/gateway /gateway

EXPOSE 8080
ENTRYPOINT ["/gateway"]
