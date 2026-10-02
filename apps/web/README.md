# @ignition/web

Vite + React 19 + TanStack Router/Query + Tailwind v4 front end for the SaaS template:
sign-up and sign-in, onboarding, workspaces, members, billing and account settings.
Shared UI lives in `@ignition/components`; the typed API clients come from `@ignition/proto`.

## Scripts

| Script | What it does |
|---|---|
| `pnpm --filter @ignition/web dev` | Vite dev server with the API proxy |
| `pnpm --filter @ignition/web build` | Production build to `dist/` |
| `pnpm --filter @ignition/web preview` | Serve the build (same proxy) |
| `pnpm --filter @ignition/web test` | Unit tests (vitest) |
| `pnpm --filter @ignition/web typecheck` | `tsc --noEmit` |

## Environment

| Variable | Default | Used by | Meaning |
|---|---|---|---|
| `VITE_API_PROXY_TARGET` | `http://localhost:8080` | Vite dev/preview server | Gateway that `/saas.v1.*` and `/gateway.v1.*` requests are proxied to |
| `VITE_API_URL` | unset (page origin) | browser build | Gateway origin to call directly. Leave unset to use the proxy / Ingress, which needs no CORS |

Put overrides in `apps/web/.env.local`.

## How the API is reached

Connect RPCs are `POST /<package>.<Service>/<Method>`. `src/lib/rpc` builds one transport on the
page origin; in development the Vite proxy forwards `/saas.v1.*` and `/gateway.v1.*` to the
gateway, in production the Ingress does. `BillingService` only exists when billing is configured:
`not_found`/`unimplemented` from it renders "Billing is not enabled".

## Routes

| Path | Access | Screen |
|---|---|---|
| `/` | any | redirects to `/app` |
| `/login?redirect=` | signed out | Sign in; returns to `redirect` (same-origin paths only) |
| `/signup` | signed out | Create account, then "Check your email" |
| `/verify-email?token=` | any | Redeems the verification token |
| `/forgot-password` | any | Send reset link |
| `/reset-password?token=` | any | Set a new password |
| `/invite/accept?token=` | signed in (else `/login?redirect=...`) | Accepts the invitation, then opens `/app` |
| `/onboarding` | signed in, no workspace | Create the first workspace |
| `/app` | signed in + workspace | Overview, with workspace switcher and nav |
| `/app/members` | " | Member list, invite form |
| `/app/billing` | " | Current plan, plans/add-ons, checkout, portal |
| `/app/settings` | " | Account email, data export, delete account |

Guards are TanStack Router `beforeLoad` functions in `src/lib/route-guards`.

## Auth model

State lives in `src/stores/auth` (zustand).

- The **access token is kept in memory only**. The **refresh token is in localStorage**
  (`ignition-refresh-token`) so a reload can sign back in silently. An httpOnly cookie would be
  stronger against XSS, but the API returns both tokens in the JSON body. The refresh token rotates
  on every use and is revoked on logout.
- On start the first guard calls `restore()`, a silent refresh from the stored token.
- A Connect interceptor (`src/lib/rpc/auth-interceptor.ts`) adds `Authorization: Bearer ...`; on
  `unauthenticated` it refreshes **once** (concurrent callers share one request) and replays the
  call. `AuthService` calls are never decorated or replayed.
- Logout revokes the refresh token server-side, then clears tokens, the selected workspace id
  (`ignition-workspace-id`) and the query cache.
- Server errors are mapped to friendly text in `src/lib/errors`; raw errors and tokens are never shown.
