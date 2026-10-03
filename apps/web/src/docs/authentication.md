# Authentication and security

## Password policy

The server enforces the policy on sign-up and when you reset a password. A password must have:

- at least 12 characters,
- an upper case letter and a lower case letter,
- a digit,
- a symbol, meaning anything that is not a letter, a digit or a space.

The web forms check the same rule so you hear about a problem before the request is sent, but the server is the authority. A password that fails is rejected with `invalid_argument` and the message "password does not meet requirements".

## Sessions

Signing in returns two tokens:

| Token | Lifetime | Purpose |
| --- | --- | --- |
| Access token | 15 minutes by default | Sent as a bearer token on every call that needs sign-in |
| Refresh token | 30 days by default | Exchanged for a new pair of tokens when the access token expires |

Both lifetimes are configurable in a self-hosted deployment, see [Self-hosting](/docs/self-hosting).

In the web app the access token lives in memory only. The refresh token is kept in the browser's local storage so a reload can sign you back in silently. A refresh token is single use: each time it is exchanged, it is replaced by a new one, and reusing an old one is detected.

Signing out revokes the refresh token on the server. An access token that was already issued stays valid until it expires, which is why its lifetime is short.

You must verify your email address before you can sign in. Until then the server answers `unauthenticated` with "email not verified".

## Resetting a password

1. On the sign-in page choose "Forgot password" and enter your address.
2. The page answers the same way whether or not the address has an account. If it does, an email with a single-use link arrives.
3. The link opens the reset page. Choose a new password that meets the policy.

A reset link works once and expires. Resetting a password signs you out everywhere by revoking every session.

## Rate limits

The public sign-in calls are limited per client address to keep guessing and email abuse in check:

| Action | Limit |
| --- | --- |
| Sign in | 20 attempts per 15 minutes |
| Sign up | 10 per hour |
| Password reset request | 10 per hour, and 3 per hour for one email address |

When a limit is hit the API answers `resource_exhausted` and the app asks you to try again later. The per-address reset limit is silent, so it never confirms that an account exists.
