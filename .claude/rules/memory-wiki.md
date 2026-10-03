---
paths:
  - ".claude/memory/**"
trigger: glob
globs: .claude/memory/**
description: The project's committed memory wiki in .claude/memory — page format, index and log, ingest and lint operations, and the routing rubric for where a lesson belongs
alwaysApply: false
---

# Memory wiki

The project's compounding knowledge base lives at `.claude/memory/` — a
committed, git-versioned markdown wiki. The `capture-learnings` skill maintains
it; you rarely hand-edit it. Read `.claude/memory/index.md` before work that may
already have recorded lessons.

## Two stores, one maintainer

| Store | Location | Committed | Holds |
| --- | --- | --- | --- |
| **Project wiki** | `.claude/memory/` | Yes | `type: project`, `reference` |
| **Personal memory** | the harness memory dir | No | `type: user`, `feedback` |

Project knowledge → the repo wiki. Personal notes (who the user is, how to work
with them) → the harness memory, never committed.

## Page format

```markdown
---
name: <slug>                 # kebab-case, equals the filename
description: <one-line>      # recall relevance + index hook
metadata:
  type: project | reference
  updated: YYYY-MM-DD        # bump on every edit — drives stale detection
  sources: [docs/superpowers/plans/<file>.md]   # provenance, optional
---

<the fact. Link related pages with [[their-slug]]. For project pages, state the why.>
```

## index.md and log.md

- `index.md` — catalog, one line per page: `- [Title](slug.md) — hook`. Every
  page appears exactly once.
- `log.md` — append-only. Entry prefix `## [YYYY-MM-DD] <op> | <topic>` where
  `<op>` is `ingest`, `lint` or `migrate`.

## Operations (run by the capture-learnings skill)

- **Ingest** — after a plan is executed: extract compounding lessons, route each,
  **integrate into existing pages** (don't duplicate), update `index.md`, append
  `log.md`, then run the checker.
- **Lint** — health-check: contradictions, stale claims, orphans, dangling
  `[[links]]`, index drift.
- Both end with `node .claude/memory/check.mjs` passing and **never
  auto-commit**: show a summary and let the user approve.

## Routing rubric

| Lesson | Destination |
| --- | --- |
| Ongoing project context, constraints, external pointers | `.claude/memory/` |
| Preference or working style | harness memory — not committed |
| Durable convention for everyone | **propose** moving it to `.claude/rules/*.md` |
| Reusable multi-step workflow | **propose** a `.claude/skills/<name>/` skill |
| Only-this-feature detail | note beside the plan, or skip |

## Reminder hook

`.claude/settings.json` enables a `Stop` hook
(`.claude/hooks/capture-learnings-nudge.sh`) that prints a reminder to run
`/capture-learnings` when a plan under `docs/superpowers/plans/` was modified in
the last 30 minutes. It never writes the wiki.
