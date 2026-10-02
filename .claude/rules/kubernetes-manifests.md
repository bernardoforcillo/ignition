---
paths:
  - "infrastructure/kubernetes/**"
  - "apps/*/Dockerfile"
trigger: glob
globs: infrastructure/kubernetes/**,apps/*/Dockerfile
description: Kubernetes manifests live per environment, namespace and deployment, every deployable has the same set of files in each environment, and workloads run hardened with probes and an HPA-owned replica count
alwaysApply: false
---

# Rule: every deployable has the same manifests in every environment

Manifests are plain YAML under
`infrastructure/kubernetes/<environment>/<namespace>/<deployment>/`
(`development` and `production`). See `infrastructure/kubernetes/README.md`.

**The failure modes:**
1. **Environment drift.** A manifest added to one environment and forgotten in
   the other, so production behaves differently from what was tested.
2. **Fighting the autoscaler.** A pinned `replicas` next to an HPA is reset on
   every apply.
3. **Unsafe defaults.** A root container with a writable filesystem and no
   probes turns a small bug into an outage or a breach.

## Do

- **Same files, every environment.** Adding, renaming or removing a deployable
  (a `Dockerfile` plus a `Deployment`) means touching both environments in the
  same change, and its directory's `kustomization.yaml`.
- **Per deployment**: `deployment.yaml`, `service.yaml`, `pdb.yaml`, plus
  `hpa.yaml` and `configmap.yaml` when needed.
- **No `replicas` on a Deployment that has an HPA**; the HPA scales on CPU only.
- **Probes**: `startupProbe`, `readinessProbe` and `livenessProbe`, the last
  one slacker than readiness.
- **Hardening**: `runAsNonRoot`, `seccompProfile: RuntimeDefault`,
  `allowPrivilegeEscalation: false`, `readOnlyRootFilesystem: true`, drop all
  capabilities, `automountServiceAccountToken: false`.
- **Resources**: always set requests and a memory limit.
- **PDB** with `maxUnavailable: 1`.
- **Config** in a `ConfigMap`; secrets are never committed — create them with
  `kubectl create secret` and reference them with `secretKeyRef`/`secretRef`.
- **Images** are pinned to an immutable tag in `production`; `latest` only in
  placeholders.
- A comment explains every non-obvious field, not the obvious ones.

## Do NOT

- Don't commit a Secret with real values.
- Don't add overlays or patches: environments are plain, explicit files.
- Don't expose `development` publicly beyond `dev.<domain>` and don't skip the
  default-deny `NetworkPolicy`.

## Verify

Each deployment directory has the same files in both environments (should print
nothing):

```bash
cd infrastructure/kubernetes
diff <(cd development/ignition-development && find . -type f | sort) \
     <(cd production/ignition-production && find . -type f | sort)
```
