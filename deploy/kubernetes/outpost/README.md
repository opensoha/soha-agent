# Soha Outpost

This artifact runs the standard `ghcr.io/opensoha/soha-agent` image published by the agent release workflow. Outpost is a forward-auth decision point; the protected application traffic continues through the ingress or reverse proxy. Check out an agent release that advertises Identity Outpost protocol `v1`, replace the control-plane URL and pinned Ed25519 trust key in `soha-outpost.yaml`, and pin the image before applying it:

```bash
git checkout vX.Y.Z
cd deploy/kubernetes/outpost
kubectl create namespace soha-outpost
kubectl create secret generic soha-outpost-secrets \
  --namespace soha-outpost \
  --from-file=agent-token \
  --from-file=control-plane-token
kustomize edit set image ghcr.io/opensoha/soha-agent=ghcr.io/opensoha/soha-agent:vX.Y.Z
kubectl apply --namespace soha-outpost -k .
```

The readiness probe stays unavailable until a valid, signed and unexpired configuration has been claimed.

The ingress controller must authenticate its subrequests with either `Authorization: Bearer <agent-token>` or `X-Soha-Outpost-Token: <agent-token>`. Keep that token in the ingress controller's secret-management path; do not commit it in an Ingress or Middleware resource. Browser sessions are accepted from `X-Soha-Session-Token` or the `soha_proxy_session` cookie. For applications under a shared parent domain, configure the Proxy provider `cookieDomain` (for example `.example.com`) so the callback cookie reaches the protected host.

Integration templates:

- [NGINX Ingress](examples/nginx-ingress.md)
- [Traefik ForwardAuth](examples/traefik.md)

Protocol compatibility:

| Component | Requirement |
| --- | --- |
| Soha control plane | Identity Outpost runtime API with protocol `v1` |
| soha-agent | Release containing Identity Outpost protocol `v1` |
| Configuration | `protocol_version: v1` and the matching pinned Ed25519 public key |

Server and agent versions do not need identical SemVer values. Protocol `v1`, the signed configuration key, and the public runtime API are the compatibility boundary.

Render and validate the artifact without a cluster:

```bash
kubectl kustomize deploy/kubernetes/outpost | kubectl apply --dry-run=client -f -
```
