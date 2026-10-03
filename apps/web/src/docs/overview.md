# Overview

Ignition is a starter for SaaS products. It is a React web app and a Go gateway that already handle the parts every subscription product needs before its first customer: accounts, workspaces with roles, plans and limits, billing, email and data requests.

## What is in the box

| Area | What you get |
| --- | --- |
| Accounts | Email sign-up with verification, sign-in, password reset and rotating sessions |
| Workspaces | A workspace per customer, with owner, admin and member roles and email invitations |
| Plans and limits | A catalog of plans, add-ons and feature flags, with metered monthly limits |
| Billing | Stripe hosted checkout, the customer portal and signed webhooks |
| Your data | Export your data as JSON and erase your account |
| Email | Templates written with react.email and sent from the Go gateway |
| Privacy | Analytics that stay off until a visitor accepts, hosted in the EU by default |

## How the pieces fit

The browser talks to one origin. The web app calls the Go gateway with typed Connect requests, and the gateway owns every rule: who may do what, which plan a workspace is on, what is sent by email. The gateway stores everything in Postgres.

The API is described by protobuf files, so the web app and the gateway share one typed contract. See [the API guide](/docs/api) for how to call it yourself.

## Where to go next

- New here? Follow [Getting started](/docs/getting-started).
- Wondering how people sign in? Read [Authentication and security](/docs/authentication).
- Running your own copy? See [Self-hosting](/docs/self-hosting).
