---
name: capture-learnings
description: Use at the end of executing a plan (or when the user asks to capture lessons / update memory) to distil reusable knowledge into the .claude/memory wiki — ingest a finished plan's lessons or lint the wiki's health. Never auto-commits.
---

# Capture learnings

Maintain the project's compounding knowledge base (`.claude/memory/`). You do the
bookkeeping — summarizing, cross-referencing, filing, dedup — so the next session
starts with accumulated knowledge. Conventions live in
`.claude/rules/memory-wiki.md`; read it before operating.

## Modes

- `/capture-learnings [plan-path]` → **ingest** (default).
- `/capture-learnings lint` → **lint** (health-check only).

## Ingest

**Main session: delegate, don't ingest inline.** Resolve the plan path (step 1),
dispatch the `librarian` subagent (Agent tool, `subagent_type: librarian`) with
that path as its prompt, then relay its "what I saved and where" summary to the
user verbatim. Steps 2–8 are the procedure the librarian executes. Run them
inline only when the Agent tool is unavailable.

1. **Locate the source.** Use `plan-path` if given; else the most recently
   completed plan in `docs/superpowers/plans/` (confirm if ambiguous).
2. **Read + reflect.** Reconstruct the session's lessons: what was non-obvious,
   what needed retries, decisions and their rationale, gotchas.
3. **Extract candidates.** Actionable, compounding lessons only. Drop anything the
   repo already records (code structure, git history, facts in rules) or that
   mattered only to this one conversation.
4. **Route** each with the rubric in `.claude/rules/memory-wiki.md`. Default is
   `.claude/memory/`; personal notes go to the harness memory; propose (don't
   apply) promotions to `.claude/rules/` or a new skill.
5. **Integrate, don't append.** Update the page a lesson belongs to rather than
   duplicate; note contradictions with older claims. Add `[[links]]`, ensure the
   page has an inbound link, bump `metadata.updated`, add the plan to
   `metadata.sources`.
6. **Update `index.md`** (one line per page) and **append `log.md`**
   (`## [<date>] ingest | <topic>`).
7. **Gate.** Run `node .claude/memory/check.mjs` — it must pass.
8. **Report, don't commit.** Print a per-file "what I saved and where" summary
   plus proposed promotions. Respect selective staging (`git add .claude/memory`,
   never `-A`).

## Lint

Read every page and `index.md` and report (fix only on approval): contradictions,
stale claims (verify referenced files still exist), orphans, dangling `[[links]]`,
concepts referenced repeatedly but lacking a page, index drift. End with
`node .claude/memory/check.mjs` and append `## [<date>] lint | <scope>` to
`log.md`.

## Guardrails

- Never auto-commit; never edit the wiki without showing the summary first.
- Personal `user`/`feedback` notes never enter `.claude/memory/`.
