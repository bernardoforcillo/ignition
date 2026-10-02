# @ignition/mailer

react.email templates for the product's transactional email, plus a Resend sender
for Node apps.

```sh
pnpm --filter @ignition/mailer dev           # preview at http://localhost:3030
pnpm --filter @ignition/mailer export        # write go-packages/mailer/templates
pnpm --filter @ignition/mailer check:export  # fail if the committed export is stale
```

- `src/emails/` — the templates (`layout.tsx` is the shared chrome; brand tokens
  mirror `@ignition/components/theme.css`).
- `src/templates.ts` — the registry: component, subject and variables per email.
- `src/send.ts` — `createMailer({ apiKey, from }).send("verify-email", { link }, { to })`.
- `scripts/export.tsx` — renders each template with `{{.Variable}}` placeholders
  for the Go mailer (see `go-packages/mailer/README.md`).
