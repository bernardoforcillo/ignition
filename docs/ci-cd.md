# CI/CD and security checks

Everything lives in `.github/`. Workflows use only `GITHUB_TOKEN`: no secrets are required
(optional: `GITLEAKS_LICENSE`, only for organisation-owned repositories). Action versions are
major tags; Renovate pins them by digest (`renovate.json`, group "github actions").

## What runs when

| Workflow | Trigger | Jobs |
|---|---|---|
| `ci.yml` | PR, push to `dev`/`main` | `lint`, `web`, `go` (matrix, 9 modules), `proto`, `email-export`, `k8s`, `e2e`, `ci-ok` |
| `security.yml` | PR, push to `main`, weekly (Mon 04:17 UTC), manual | `govulncheck` (matrix, 8 modules), `pnpm audit`, `dependency review` (PR only), `gitleaks`, `codeql` (go, js/ts), `hadolint`, `trivy` fs + IaC |
| `images.yml` | push to `dev`/`main`, manual, PRs touching code | build, Trivy gate, SBOM, push, cosign sign + attest, provenance |

Images (`ghcr.io/<owner>/ignition-{gateway,web}`): PRs build and scan only. `dev` pushes
`<UTC-timestamp>-<sha>-canary`; `main` pushes `<UTC-timestamp>-<sha>` and `latest`. The image is
scanned before it is pushed; a fixable HIGH/CRITICAL finding fails the job.

## Branch protection (dev and main)

Require status check **`ci-ok`** (aggregates every CI job) and "up to date before merging". Also
require, as you adopt them: `govulncheck (...)`, `pnpm audit`, `gitleaks`, `codeql (go)`,
`codeql (javascript-typescript)`, `dependency review`, `trivy (filesystem + IaC)`. Enable code
scanning so SARIF uploads show in the Security tab. Never push to `main` directly (`git-flow.md`).

## Run each check locally

| Check | Command |
|---|---|
| lint | `pnpm exec biome check .` |
| web | `pnpm typecheck && pnpm --filter @ignition/web test && pnpm --filter @ignition/web build` |
| go | `pnpm gen:email`, then per module: `gofmt -l .`, `go vet ./...`, `go test -short -race ./...` |
| gateway without go.work | `cd apps/gateway && GOWORK=off go build ./...` |
| proto | `pnpm lint:proto && pnpm gen:proto && git diff --exit-code` |
| email export | `pnpm gen:email && git status --porcelain go-packages/mailer/templates` (must be empty) |
| k8s | `kubectl kustomize infrastructure/kubernetes/<env>/<ns> \| kubeconform -strict -ignore-missing-schemas` |
| e2e | see `e2e/README.md` (`pnpm test:e2e`) |
| govulncheck | `cd <module> && go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...` |
| pnpm audit | `pnpm audit --prod --audit-level high` |
| hadolint | `docker run --rm -i hadolint/hadolint < apps/gateway/Dockerfile` |
| trivy | `trivy fs --scanners vuln,misconfig --severity HIGH,CRITICAL --ignore-unfixed .` |
| gitleaks | `gitleaks detect --source .` |
| workflows | `go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/*.yml` |

## Allow-listing a finding

Fix first (a Renovate PR is usually open). If it is not exploitable here, allow-list it **with a
comment saying why and when to revisit**, in a PR a reviewer can challenge:

- govulncheck: only vulnerabilities the code calls fail the job. Fix by bumping the module; do
  not disable the job.
- pnpm: add the advisory to `auditConfig` in `pnpm-workspace.yaml` (`ignoreGhsas`, or
  `ignoreCves`; check the key against the installed pnpm docs), with a comment.
- Trivy (filesystem and image): add the ID to `.trivyignore` with a comment.
- gitleaks: a `.gitleaksignore` fingerprint or `.gitleaks.toml` allowlist, with a comment.
- Dependency review: `allow-ghsas:` on the action, with a comment.

## Verify a published image

```sh
IMG=ghcr.io/<owner>/ignition-gateway@sha256:<digest>
ID='^https://github.com/<owner>/ignition/\.github/workflows/images\.yml@'
cosign verify "$IMG" --certificate-identity-regexp "$ID" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
cosign verify-attestation --type spdxjson "$IMG" --certificate-identity-regexp "$ID" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
gh attestation verify "oci://$IMG" --owner <owner>      # build provenance
```

SBOMs (SPDX and CycloneDX JSON) are workflow artifacts `sbom-<name>-spdx` / `-cyclonedx`;
attach them to a release with `gh release upload <tag> gateway.spdx.json gateway.cdx.json`.

## Known limits

- `tools` is not scanned by govulncheck (only `tool` directives, no packages: it exits 2).
- The Trivy gate scans the locally built image; the push rebuilds from the layer cache.
- gitleaks-action needs a free `GITLEAKS_LICENSE` secret on organisation-owned repositories.
