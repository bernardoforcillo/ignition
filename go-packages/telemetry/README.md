# telemetry

Service-side observability on [PostHog](https://posthog.com): structured logs,
error tracking and product events.

| You get | How |
|---|---|
| JSON logs | always, to the writer you pass to `New` |
| Error tracking | with an API key, `slog` records at **Error** level also reach PostHog as `$exception` events with a stack trace; `CaptureException` sends a handled error |
| Product events | `Capture(ctx, accountID, "workspace_created", props)` |
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
