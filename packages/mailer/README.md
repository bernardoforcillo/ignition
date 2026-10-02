# @ignition/mailer

react.email templates for the product's transactional email, plus a Resend sender
for Node apps.

```sh
pnpm --filter @ignition/mailer dev           # preview at http://localhost:3030
pnpm --filter @ignition/mailer export        # write go-packages/mailer/templates
pnpm --filter @ignition/mailer check:export  # fail if the committed export is stale
```

- `src/emails/` — the templates: the react.email Barebone collection
  (https://demo.react.email/preview/01-Barebone/welcome) plus `account-exists` and
  `workspace-invitation` built on `shell.tsx`. `theme.ts` is the Tailwind config,
  `brand.ts` the footer copy to replace, `static/` the images (served at
  `<origin>/static/...`; the preview server serves them itself).
- `src/templates.ts` — the registry: component, subject and variables per email.
- `src/send.ts` — `createMailer({ apiKey, from }).send("verify-email", { link }, { to })`.
- `scripts/export.tsx` — renders each template with `{{.Variable}}` placeholders
  for the Go mailer (see `go-packages/mailer/README.md`).
