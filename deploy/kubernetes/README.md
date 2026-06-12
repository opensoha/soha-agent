# Soha Agent Kubernetes Deployment

This sample runs `soha-agent` in production mode with in-cluster ServiceAccount
authentication.

Before applying it, replace the `change-me` values in the Secret and update the
control-plane URLs in the ConfigMap. The placeholders intentionally fail
production config validation until real runtime tokens are supplied.

Logs are emitted as JSON to stdout; collect them with the cluster log pipeline.

High-risk action audit records are written to
`/var/log/soha-agent/actions.jsonl`. The sample keeps that path on a PVC so a
log shipper or sidecar can forward JSONL records without exposing token
material.

Runtime metrics are available at `GET /api/v1/runtime/metrics` and require the
agent bearer token. Scrape through an authenticated Prometheus job or an
internal auth proxy; do not expose the endpoint anonymously.

Render the sample with Kustomize from this repository:

```sh
kustomize build deploy/kubernetes/base
```
