# Kubernetes

Plain manifests, applied by hand (no automation yet). Layout:

```
<environment>/<namespace>/<deployment>/<manifest>.yaml
```

```
development/ignition-development/
staging/ignition-staging/
production/ignition-production/
  namespace.yaml  networkpolicy.yaml
  gateway/   deployment.yaml service.yaml configmap.yaml hpa.yaml pdb.yaml
  web/       deployment.yaml service.yaml pdb.yaml
  ingress/   ingress.yaml   (staging and production only)
```

`development` is not exposed publicly (no Ingress): reach it with
`kubectl port-forward`. `staging` is served at `dev.example.com`, `production`
at `example.com`.

## Usage

```sh
kubectl apply -f development/ignition-development/namespace.yaml
kubectl apply -R -f development/ignition-development
```

## Before first deploy

- Replace `ghcr.io/OWNER/...` image names (and pin real tags in staging and
  production) and the `example.com` hosts.
- Set `GATEWAY_ROUTES` in each `gateway/configmap.yaml` to your real upstreams
  (the gateway refuses to start without a route).
- Optional auth secret:
  `kubectl -n ignition-development create secret generic gateway-secrets --from-literal=GATEWAY_AUTH_TOKEN=...`
- Requires an `nginx` ingress controller in the `ingress-nginx` namespace
  (adjust `ingressClassName` and `networkpolicy.yaml` otherwise) and
  metrics-server for the HPA.
- Images: `apps/gateway/Dockerfile` and `apps/web/Dockerfile` (build the web
  image from the repo root: `docker build -f apps/web/Dockerfile .`).
