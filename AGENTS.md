<!-- BEGIN:turborepo-agent-rules -->

# This is NOT the Turborepo you know

Turborepo configuration, task behavior, and CLI commands can vary between installed versions and may differ from your training data. Resolve the `turbo` package from this file's directory or relevant workspace; in monorepos, it may not be visible from the repository root. For example, run `node -p "require.resolve('turbo/package.json')"` from a workspace that depends on `turbo`.

Read `docs/README.md` inside that installed package first, then read the relevant pages from its `docs/` directory before changing Turborepo configuration or commands. Heed deprecation notices. These bundled docs match the installed package version and are available without network access.

This block is written and re-added by `turbo` before repository-scoped commands when an AI agent is detected. In the Turborepo source repository, its template is defined in `crates/turborepo-cli/src/cli/agent_guidance.rs`. Removing the managed block while updates are enabled means a later qualifying invocation will add it again. Set `"agentGuidance": false` in the root `turbo.json` or `turbo.jsonc` to opt out; this does not remove an existing block. Keep the block committed with your work to avoid an uncommitted change on the next agent invocation.
<!-- END:turborepo-agent-rules -->

# Project rules

This file guides every coding agent in this repository: Claude Code reads it as
`CLAUDE.md` (a symlink to this file); Codex, Cursor and Antigravity read it as
`AGENTS.md`. Rules live in `.claude/rules/`: a rule without `paths:` loads in
every session, a path-scoped rule loads when a matching file is read; other
agents read them by path. Open the matching rule **before** the work it covers.

- Every change: simplest thing that works, reuse before writing, SOLID, leave
  touched legacy better → `.claude/rules/engineering-principles.md`
- Go code under `apps/` — layout, transport, errors, config, logging →
  `.claude/rules/go-service-architecture.md`
- Go tests → `.claude/rules/go-testing.md`
- Frontend layout, feature folders, imports, shared UI →
  `.claude/rules/frontend-architecture.md`
- Editing a `.proto`, `buf*.yaml` or generated code →
  `.claude/rules/protobuf-codegen.md`
- Adding or changing a deployable, its Dockerfile or any manifest under
  `infrastructure/kubernetes/` → `.claude/rules/kubernetes-manifests.md`
- Layering and dependency direction (UI → transport → domain → capabilities),
  build order, supply-chain hygiene → `.claude/rules/code-organization.md`
- Scaling and service-boundary decisions (new service, broker, cache, rate
  limiting, uploads) → `.claude/rules/system-design.md`
- Branches, commit hygiene, worktrees, agent worktrees →
  `.claude/rules/git-flow.md`
- Touching the memory wiki → `.claude/rules/memory-wiki.md`
- Adding, moving or removing an agent rule, skill, MCP config or instruction
  file → `.claude/rules/agent-resources-via-symlinks.md`

## Shared agent resources

`.claude/` is the only place agent tooling is authored; the other agents reach
it through **relative symlinks** (map and checks in the rule above).

| Agent | Instructions | Rules | Skills | MCP servers |
|---|---|---|---|---|
| Claude Code | `CLAUDE.md` → `AGENTS.md` | `.claude/rules/` | `.claude/skills/` | `.mcp.json` |
| Codex | `AGENTS.md` | by path, from the list above | `.codex/skills/`, `.agents/skills/` | per machine (`codex mcp add`) |
| Cursor | `AGENTS.md` | `.cursor/rules/*.mdc` | `.cursor/skills/` | `.cursor/mcp.json` |
| Antigravity | `AGENTS.md` | `.agents/rules/` | `.agents/skills/` | `.agents/mcp_config.json` |

Skills (`.claude/skills/<name>/SKILL.md`): `commit` — a Conventional-Commits
message that passes commitlint; `capture-learnings` — distil a finished plan's
lessons into the memory wiki.

Subagents (`.claude/agents/`, Claude Code only): `software-architect` reviews a
design against `system-design.md` and `code-organization.md`; `librarian` runs
the memory-wiki ingest.

## Memory wiki

Project knowledge accumulates in `.claude/memory/`, a committed markdown wiki
maintained by `capture-learnings` (conventions in
`.claude/rules/memory-wiki.md`). Read `.claude/memory/index.md` before work that
may already have recorded lessons. PRDs, specs and plans live in
`docs/superpowers/` and are local-only (gitignored).

## Conventions

- **pnpm only**, never npm or yarn. Root dev deps: `pnpm add -Dw <pkg>`; a
  workspace: `pnpm add <pkg> --filter <workspace>`. Internal packages use
  `workspace:*`.
- **Biome only** for lint and format; the `pre-commit` hook runs
  `biome check --write --staged`, `commit-msg` runs commitlint.
- Declare per-package tasks (`build`, `dev`, `lint`, `typecheck`) as scripts so
  Turbo can orchestrate them.
