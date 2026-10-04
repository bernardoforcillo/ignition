---
paths:
  - "proto/**"
  - "buf.yaml"
  - "buf.gen.yaml"
  - "tools/**"
  - "go-packages/proto/**"
  - "packages/proto/**"
trigger: glob
globs: proto/**,buf.yaml,buf.gen.yaml,tools/**,go-packages/proto/**,packages/proto/**
description: The .proto files in /proto are the single contract for Go and TypeScript, both generated outputs are committed and never edited by hand, and breaking changes are checked with buf
alwaysApply: false
---

# Rule: one proto, two generated packages, committed

The API contract lives once, in `proto/<package>/v1/*.proto`. `buf generate`
(`pnpm gen:proto`) writes it to two **committed** shared packages:

| Output | Package | Consumed by |
|---|---|---|
| `go-packages/proto/gen` | Go module `.../go-packages/proto` | `apps/gateway` and every Go service |
| `packages/proto/src/gen` | `@ignition/proto` | `apps/web` and every TS app |

The generators are pinned and local: `tools/go.mod` (`tool` directives for
`buf`, `protoc-gen-go`, `protoc-gen-connect-go`) and the `@ignition/proto`
devDependencies (`protoc-gen-es`). Generation needs no remote plugin.

**The failure modes:**
1. **Hand-edited generated code.** The next `pnpm gen:proto` silently reverts it
   (`clean: true` wipes both output trees first).
2. **Stale generated code.** The `.proto` changed and one output didn't, so the
   Go and TypeScript sides compile against different contracts.
3. **Silent breaking changes.** Renumbering or removing a field breaks every
   deployed client.
4. **A service-local copy.** A `proto/` folder or generated tree inside an app
   forks the contract; the app and the web then disagree.

## Do

- Edit the `.proto`, run `pnpm gen:proto`, commit the `.proto` **and** both
  generated trees together.
- Run `pnpm lint:proto` and `go tool buf breaking --against '.git#branch=main'`
  before committing; the module uses `STANDARD` lint and `FILE` breaking rules.
- Give each file `option go_package =
  "github.com/bernardoforcillo/ignition/go-packages/proto/gen/<pkg>/v1;<pkg>v1"`.
- Add fields with new numbers; mark removed ones `reserved`. Never reuse a field
  number or change a type. A breaking change is a new package version (`v2`).
- Import Go types from `go-packages/proto/gen/...` and TS types from
  `@ignition/proto/<pkg>/v1/<file>_pb`; the web builds clients with
  `createClient(Service, transport)` from `@connectrpc/connect`.

## Do NOT

- Don't edit anything in `go-packages/proto/gen` or `packages/proto/src/gen`.
- Don't commit a `.proto` change without both regenerated outputs.
- Don't add a `proto/` folder, `buf*.yaml` or generated code under an app.

## Verify

Generated output matches the proto (should print nothing):

```bash
pnpm gen:proto && git status --porcelain go-packages/proto/gen packages/proto/src/gen
```
