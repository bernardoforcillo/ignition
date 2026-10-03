# @ignition/mailer

react.email templates for the product's transactional email. **This package
never sends anything**: it is a design-time tool whose output is generated HTML (never committed),
and email is sent only from Go (`go-packages/mailer`).

```sh
pnpm --filter @ignition/mailer dev   # preview at http://localhost:3030
pnpm gen:email                       # write go-packages/mailer/templates (generated, not committed)
```

- `src/emails/` — the templates: the react.email Barebone collection
  (https://demo.react.email/preview/01-Barebone/welcome) plus `account-exists` and
  `workspace-invitation` built on `shell.tsx`. `theme.ts` is the Tailwind config,
  `brand.ts` the footer copy to replace, `static/` the images (served at
  `<origin>/static/...`; the preview server serves them itself).
- `src/templates.ts` — the registry: component, subject and variables per email.
- `scripts/export.tsx` — renders each template with `{{.Variable}}` placeholders
  for the Go mailer (see `go-packages/mailer/README.md`).
