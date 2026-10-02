# mailer

Transactional email for Go services. Templates are written in React with
[react.email](https://react.email) in `packages/mailer`, exported to static HTML
and plain text, and embedded here, so Go renders and sends email with no Node
runtime. Delivery goes through a `Sender`; [Resend](https://resend.com) is the
shipped one.

```
packages/mailer (react.email)  --pnpm export-->  go-packages/mailer/templates/*.html|txt|json
                                                        |  go:embed
                                         mailer.Mailer  --Sender-->  resend.Client  -->  Resend API
```

## Use

```go
sender, err := resend.New(resend.Config{APIKey: os.Getenv("RESEND_API_KEY")}) // read env in main.go
m, err := mailer.New(sender, mailer.Config{
	From:   "Ignition <hello@example.com>",
	AppURL: "https://app.example.com",
}, slog.Default())

err = m.SendVerification(ctx, "ada@example.com", "https://app.example.com/verify?token=...")
```

`*mailer.Mailer` satisfies the `Mailer` ports of `go-packages/identity`
(`SendVerification`, `SendAccountExists`, `SendInvitation`) directly: pass it to
`auth.NewService` and `workspace.NewService`.

Local development without a key: `mailer.NewLogSender(logger)` logs the
recipient and subject (never the body, which carries single-use links).

## Add or change a template

1. Write the component in `packages/mailer/src/emails/<name>.tsx` and register it
   in `src/templates.ts` with its subject and variables.
2. Preview it: `pnpm --filter @ignition/mailer dev` (http://localhost:3030).
3. Export: `pnpm --filter @ignition/mailer export`, and commit the files under
   `go-packages/mailer/templates/`. `check:export` fails if they are stale.
4. Add a typed method on `Mailer` if a service sends it, or call
   `SendTemplate(ctx, name, to, map[string]string{...})`.

Variables are PascalCase in Go (`workspaceName` becomes `WorkspaceName`).

## Safety

- Every declared variable must be supplied (`ErrMissingVariable`).
- Variables ending in `Link` or `Url` must be absolute http(s) URLs
  (`ErrInvalidURL`), so a `javascript:` link cannot reach an email.
- Values are HTML-escaped in the HTML body; the subject is collapsed to one line
  (no header injection through a workspace name).
- The Resend client classifies failures: `resend.ErrTransient` (429, 5xx,
  network: retry) and `resend.ErrRejected` (validation, auth: do not).
  Pass `SendTemplateOnce` an idempotency key when your caller retries.

## Node apps

`@ignition/mailer` also exports `createMailer({ apiKey, from })`, which renders
the same templates and sends through the Resend SDK for Node/Edge code.

## Tests

`go test -short -race ./...` needs no network: Resend is exercised against
`httptest`, and `mailertest.Recorder` captures messages.
