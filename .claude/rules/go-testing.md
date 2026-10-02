---
paths:
  - "apps/**/*.go"
trigger: glob
globs: apps/**/*.go
description: Go tests follow the layer they cover — table tests and hand-written fakes for core, error-mapping tests for inbound adapters — and every bug fix starts with a failing test
alwaysApply: false
---

# Rule: test each layer where it can fail, with the cheapest honest tool

**The failure modes:**
1. **Untestable logic.** Core code holding a real DB or HTTP client can only be
   tested against it, so it isn't tested.
2. **Mocks that test the mock.** Generated-mock expectations pin call order
   instead of behavior and break on every refactor.
3. **Slow suites nobody runs.** If the quick check is slow, it stops being run.

## Do

- **Core (`internal/core`)**: unit tests next to the code (`x_test.go`, same
  package), table tests for branching logic, **hand-written fakes** of the ports
  (a small `fakeX` struct in the test file). Model: `internal/core/route_test.go`.
- **Inbound adapters**: test the error-to-code mapping and request handling with
  `httptest` and a fake core.
- **Outbound adapters**: integration tests that start with
  `if testing.Short() { t.Skip("needs external service") }`.
- **Every bug fix starts with a failing test** that reproduces it.
- **Names state behavior**: `TestMatch_LongestPrefixWins`.
- No coverage threshold: a test must protect a behavior someone relies on.

## Do NOT

- Don't add mock-generation libraries (`testify/mock`, `gomock`) in new code.
- Don't call the network or real clock in a unit test; inject them.

## Verify

`git diff` shows new files only once git knows them: run `git add -N <files>`
first. Vet and short tests for every Go module the branch touches:

```bash
BASE=$(git merge-base origin/main HEAD)
git diff --name-only "$BASE" -- '*.go' '*go.mod' | while read -r f; do
  d=$(dirname "$f")
  while [ "$d" != . ] && [ ! -f "$d/go.mod" ]; do d=$(dirname "$d"); done
  [ "$d" != . ] && echo "$d"
done | sort -u | while read -r m; do
  echo "== $m"
  go -C "$m" vet ./... && go -C "$m" test -short -race ./...
done
```
