# syntax=docker/dockerfile:1

# Build stage. internal/gen is checked into the repo (see README), so
# this stage only needs the Go toolchain — no buf/protoc required to
# produce a container image.
FROM golang:1.27-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/gateway .

# Runtime stage: static binary on a distroless base, running as the
# image's built-in non-root user. No shell, no package manager — the
# smallest attack surface reasonable for a template default.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/gateway /gateway

EXPOSE 8080
ENTRYPOINT ["/gateway"]
