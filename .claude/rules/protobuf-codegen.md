---
paths:
  - "apps/**/proto/**"
  - "apps/**/buf*.yaml"
  - "apps/**/internal/gen/**"
trigger: glob
globs: apps/**/proto/**,apps/**/buf*.yaml,apps/**/internal/gen/**
description: The .proto files are the source of truth, generated code is committed and never edited by hand, and breaking changes are checked with buf
alwaysApply: false
---

# Rule: the proto is the source of truth and the generated code is committed

Each service keeps its `.proto` files under `apps/<svc>/proto` and its
generated Go in `apps/<svc>/internal/gen`. The generated tree is **committed**
so a container build needs only the Go toolchain.

**The failure modes:**
1. **Hand-edited generated code.** The next `buf generate` silently reverts it.
2. **Stale generated code.** The `.proto` changed and `internal/gen` didn't, so
   the code compiles against the old contract.
3. **Silent breaking changes.** Renumbering or removing a field breaks every
   deployed client.

## Do

- Edit the `.proto`, then run `buf generate` from `apps/<svc>` and commit both.
- Run `buf lint` and `buf breaking --against '.git#branch=main'` before
  committing; the module uses `STANDARD` lint and `FILE` breaking rules.
- Add fields with new numbers; mark removed ones `reserved`. Never reuse a
  field number or change a type.
- Package names are versioned (`gateway.v1`); a breaking change is a new
  version.

## Do NOT

- Don't edit anything in `internal/gen`.
- Don't commit a `.proto` change without its regenerated output.

## Verify

Generated output matches the proto (should print nothing):

```bash
cd apps/gateway && buf generate && git status --porcelain internal/gen
```
