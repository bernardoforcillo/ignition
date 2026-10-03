# mailer

Transactional email for Go services. Templates are written in React with
[react.email](https://react.email) in `packages/mailer`, exported to static HTML
and plain text, and embedded here, so Go renders and sends email with no Node
runtime. Delivery goes through a `Sender`; [Resend](https://resend.com) is the
shipped one.

```
packages/mailer (react.email)  --pnpm gen:email-->  go-packages/mailer/templates/*.html|txt|json (generated)
                                                        |  go:embed
                                         mailer.Mailer  --Sender-->  resend.Client  -->  Resend API
```

## Generated templates

The embedded HTML/text is not in git. After a fresh clone run `pnpm gen:email`
once before `go build` or `go test`; without it `LoadTemplates` fails with
`ErrTemplatesNotGenerated`. Re-run it after editing a template.

## Use

```go
sender, err := resend.New(resend.Config{APIKey: os.Getenv("RESEND_API_KEY")}) // read env in main.go
m, err := mailer.New(sender, mailer.Config{
	From:        "Ignition <hello@example.com>",
	CompanyName:  "Ignition",
	AppURL:       "https://app.example.com",
	AssetBaseURL: "https://app.example.com", // serves <origin>/static/... (defaults to AppURL)
}, slog.Default())

err = m.SendVerification(ctx, "ada@example.com", "https://app.example.com/verify-email?token=...")
err = m.SendTemplate(ctx, "welcome", "ada@example.com", map[string]string{"Url": "https://app.example.com"})
```

`*mailer.Mailer` satisfies the `Mailer` ports of `go-packages/identity`
(`SendVerification`, `SendAccountExists`, `SendPasswordReset`, `SendInvitation`) directly: pass it to
`auth.NewService` and `workspace.NewService`.

Local development without a key: `mailer.NewLogSender(logger)` logs the
recipient and subject (never the body, which carries single-use links).

## Templates

The eleven templates come from the react.email
[Barebone collection](https://demo.react.email/preview/01-Barebone/welcome)
(`activation`, `welcome`, `password-reset`, `subscription-confirmation`,
`subscription-update`, `feature-announcement`, `product-update`, `text-only`)
plus three in the same style: `account-exists`, `workspace-invitation` and `trial-ending`.
`SendVerification` uses `activation`, `SendPasswordReset` uses `password-reset`,
`SendTrialEnding` uses `trial-ending` (sent by the gateway's `trials.remind` job).

`CompanyName` and `AssetBaseUrl` are filled in by the `Mailer` from its `Config`
and cannot be overridden by a caller. Everything else is passed in the data map
(`Url`, `UserName`, `PlanName`, ...; `Variables(name)` lists them).

The email images live in `packages/mailer/src/emails/static` and must be served
at `<AssetBaseURL>/static/...` (for example from the web app's public folder or a
CDN). Footer copy (tagline, address, social links) is in
`packages/mailer/src/emails/brand.ts`: replace the placeholders.

## Add or change a template

1. Write the component in `packages/mailer/src/emails/<name>.tsx` and register it
   in `src/templates.ts` with its subject and variables.
2. Preview it: `pnpm --filter @ignition/mailer dev` (http://localhost:3030).
3. Export: `pnpm gen:email`. The files under `go-packages/mailer/templates/` are
   **generated and not committed**; the gateway Dockerfile exports them in a Node
   stage.
4. Add a typed method on `Mailer` if a service sends it, or call
   `SendTemplate(ctx, name, to, map[string]string{...})`.

Variables are PascalCase in Go (`workspaceName` becomes `WorkspaceName`, `url` becomes `Url`).

## Safety

- Every declared variable must be supplied (`ErrMissingVariable`).
- Variables ending in `Link` or `Url` must be absolute http(s) URLs
  (`ErrInvalidURL`), so a `javascript:` link cannot reach an email.
- Values are HTML-escaped in the HTML body; the subject is collapsed to one line
  (no header injection through a workspace name).
- The Resend client classifies failures: `resend.ErrTransient` (429, 5xx,
  network: retry) and `resend.ErrRejected` (validation, auth: do not).
  Pass `SendTemplateOnce` an idempotency key when your caller retries.

## Only Go sends

Email leaves the system from this module and nowhere else. `packages/mailer` has
no sender, no provider SDK and no API key: it only renders templates at build
time. A TypeScript app that needs an email calls a Go service (an RPC that ends in
the `Mailer`), so there is one place that holds the provider key, escapes data,
validates links and retries.

## Tests

`go test -short -race ./...` needs no network: Resend is exercised against
`httptest`, and `mailertest.Recorder` captures messages.
