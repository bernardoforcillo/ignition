---
paths:
  - "apps/web/src/**"
  - "packages/components/src/**"
trigger: glob
globs: apps/web/src/**,packages/components/src/**
description: Frontend code is split by domain into thin routes plus feature folders, every importable module is a folder with an index.ts, features import each other only through module indexes, and shared UI lives in packages/components
alwaysApply: false
---

# Rule: a route renders a feature, and every module is a folder with an index

`apps/web` (Vite + React + TanStack Router/Query) is split by **domain**
(`features/<domain>`), not by technical layer. Every importable unit is a
folder named after it with an `index.ts` as its public entry.

**The failure modes:**
1. **UI inside route files.** A screen written inside a route cannot be reused
   or tested without the router.
2. **Files that grow without limit.** A flat `name.tsx` has nowhere to put its
   parts, so they pile up inside it.
3. **Hidden coupling.** Features importing each other's private files turn a
   refactor into a hunt.

## Layout

```text
apps/web/src/
  app/                    # providers and the app shell
  routes/<name>/          # route definitions only; render one feature entry
  features/<domain>/
    index.ts              # the feature's public surface
    <name>-panel.tsx      # private parts
  lib/api/                # data access (TanStack Query hooks call these)
  stores/<name>/          # client state (zustand), one folder per store
  styles/
packages/components/src/
  atoms/<name>/           # smallest UI units
  molecules/<name>/       # compositions of atoms
  styles/theme.css        # design tokens
```

## Do

- **Routes are thin.** A route file wires path, loader and one feature entry
  component; no UI, data calls or state of its own.
- **A module is a folder** `{kebab-name}/index.ts` exporting the public API.
  Other files in the folder are **private parts**. When something outside the
  folder needs a part, promote it to its own module folder.
- **Named exports** (`export const GreetingPanel`), props declared as
  `type Props`; `export default` only where a tool requires it.
- **Imports**: from another feature import only its `index.ts`, never a private
  file. Barrels only at a package root (`packages/components/src/index.ts`) and
  at a feature's own `index.ts`.
- **Server state** through TanStack Query; **URL state** through the router;
  **client state** through a store in `stores/`.
- **UI used by a second app moves to `@ignition/components`** as an atom or
  molecule, styled with tokens from `theme.css`.

## Do NOT

- Don't put UI, data calls or state in `routes/**`.
- Don't add `utils/` at the app root: helpers go in the feature or in
  `lib/<name>/`.
- Don't hard-code colors or spacing a token already covers.

## Verify

`git diff` shows new files only once git knows them: run `git add -N <files>`
first. Every new non-route file sits in a module folder (should print nothing):

```bash
BASE=$(git merge-base origin/main HEAD)
git diff --name-only --diff-filter=A "$BASE" \
  | grep -E '^(apps/web/src|packages/components/src)/.*\.tsx?$' \
  | grep -vE '/index\.ts$|/routes/|/app/|\.d\.ts$' | while read -r f; do
  d=$(dirname "$f")
  [ -f "$d/index.ts" ] || echo "not in a module folder: $f"
done
```

Then `pnpm biome check` and `pnpm typecheck`.
