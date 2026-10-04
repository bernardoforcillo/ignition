---
trigger: always_on
description: Branches go feature to dev to main, commits stage only their own files, never amend on a shared branch, and parallel work prefers git worktrees
alwaysApply: true
---

# Git flow & commit hygiene

Applies to the whole monorepo.

## Branch roles

- `feature/*`, `fixes/*`, `chore/*` — short-lived work branches off `dev`,
  prefixed by change type. Open a PR **into `dev`**.
- `dev` — integration branch; deployed to the `development` environment.
- `main` — production; promoted by a PR from `dev`. Never push work directly to
  `main`.

```
feature|fixes|chore/* ──PR──▶ dev ──PR──▶ main
                           (development)  (production)
```

When CI publishes images, give every build an immutable `<UTC-timestamp>-<sha>`
tag, with a `-canary` suffix for `dev` builds, so any commit's image can be
pulled after `latest` moves on.

## Commit hygiene

Work branches often carry unrelated in-progress changes. So:

- **Stage only the files your change touches** — `git add <paths>`, not
  `git add -A`. Don't sweep someone else's half-finished work into your commit.
- **Format only your own files** (`pnpm exec biome check --write <paths>`).
- If a project-wide check fails, confirm the error is in *your* files before
  reacting: it may come from someone else's uncommitted work.
- The `pre-commit` hook runs Biome on **staged files only**, so unrelated WIP
  doesn't block a clean commit. Never bypass hooks with `--no-verify`.
- **History can move under you.** Re-check `git rev-parse HEAD` and `git log`
  after any gap before assuming your last commit is still the tip.
- **Never `git commit --amend` on a shared branch**: authorship is the same repo
  user for every commit, so an amend can silently rewrite someone else's commit.
  Make fresh commits only.

## Parallel work: prefer git worktrees

When more than one feature is in flight, the preferred setup is **one worktree
per feature**, each on its own `feature/*` branch off `dev`:

```sh
git worktree add ../ignition-<feature> -b feature/<feature> dev   # then: pnpm install
```

`node_modules` is not shared: run `pnpm install` in each. Remove a finished one
with `git worktree remove ../ignition-<feature>`. This is the recommended
default, not a mandate: suggest it, but never create or switch worktrees, or
move the user's uncommitted work, without asking.

## Agent worktrees → `dev` (port, don't merge)

Subagents run with `isolation: "worktree"` work on a throwaway
`worktree-agent-*` branch cut from **whatever branch the session is on**, so it
can carry unrelated commits.

- **Never merge or PR an agent's branch straight into `dev`.**
- **Port its commit onto a fresh `feature/<name>` cut from `dev`** with
  `git cherry-pick <sha>`, verify (build and tests), then PR as usual. Remove
  the agent tree and branch once the port is verified.
