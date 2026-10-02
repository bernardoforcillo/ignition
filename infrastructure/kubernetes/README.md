# Kubernetes

[Kustomize](https://kustomize.io) manifests for the `gateway` and `web` apps.

```
base/                shared manifests (deployments, services, ingress, HPA, PDB, network policies)
overlays/dev         1 replica, dev host, namespace ignition-dev
overlays/production  pinned images, more replicas, namespace ignition-prod
```

## Usage

```sh
kubectl kustomize infrastructure/kubernetes/overlays/dev   # render
kubectl apply -k infrastructure/kubernetes/overlays/dev    # deploy
```

## Before first deploy

- Replace `ghcr.io/OWNER/...` image names and the `example.com` host.
- Set `GATEWAY_ROUTES` in `base/kustomization.yaml` to your real upstreams
  (the gateway refuses to start without a route).
- Create the optional auth secret:
  `kubectl -n ignition-dev create secret generic gateway-secrets --from-literal=GATEWAY_AUTH_TOKEN=...`
- Requires an `nginx` ingress controller in the `ingress-nginx` namespace;
  adjust `ingressClassName` and `networkpolicy.yaml` otherwise. The HPA needs
  metrics-server.
- Images: `apps/gateway/Dockerfile` and `apps/web/Dockerfile` (build the web
  image from the repo root: `docker build -f apps/web/Dockerfile .`).
