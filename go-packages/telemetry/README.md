# telemetry

Service-side observability, split on purpose:

- **Everything goes to stdout as JSON**, which the platform collects: Cloud Logging
  on GCP, or Loki or anything else. `FormatGCP` writes `severity`, `message` and
  `timestamp` so Cloud Logging's severity filters and alerts work with no parsing
  rule.
- **Only the critical things go to [PostHog](https://posthog.com)**: `slog` records
  at **Error** and above become `$exception` events with a stack trace (Info and
  Warn never leave the process), plus the few events the server alone can vouch
  for. Product analytics lives in the browser (`apps/web`), not here.

| You get | How |
|---|---|
| JSON logs | always, to the writer you pass to `New`; `LogFormat: telemetry.FormatGCP` for Cloud Logging |
| Error tracking | with an API key, Error+ records reach PostHog as `$exception`; `CaptureException` sends a handled error; `CaptureLevel` can only be raised |
| Server events | `Capture(ctx, accountID, "subscription_changed", props)`: keep these to business facts the browser cannot know (a webhook-driven plan change, an account erasure) |
| Local development | no key: everything is a safe no-op and logs still print |

```go
tel, err := telemetry.New(telemetry.Config{
	APIKey:      cfg.PostHogAPIKey, // read env in your config package, not here
	Host:        cfg.PostHogHost,   // default https://eu.i.posthog.com (EU region)
	ServiceName: "gateway",
	Environment: cfg.Environment,
}, os.Stdout)
defer tel.Close() // flushes pending events
slog.SetDefault(tel.Logger())

// attribute errors logged while serving a request to the signed-in account:
ctx = telemetry.WithDistinctID(ctx, accountID)
slog.ErrorContext(ctx, "checkout failed", "error", err)
```

## Privacy (EU product)

- The distinct id is the **account id** (a UUID), never an email or name.
- Event properties must hold no personal data or free text: send a length, a
  category or a boolean, not the raw value.
- An error with no signed-in user is attributed to `service:<name>` with
  `$process_person_profile=false`, so it never creates a person profile.
- Exception titles are the Go error *type*; the message goes in the description.
  Do not put secrets in error messages.
- When an account is erased (GDPR art. 17) also delete its person in PostHog.

## Tests

`go test -short -race ./...` runs against an `httptest` server that stands in for
PostHog; nothing leaves the machine.
