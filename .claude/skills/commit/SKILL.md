---
name: commit
description: Use when the user asks to commit changes, stage work, or write a git commit message in this repository — produces a Conventional Commits message that passes commitlint.
---

# Commit

Write commits that pass the `commit-msg` hook (commitlint with
`@commitlint/config-conventional` plus the overrides in `commitlint.config.mjs`)
and the `pre-commit` hook (`biome check --write --staged`): atomic, clearly
typed, imperative, explaining **why**.

## Header

```
type(scope): subject
```

| Rule | Requirement |
|------|-------------|
| `type` | One of `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `chore`, `revert`. Lower-case. `build` and `ci` are **not** allowed: use `chore`. |
| `scope` | Optional, lower-case: the workspace — `web`, `components`, `gateway`, `infra`, `deps`. |
| `subject` | Imperative, no leading capital, no trailing period. |
| header length | **≤ 72 chars**. |
| `!` before `:` | Breaking change, e.g. `feat(gateway)!: …`. |

## Body and footers

- Blank line after the subject; body lines ≤ 100 chars; explain *why*.
- Footers after a blank line: `Closes #142`, `BREAKING CHANGE: …`, and the
  co-author trailer the session asks for.

## Workflow

1. `git status` and `git diff` (and `--staged`).
2. Make it atomic: stage only related hunks, commit unrelated ones separately.
3. Pick the type, the narrowest scope, a subject under 72 chars.
4. Commit with a real multi-line message (one `-m` per paragraph).
5. Respect the hooks: if Biome or commitlint fails, fix the cause and retry.
   **Never** use `--no-verify`.

## Common mistakes

| Mistake | Fix |
|---------|-----|
| `Fix bug` (capitalized) | `fix: handle expired session` |
| `fixed the login bug` | Imperative: `fix: …` |
| `fix: update stuff.` | Be specific, drop the period |
| `ci:` / `build:` type | Use `chore:` |
| Header over 72 chars | Shorten; move detail to the body |
| Bundling unrelated edits | Split into atomic commits |
