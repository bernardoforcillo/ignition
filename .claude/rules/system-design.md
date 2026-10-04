---
paths:
  - "apps/gateway/**"
  - "infrastructure/**"
trigger: glob
globs: apps/gateway/**,infrastructure/**
description: Scaling and service-boundary decisions follow trigger-based rules — stateless handlers, horizontal before vertical, no new service without a proven bottleneck, presigned uploads, a broker only for real fan-out, rate limiting at the edge
alwaysApply: false
---

# Rule: reach for a scaling layer when its trigger is real, not preemptively

Trigger → action rules, not a target architecture. Dispatch the
`software-architect` agent to apply this checklist to a design review or to
scaffold one of these changes.

## Statelessness

Handlers are stateless — request-scoped only. Anything that must persist goes to
its datastore (database, object storage, cache), never held in-process across
requests. In-process state kills horizontal scaling and pod restarts.

## Scale order: horizontal before vertical

Prefer more replicas (HPA bounds in
`infrastructure/kubernetes/<env>/<namespace>/<app>/hpa.yaml`) over bigger
requests/limits. Reach for vertical scaling only when a workload is genuinely
CPU/memory-bound in a way replicas can't parallelize. The Service already
load-balances across endpoints: don't build a custom load balancer. If one route
is disproportionately expensive, split it into its own workload.

## Microservices threshold

Don't add an `apps/<name>` service until there is a **proven, isolated
bottleneck** or a genuinely separate ownership boundary. The hexagonal layout
already isolates domains inside one binary; add a core domain or an adapter
first. See `kubernetes-manifests.md` and `git-flow.md` for what a new deployable
costs (Dockerfile, manifests in every environment, CI); `pnpm gen service` scaffolds the
repetitive part.

## Gateway / routing

Public traffic enters through the gateway and the Ingress. Services call each
other in-cluster by Service name, never through the public network, and
non-gateway services stay off the public network.

## AuthN vs authZ

Validate tokens (signature and expiry) at the edge without a network hop per
request. Keep token *issuance* in one place; don't duplicate signing logic.

## Large files / blobs

Never proxy large uploads through a Go binary and never store blob bytes in the
relational database. Use presigned URLs: the API writes metadata and returns a
short-lived, size-capped upload URL; the client uploads straight to object
storage.

## Async fan-out

When more than one downstream must react to the same event, introduce a broker
instead of a synchronous call chain. A broker needs ack/redelivery and a
dead-letter path with alerting, not just the happy path.

## Caching vs CDN

Cache small, hot **metadata** in an in-memory KV store. Never cache blobs there:
that is what a CDN is for.

## Rate limiting

Enforce at the edge (the gateway already supports `RATE_LIMIT_RPS`), backed by a
fast counter store. Add it when there is a credible abuse or cost risk.

## The meta-rule

Before adding a layer, state the **current, observed** pain it solves (an
incident, a PR, a measurement), not a hypothetical one. It is the same YAGNI
discipline the rest of the rules apply to code.
