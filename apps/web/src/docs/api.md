# API

The gateway speaks Connect, which works over plain HTTP and JSON. Any HTTP client can call it, so `curl` is enough to try it out.

## Base URL and shape

Every call is a `POST` to `<base>/<package>.<Service>/<Method>` with a JSON body and a `Content-Type: application/json` header. The base URL is the gateway, or the web app's own origin when it forwards these paths to the gateway, which is how the deployed app is set up. The examples use a shell variable:

```sh
BASE=https://example.com
```

The services live in the `saas.v1` protobuf package:

| Service | Methods |
| --- | --- |
| `AuthService` | `SignUp`, `VerifyEmail`, `Login`, `Refresh`, `Logout`, `RequestPasswordReset`, `ResetPassword` |
| `WorkspaceService` | `ListWorkspaces`, `CreateWorkspace`, `GetWorkspace`, `ListMembers`, `InviteMember`, `AcceptInvite` |
| `FeatureService` | `CheckFeature` |
| `BillingService` | `ListPrices`, `GetSubscription`, `StartCheckout`, `OpenPortal` |
| `AccountService` | `GetMe`, `ExportData`, `DeleteAccount` |

Field names in JSON are lower camel case, for example `accessToken`.

## Public calls

These need no token: `SignUp`, `VerifyEmail`, `Login`, `Refresh`, `RequestPasswordReset`, `ResetPassword` and `BillingService/ListPrices`.

Sign in:

```sh
curl -s $BASE/saas.v1.AuthService/Login \
  -H 'Content-Type: application/json' \
  -d '{"email":"ada@example.com","password":"Correct-Horse-9-Battery!"}'
```

The answer holds the user and the two tokens:

```json
{
  "user": { "id": "...", "email": "ada@example.com" },
  "accessToken": "...",
  "refreshToken": "..."
}
```

List what can be bought:

```sh
curl -s $BASE/saas.v1.BillingService/ListPrices \
  -H 'Content-Type: application/json' -d '{}'
```

It returns `prices`, each with a `priceId`, a `kind` (`plan` or `addon`) and the catalog `id`. Amounts are not part of the API. A deployment without billing answers `unimplemented` or `not_found`.

## Calls that need a token

Everything else needs the access token as a bearer token. Keep it in a variable:

```sh
TOKEN=eyJ...
```

List your workspaces:

```sh
curl -s $BASE/saas.v1.WorkspaceService/ListWorkspaces \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{}'
```

Create a workspace:

```sh
curl -s $BASE/saas.v1.WorkspaceService/CreateWorkspace \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Acme","slug":"acme"}'
```

Invite a member, using the workspace id from the previous answer:

```sh
curl -s $BASE/saas.v1.WorkspaceService/InviteMember \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"workspaceId":"WORKSPACE_ID","email":"grace@example.com","roleKey":"member"}'
```

Ask whether a workspace may use a feature. It consumes nothing:

```sh
curl -s $BASE/saas.v1.FeatureService/CheckFeature \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"workspaceId":"WORKSPACE_ID","featureKey":"api.calls"}'
```

The answer has `enabled`, a `reason`, and for a metered feature the `limit` and `remaining`.

When the access token expires, exchange the refresh token for a new pair. The old refresh token stops working:

```sh
curl -s $BASE/saas.v1.AuthService/Refresh \
  -H 'Content-Type: application/json' \
  -d '{"refreshToken":"..."}'
```

## Errors

A failed call answers with a non-200 status and a JSON body holding a `code` and a `message`:

```json
{ "code": "unauthenticated", "message": "missing or invalid access token" }
```

| Code | HTTP status | Typical cause |
| --- | --- | --- |
| `invalid_argument` | 400 | A weak password, an unknown role or price, bad input |
| `unauthenticated` | 401 | No or expired token, wrong credentials, unverified email |
| `permission_denied` | 403 | Your role in the workspace does not allow it |
| `not_found` | 404 | The workspace does not exist or you are not a member |
| `already_exists` | 409 | The workspace slug is taken |
| `failed_precondition` | 400 | An expired invitation, or a deletion that is blocked |
| `resource_exhausted` | 429 | A rate limit or a feature limit was reached |
| `unavailable` | 503 | A feature is switched off |
| `internal` | 500 | Something unexpected, with no details |

Messages are fixed text, so they never leak internal details.

## The contract

The protobuf files under `proto/saas/v1` in the repository are the source of truth. The web app and the gateway are generated from them, so a client in any language can be generated the same way.
