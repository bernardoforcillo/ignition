# Self-hosting

You run two things: the Go gateway, which is the API, and the web app, a static site that talks to it. The gateway needs a Postgres database to switch the account, workspace and billing features on.

## Requirements

- A Postgres database. The end-to-end tests run against Postgres 16.
- A way to serve the web app's static files and send the paths that start with `/saas.v1.` and `/gateway.v1.` to the gateway, so the browser sees one origin. The Kubernetes manifests use an Ingress for that.
- Optionally a Resend account for email and a Stripe account for billing.

## Gateway environment

Everything is an environment variable. Secrets belong in a Kubernetes Secret, never in a ConfigMap or the repository.

The gateway always needs a route to proxy to:

| Variable | Meaning |
| --- | --- |
| `GATEWAY_ROUTES` or `GATEWAY_UPSTREAM_URL` | At least one is required. Routes are `prefix=url` pairs. |

Setting `DATABASE_URL` switches on the SaaS surface. With it set, these are required:

| Variable | Meaning |
| --- | --- |
| `DATABASE_URL` | Postgres connection string. Migrations run at startup. |
| `AUTH_SECRET` | Signing key for access tokens, at least 32 bytes |
| `APP_URL` | Public URL of the web app, the base of email links and Stripe return URLs |
| `COMPANY_NAME` | Name shown in emails |
| `MAIL_FROM` | Sender address, for example `Ignition <hello@example.com>` |

These are optional:

| Variable | Default | Meaning |
| --- | --- | --- |
| `GATEWAY_LISTEN_ADDR` | `:8080` | Address to listen on |
| `RESEND_API_KEY` | none | Sends email through Resend. Without it, emails are only logged, which is fine for development and not for production. |
| `MAIL_REPLY_TO` | none | Reply-to address |
| `ACCESS_TTL` | `15m` | Access token lifetime |
| `REFRESH_TTL` | `720h` | Refresh token lifetime |
| `TRUSTED_PROXIES` | none | Networks of the reverse proxies whose forwarded client address is trusted, so rate limits see the real client |
| `LOG_FORMAT` | `json` | `json`, or `gcp` for Cloud Logging |
| `ENVIRONMENT` | `development` | Tag on reported events |
| `POSTHOG_API_KEY` | none | Reports errors and subscription changes to PostHog. Unset, nothing leaves the process. |
| `POSTHOG_HOST` | EU region | PostHog ingestion host |
| `RATE_LIMIT_RPS` | off | Requests per second per client for proxied traffic |

Billing is optional and is switched on by `STRIPE_WEBHOOK_SECRET`:

| Variable | Meaning |
| --- | --- |
| `STRIPE_WEBHOOK_SECRET` | Enables billing and the `/webhooks/stripe` endpoint |
| `STRIPE_API_KEY` | Needed for checkout and the customer portal |
| `BILLING_PRICES` | The catalog of what can be bought, as `price_id=plan:<plan>` or `price_id=addon:<add-on>` pairs separated by commas |
| `BILLING_FREE_PLAN` | The plan a new or lapsed workspace holds. Defaults to the catalog's free plan. |

Plan and add-on ids in `BILLING_PRICES` must exist in the feature catalog in the code, or the gateway refuses to start.

## Web app build settings

These are read when the web app is built, not at runtime:

| Variable | Meaning |
| --- | --- |
| `VITE_API_URL` | Gateway origin to call directly. Leave it unset to use the page's own origin, which needs no CORS. |
| `VITE_POSTHOG_KEY` | Turns analytics on. Unset, there is no analytics and no consent banner. |
| `VITE_POSTHOG_HOST` | PostHog host, the EU region by default |

## Kubernetes

Plain manifests live under `infrastructure/kubernetes` in the repository, one directory per environment, with the gateway, the web app and the Ingress as separate deployments. The README there lists what to replace before a first deploy: image names, hosts, routes and secrets.

## Health probes

| Path | Meaning |
| --- | --- |
| `/healthz` | Liveness. Answers 200 as soon as the process is up. |
| `/readyz` | Readiness. Answers 200 once a route is configured and, with the SaaS surface on, the database answers a ping. Otherwise 503. |

Any replica can start first: migrations take a lock and are safe to run twice.
