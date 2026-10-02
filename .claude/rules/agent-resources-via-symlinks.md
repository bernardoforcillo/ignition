---
paths:
  - ".claude/**"
  - ".agents/**"
  - ".codex/**"
  - ".cursor/**"
  - "AGENTS.md"
  - "CLAUDE.md"
  - ".mcp.json"
  - ".dockerignore"
trigger: glob
globs: .claude/**,.agents/**,.codex/**,.cursor/**,AGENTS.md,CLAUDE.md,.mcp.json,.dockerignore
description: Agent rules, skills, instructions and MCP config are authored once and reach Codex, Cursor and Antigravity only through relative symlinks, each covered by a literal .dockerignore path
alwaysApply: false
---

# Rule: agent resources are authored once and reach other agents only through symlinks

Claude Code, Codex, Cursor and Antigravity each scan their own paths for
instructions, rules, skills and MCP config. This repo authors every one of
those files **once** and exposes it at the other agents' paths as a **symlink**.

**The failure modes:**
1. **Drift**: a copy, or a pointer/stub file that summarizes or `@`-includes the
   source, diverges the first time someone edits one side — and each agent then
   follows a different version of the rule.
2. **Broken image builds**: a Docker `COPY . .` can die on a symlink that
   `.dockerignore` doesn't exclude by a **literal** path; glob entries like
   `**/.agents` don't stop it.

## Source → symlink map

| Authored source | Symlink | Read natively by |
|---|---|---|
| `AGENTS.md` | `CLAUDE.md` | Claude Code |
| `.claude/rules/` | `.agents/rules` | Antigravity |
| `.claude/rules/<name>.md` | `.cursor/rules/<name>.mdc` | Cursor (loads only `.mdc`) |
| `.claude/skills/` | `.agents/skills` | Antigravity, Codex, Cursor |
| `.claude/skills/` | `.codex/skills` | Codex, Cursor |
| `.claude/skills/` | `.cursor/skills` | Cursor |
| `.mcp.json` | `.cursor/mcp.json` | Cursor |
| `.mcp.json` | `.agents/mcp_config.json` | Antigravity |

`AGENTS.md` is the authored instruction file here because Turborepo manages a
block inside it. `.dockerignore` covers the mirrors with literal paths.

Codex reads MCP servers only from TOML, so `.mcp.json` can't be symlinked to
it: configure Codex MCP per machine with `codex mcp add`.

## Do

- **Edit only the authored source.** Every symlink follows automatically.
- **Adding a rule:** create `.claude/rules/<name>.md` with frontmatter that
  serves Claude Code (`paths`), Antigravity (`trigger`, `globs`) and Cursor
  (`description`, `globs`, `alwaysApply`). Keep `paths:` (a YAML list) and
  `globs:` (comma-separated, no spaces) identical. For an always-on rule drop
  both and use `trigger: always_on` and `alwaysApply: true`. Then link it for
  Cursor and list it in `AGENTS.md` **Project Rules** in backticks, never as an
  `@` import:
  ```bash
  ln -s ../../.claude/rules/<name>.md .cursor/rules/<name>.mdc
  ```
- **Adding a skill:** `.claude/skills/<name>/SKILL.md` (with `name` and
  `description` frontmatter) as a **direct** child of `.claude/skills/`.
- **Adding an MCP server:** `.mcp.json` only. A remote server carries the URL
  twice: `type: "http"` + `url` (Claude Code, Cursor) and `serverUrl`
  (Antigravity).
- **Adding a mirror:** a **relative** symlink, plus its literal path in
  `.dockerignore` in the same commit, plus its row in the map above.
- **Removing or renaming:** change the source and every symlink together.

## Do NOT

- Don't copy a file into another agent's directory or write a pointer/stub where
  a symlink belongs.
- Don't use absolute symlink targets.
- Don't edit through a symlink path with an editor that saves atomically: it
  replaces the link with a regular file (`git status` shows `T`).

## Verify

Every mirror is a working symlink (should print nothing):

```bash
for l in CLAUDE.md .agents/rules .agents/skills .agents/mcp_config.json \
  .codex/skills .cursor/skills .cursor/mcp.json .cursor/rules/*.mdc; do
  { [ -L "$l" ] && [ -e "$l" ]; } || echo "not a working symlink: $l"
done
```

Every rule has its `.mdc` symlink and none is orphaned (should print nothing):

```bash
for f in .claude/rules/*.md; do n=$(basename "$f" .md)
  [ "$(readlink ".cursor/rules/$n.mdc")" = "../../.claude/rules/$n.md" ] \
    || echo "missing .mdc symlink: $n"
done
for m in .cursor/rules/*.mdc; do
  [ -f ".claude/rules/$(basename "$m" .mdc).md" ] || echo "orphaned: $m"
done
```

Every rule's `paths:` and `globs:` list the same patterns (should print nothing):

```bash
for f in .claude/rules/*.md; do
  fm=$(awk 'NR==1 && /^---$/ {f=1; next} f && /^---$/ {exit} f' "$f")
  p=$(printf '%s\n' "$fm" | awk '/^paths:/ {f=1; next} f && /^  - / {sub(/^  - /, ""); gsub(/"/, ""); print; next} {f=0}' | sort)
  g=$(printf '%s\n' "$fm" | sed -n 's/^globs: *//p' | tr ',' '\n' | sort)
  [ "$p" = "$g" ] || echo "paths and globs differ: $f"
done
```
