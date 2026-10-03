---
paths:
  - ".github/**"
  - "docs/ci-cd.md"
trigger: glob
globs: .github/**,docs/ci-cd.md
description: GitHub Actions workflows run with least privilege, pin action versions, gate on ci-ok and security checks, and publish signed, scanned images
alwaysApply: false
---

# Rule: CI/CD is least-privilege, reproducible and the only road to a published image

Workflows are in `.github/workflows/` (`ci.yml`, `security.yml`, `images.yml`), the
shared setup is `.github/actions/setup-js`. Details: `docs/ci-cd.md`.

**The failure modes:**
1. **Over-privileged tokens**: a top-level `write` permission, or `id-token` / `packages: write`
   in a job that does not push or sign.
2. **Unpinned or invented action versions**: a floating tag is a supply-chain hole; an invented
   SHA breaks the run. Renovate pins majors by digest.
3. **A check that cannot fail**: `continue-on-error`, `|| true`, or a required check that is
   skipped counts as green.
4. **Drift from local**: a CI step with no local equivalent in `docs/ci-cd.md`, or a Go module
   added without joining the CI/govulncheck matrices.

## Do

- Default `permissions: contents: read`; widen per job (`packages: write`, `id-token: write`,
  `attestations: write`, `security-events: write` only in `images.yml` / the scanning jobs).
- Use the major tag (`@v4`); never write a commit SHA by hand.
- Add every new Go module to the matrices in `ci.yml` and `security.yml`, every new
  Dockerfile to `hadolint` and `images.yml`.
- Keep `ci-ok` aggregating every job in `ci.yml`; branch protection requires only it.
- Images: tags per `git-flow.md`, pushed only from `dev`/`main`, scanned before push, signed by
  digest. Allow-list findings with a justification comment (`docs/ci-cd.md`).
- Reuse `.github/actions/setup-js` instead of repeating Node/pnpm setup.

## Do NOT

- Don't add secrets or long-lived registry credentials: `GITHUB_TOKEN` and OIDC only.
- Don't use `pull_request_target`, or interpolate `${{ github.event.* }}` into `run:` (pass
  through `env:`).
- Don't weaken a gate (severity, `ignore-unfixed`, `fail-on`) to make a build pass.

## Verify

```bash
go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/*.yml
pnpm exec biome check .
```
