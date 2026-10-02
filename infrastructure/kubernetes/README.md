# Kubernetes

Plain manifests, applied by hand (no automation yet). Layout:

```
<environment>/<namespace>/<deployment>/<manifest>.yaml
```

```
development/ignition-development/
production/ignition-production/
  kustomization.yaml  namespace.yaml  networkpolicy.yaml
  gateway/   kustomization.yaml deployment.yaml service.yaml configmap.yaml hpa.yaml pdb.yaml
  web/       kustomization.yaml deployment.yaml service.yaml pdb.yaml
  ingress/   kustomization.yaml ingress.yaml
```

`development` is served at `dev.example.com`, `production` at `example.com`.

## Usage

```sh
kubectl apply -f development/ignition-development/namespace.yaml
kubectl apply -R -f development/ignition-development
# or, with the per-directory kustomization.yaml files:
kubectl apply -k development/ignition-development
```

## Before first deploy

- Replace `ghcr.io/OWNER/...` image names (and pin real tags in production) and the `example.com` hosts.
- Set `GATEWAY_ROUTES` in each `gateway/configmap.yaml` to your real upstreams
  (the gateway refuses to start without a route).
- Secrets: the gateway reads an optional Secret `gateway-secrets` (`envFrom`).
  `kubectl -n ignition-development create secret generic gateway-secrets --from-literal=GATEWAY_AUTH_TOKEN=...`
  Add `DATABASE_URL` to switch the SaaS surface on, together with `AUTH_SECRET`
  (>= 32 bytes) and, when used, `RESEND_API_KEY`, `STRIPE_API_KEY`,
  `STRIPE_WEBHOOK_SECRET` and `BILLING_PRICES`. `APP_URL`, `COMPANY_NAME` and
  `MAIL_FROM` are in each `gateway/configmap.yaml`. Never commit these values.
- The gateway image is built from the repo root:
  `docker build -f apps/gateway/Dockerfile .`
- Requires an `nginx` ingress controller in the `ingress-nginx` namespace
  (adjust `ingressClassName` and `networkpolicy.yaml` otherwise) and
  metrics-server for the HPA.
- Images: `apps/gateway/Dockerfile` and `apps/web/Dockerfile` (build the web
  image from the repo root: `docker build -f apps/web/Dockerfile .`).

## Conventions

- The `kustomization.yaml` files only list the resources of each directory; no
  overlays or patches. Add a manifest to the directory and to its list.
- Autoscaled Deployments carry no `replicas`: the HPA owns the count. The HPA
  scales on CPU only (memory targets keep pods pinned at `maxReplicas`).
- PodDisruptionBudgets use `maxUnavailable: 1` so a drain is never blocked when
  an environment runs a single replica.
- Every container has a `startupProbe`, `readinessProbe` and `livenessProbe`,
  and runs non-root with a read-only root filesystem.
