module github.com/bernardoforcillo/ignition/apps/gateway

go 1.27.0

require (
	connectrpc.com/connect v1.21.0
	github.com/bernardoforcillo/authlayer v0.1.0
	github.com/bernardoforcillo/drops v0.6.0
	github.com/bernardoforcillo/featurelayer v0.0.0-20260901212336-9d484ab9aac2
	github.com/bernardoforcillo/ignition/go-packages/billing v0.0.0
	github.com/bernardoforcillo/ignition/go-packages/database v0.0.0
	github.com/bernardoforcillo/ignition/go-packages/features v0.0.0
	github.com/bernardoforcillo/ignition/go-packages/identity v0.0.0
	github.com/bernardoforcillo/ignition/go-packages/mailer v0.0.0
	github.com/bernardoforcillo/ignition/go-packages/proto v0.0.0
	github.com/bernardoforcillo/ignition/go-packages/telemetry v0.0.0
	github.com/buildwithgo/amaro v0.4.1-0.20260131070744-089fe184ddeb
)

require (
	github.com/andybalholm/brotli v1.1.1 // indirect
	github.com/goccy/go-json v0.10.6 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.11.0 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/posthog/posthog-go v1.32.0 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)

replace github.com/bernardoforcillo/ignition/go-packages/billing => ../../go-packages/billing

replace github.com/bernardoforcillo/ignition/go-packages/database => ../../go-packages/database

replace github.com/bernardoforcillo/ignition/go-packages/features => ../../go-packages/features

replace github.com/bernardoforcillo/ignition/go-packages/identity => ../../go-packages/identity

replace github.com/bernardoforcillo/ignition/go-packages/mailer => ../../go-packages/mailer

replace github.com/bernardoforcillo/ignition/go-packages/proto => ../../go-packages/proto

replace github.com/bernardoforcillo/ignition/go-packages/telemetry => ../../go-packages/telemetry
