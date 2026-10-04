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
| `/` | any | Landing page: hero, features, pricing (from the public `ListPrices`), FAQ. Signed-in visitors see "Open app" |
| `/docs`, `/docs/<slug>` | any | Documentation; `/docs` redirects to `/docs/overview` |
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

Every route component is loaded lazily (`lazyRouteComponent`), so the entry chunk holds only the
router, the API clients and the shell; `react-markdown` ships only in the docs chunk and `posthog-js`
is fetched only after a visitor accepts analytics.

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

## Landing page and docs

- **Pricing copy** lives in `src/features/landing/plans.ts`, keyed by plan id. The API lists which
  plans can be bought (`kind`, `id`) but not their prices, so names, taglines, bullets and price
  text are marketing copy to replace. A plan id without copy renders with a derived title and no
  price; with billing disabled only the Free card shows.
- **Docs** are Markdown files in `src/docs/<slug>.md`, rendered by `react-markdown` (no raw HTML;
  links may only be `/docs/<slug>[#anchor]` or `https://`). Add a page by adding the file, an entry
  in `src/features/docs/manifest.ts` (that order is the sidebar and prev/next order) and a line in
  `public/sitemap.xml`; a unit test fails if any of the three is missing.
- **SEO**: `useDocumentMeta` sets the title and description per route; `index.html` carries the
  Open Graph tags. `public/robots.txt` and `public/sitemap.xml` use `https://example.com` as a
  **placeholder origin**: replace it with your public origin before deploying.
- **Analytics**: `pricing_cta_clicked` (plan id) and `docs_page_viewed` (slug); no URLs or queries.
