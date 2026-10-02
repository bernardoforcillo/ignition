# @ignition/e2e

Playwright tests that drive the **real** stack in a browser: the Go gateway, a Postgres database and the
production build of `apps/web`. Only the two third parties are faked, because they cannot be reached from
a test: Resend (email) and Stripe (checkout, portal, webhooks), plus a PostHog capture host.

## What `playwright test` starts

Global setup (`src/harness/global-setup.ts`) brings the stack up in order and tears it down at the end.
Every port is picked dynamically, so runs never collide with each other or with a dev server.

1. **Database**: `E2E_DATABASE_URL` is dropped and recreated (empty, so migrations run from scratch).
2. **Fake Resend** (`POST /emails`; tests read mail through `src/support/mail.ts`).
3. **Fake Stripe API** (`POST /v1/checkout/sessions`, `/v1/billing_portal/sessions`; records the form each
   call carried). Webhooks are posted by the tests to the real gateway, signed with `whsec_test`.
4. **Fake PostHog** host (records every request, decodes bodies).
5. **Gateway**: `go build` into `.run/`, then run with `RESEND_BASE_URL` / `STRIPE_API_BASE_URL` pointing at
   the fakes, billing prices `price_pro=plan:pro,price_extra=addon:extra-api-calls`, and `/readyz` awaited.
6. **Web**, built **twice** (`VITE_*` is baked in at build time) and served by `vite preview`, which proxies
   the Connect RPCs to the gateway so the browser sees one origin:
   - `chromium` project: no analytics key.
   - `analytics` project: `VITE_POSTHOG_KEY` and `VITE_POSTHOG_HOST` set to the fake PostHog.

Logs of every process are in `e2e/.run/*.log` (git-ignored); read `gateway.log` first when a test fails.

## Running

```sh
pnpm install
pnpm gen:email                      # once: the gateway embeds the generated email templates
pnpm test:e2e                       # = pnpm --filter @ignition/e2e test
pnpm --filter @ignition/e2e test:headed
pnpm --filter @ignition/e2e test:debug          # Playwright inspector, one worker
pnpm --filter @ignition/e2e exec playwright test --project=analytics
pnpm --filter @ignition/e2e exec playwright test tests/billing.spec.ts -g "checkout"
pnpm --filter @ignition/e2e exec playwright show-report   # after a CI-style run
```

Requirements: Go (the gateway is built from source), `psql` on `PATH` (or `E2E_PSQL`), a Postgres 16 server
and a Chromium.

| Variable | Default | Meaning |
|---|---|---|
| `E2E_DATABASE_URL` | `postgres://postgres@/ignition_e2e?host=/var/tmp/igpg&port=55432&sslmode=disable` | Database the gateway uses. **Dropped on every run**: the name must contain `e2e`. Point it at a service container in CI. The server's `postgres` maintenance database must be reachable with the same credentials. |
| `E2E_PSQL` | `psql` (or PG16's default path) | `psql` binary used to drop/create the database |
| `E2E_CHROMIUM_PATH` | bundled browser, else `$PLAYWRIGHT_BROWSERS_PATH/chromium` | Browser executable to launch |
| `E2E_WORKERS` | 3 locally, 2 in CI | Parallel workers; any value is valid, each test owns its data |
| `CI` | unset | `retries: 1`, GitHub + HTML reporters, `forbidOnly` |

Browsers: use `pnpm exec playwright install chromium` once on a fresh machine, or set `E2E_CHROMIUM_PATH` /
`PLAYWRIGHT_BROWSERS_PATH` when a Chromium is pre-installed (do not install over a managed image).

Existing Postgres, for example:

```sh
E2E_DATABASE_URL='postgres://postgres:postgres@localhost:5432/ignition_e2e?sslmode=disable' pnpm test:e2e
```

## CI

```yaml
services:
  postgres:
    image: postgres:16
    env: { POSTGRES_PASSWORD: postgres }
    ports: ["5432:5432"]
    options: >-
      --health-cmd "pg_isready -U postgres" --health-interval 5s --health-retries 10
steps:
  - uses: actions/setup-go / pnpm / node
  - run: pnpm install --frozen-lockfile
  - run: pnpm gen:email
  - run: pnpm --filter @ignition/e2e exec playwright install --with-deps chromium
  - run: pnpm test:e2e
    env:
      E2E_DATABASE_URL: postgres://postgres:postgres@localhost:5432/ignition_e2e?sslmode=disable
  - uses: actions/upload-artifact@v4   # if: failure()
    with: { name: e2e, path: "e2e/playwright-report\ne2e/test-results\ne2e/.run/*.log" }
```

## How the tests stay independent and deterministic

- **Own data.** Every test creates its own users with `uniqueEmail()` (counter + random suffix) and its own
  browser contexts (`isolated()` / `workspaceUser()` fixtures in `src/support/test.ts`).
- **Own rate-limit budget.** The gateway limits sign-ups (10/h) and logins per client IP, and every test runs
  from loopback. The gateway therefore trusts the loopback hop (`TRUSTED_PROXIES`) and each context sends its
  own `X-Forwarded-For`, which the preview proxy forwards.
- **No fixed sleeps.** Waits are Playwright auto-waiting, web-first assertions, and `expect.poll` for the
  asynchronous email (`waitForLink`).
- **Accessible selectors only** (roles, labels, names). No test ids.
- **One page at a time per context**: a background tab's animation frames are throttled, which makes
  Playwright's "stable" checks take seconds. Open the second page after finishing with the first, or use a
  second context.
- Traces are kept on the first retry (`retries: 1` in CI only). Open one with `playwright show-trace`.
- The `analytics` project fakes `navigator.webdriver = false`: posthog-js treats automated browsers as bots and
  silently drops their events, which would make "nothing leaks" assertions vacuous.

## Layout

```
playwright.config.ts      projects: chromium, analytics; picks the two web ports
src/harness/              global setup, the fakes, process and database helpers
src/support/              mail, signed Stripe webhooks, PostHog captures, users and fixtures
tests/                    registration, password-reset, invitations, billing, gdpr, auth-hygiene, analytics
```

## Known limits

- Session replay is not exercised: the fake PostHog does not serve the recorder script, so a recording that
  captured the page URL (rrweb `Meta` events) is not covered by the token assertions.
- Stripe is a fake of the two REST calls the adapter makes; the hosted pages are placeholders.
- Email is checked through its plain-text part (the verify/reset/invite links); HTML rendering is covered by
  the mailer's own tests.
