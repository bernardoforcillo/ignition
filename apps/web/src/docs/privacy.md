# Privacy and analytics

Ignition is built for products that serve people in the EU, so analytics are off until a visitor says yes.

## Consent

When analytics are configured for a deployment, a banner asks once. Nothing is collected, and no analytics script is even loaded, until you choose Accept analytics. Declining is as easy as accepting and the product works the same either way. The browser's Do Not Track setting is respected as well.

A deployment that does not configure analytics shows no banner and collects nothing.

## What is collected after you accept

- Page views, recorded by path only. Single-use tokens and redirect targets in addresses are stripped before anything is sent.
- Clicks and form submissions, not what you typed or copied.
- A few product events, such as signing up or starting a checkout, with ids and categories, never emails, names or free text.
- Errors, so they can be fixed.
- Session recordings with every input masked.

When you are signed in, your activity is tied to your account id only, never your email address, and the link is cleared when you sign out.

On this site, choosing a plan on the landing page records which plan, and reading a docs page records its name.

## Where it goes

Data goes to PostHog, in the EU region by default. A self-hosted deployment can point it somewhere else, see [Self-hosting](/docs/self-hosting).

## What the server reports

All server logs go to the standard output of the gateway for your platform to collect. Only two things can go to PostHog, and only when `POSTHOG_API_KEY` is set: errors, with a stack trace, and the fact that a workspace's subscription changed. Passwords, tokens and email bodies are not logged.

## Email

Email is sent only from the Go gateway, through Resend. Ignition sends messages you ask for or need: verification, password reset, invitations and a notice when an address already has an account.

## Your rights

You can [export your data and delete your account](/docs/your-data) yourself from the Settings page.
