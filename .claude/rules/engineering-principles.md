---
trigger: always_on
description: Every change follows the same engineering principles — simplest thing that works, reuse before writing, SOLID in concrete terms, and leave touched legacy code better
alwaysApply: true
---

# Rule: build the simplest thing that fits, reuse before writing, leave what you touch better

These principles apply to every change. The area rules (`go-*`, `frontend-*`,
`kubernetes-manifests`) make them concrete; where an area rule is silent, this
file decides.

**The failure modes:**
1. **Copying the file you opened first.** The nearest file becomes the template,
   even when it is a shortcut, a placeholder or legacy.
2. **Copy-paste instead of reuse.** Every copy is one more place a fix must
   land and one more place it gets forgotten.
3. **Speculative abstraction.** An interface, option or generic helper with a
   single caller costs reading time and hides the one real behavior.

## Do

- **KISS / YAGNI.** Write the simplest code that meets today's requirement. Add
  a layer, interface, generic helper, option or config flag only when a second
  *real* caller exists. Prefer deleting code to adding it.
- **Reuse, in this order: search → reuse → extract.**
  - Search `packages/*` and the other apps before writing a helper, component
    or type.
  - The moment a **second deployable** (app or service) needs code, move it to
    `packages/<pkg>`. Never copy it.
  - Inside one deployable, extract on the **third** copy.
- **SOLID, in concrete terms:**
  - *Single responsibility* — one reason to change per file. Don't grow a file
    past ~800 lines; when you touch a file already over that, extract the part
    you change.
  - *Open/closed* — extend by adding an adapter behind an existing port, not by
    adding flags or `switch`es to the use case.
  - *Liskov* — an adapter fully satisfies its port; in Go, assert it with
    `var _ port.X = (*Y)(nil)`.
  - *Interface segregation* — small interfaces (1–3 methods), declared by the
    consumer, where they are used.
  - *Dependency inversion* — core logic depends on ports; concrete adapters are
    built and wired only in the composition root (`main.go` in the gateway).
- **Leave it better.** New code always follows the standard. When a change
  substantially touches legacy code, first move the part you touch to the
  standard in its own `refactor` commit, then make the change. Don't refactor
  what you don't touch.
- **The rule beats the code.** Code that contradicts a rule is not a precedent:
  follow the rule and point the divergence out (commit body, PR, or to the
  user).
- **Use only Biome** for linting and formatting TS/JS/JSON/CSS. Do not add
  ESLint or Prettier.

## Where the details live

Every rule, indexed by when it applies: **Project Rules** in `AGENTS.md`.

## Verify

`git diff` shows new files only once git knows them: run `git add -N <files>`
first.

Files the branch touches that are over 800 lines — for each, check the change
didn't grow it and whether the touched part can move out:

```bash
BASE=$(git merge-base origin/main HEAD)
git diff --name-only "$BASE" -- '*.go' '*.ts' '*.tsx' | while read -r f; do
  [ -f "$f" ] || continue
  n=$(wc -l < "$f" | tr -d ' ')
  [ "$n" -gt 800 ] && echo "$n $f"
done
```
