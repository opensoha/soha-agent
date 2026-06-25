# Soha Agent

Standalone agent runtime for OpenSoha.

This repository builds the `soha-agent` binary from `./cmd/agent`. It is split from the open-source Soha core and must not import core repository internal packages.

## Development

```sh
go mod tidy
go test ./...
go build ./cmd/agent
```

Build metadata is injected with Go ldflags and is available from the CLI and HTTP API:

```sh
go build -trimpath \
  -ldflags "-X github.com/opensoha/soha-agent/internal/agent/buildinfo.Version=v0.1.0 -X github.com/opensoha/soha-agent/internal/agent/buildinfo.Commit=$(git rev-parse --short HEAD) -X github.com/opensoha/soha-agent/internal/agent/buildinfo.Date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o bin/soha-agent ./cmd/agent
bin/soha-agent --version
curl -s http://127.0.0.1:18080/api/v1/build-info
```

The default config is `configs/agent.config.yaml`. Override it with `SOHA_AGENT_CONFIG_FILE` when running the binary.

```sh
SOHA_AGENT_CONFIG_FILE=configs/agent.config.yaml go run ./cmd/agent
```

## Production hardening

Set `app.env: production` only with non-demo bearer tokens. Production config validation rejects wildcard mutation allowlists, unknown mutation actions, wildcard Docker operation kinds, and Docker terminal access without explicit `http.allowed_origins`.

Mutation actions are denied by default. Add only the operations this agent is expected to perform:

```yaml
security:
  allowed_actions:
    - platform.pods.exec
    - platform.deployments.restart
    - platform.deployments.scale
    - platform.deployments.image
    - platform.deployments.rollback
    - platform.statefulsets.restart
    - platform.statefulsets.scale
    - platform.daemonsets.restart
    - runtime.execution_tasks.cancel
    - docker.runtime.terminal
```

Docker runner operation kinds are a separate allowlist under `control_plane.docker.operation_kinds`; keep it to the exact kinds the control plane should claim, for example `host_provision`, `project_deploy`, or `service_action`.

Structured action audit is always written to the configured logger. To persist high-risk action decisions as JSON Lines, set:

```yaml
audit:
  file_path: /var/log/soha-agent/actions.jsonl
```

Runner execution controls live under `control_plane`:

```yaml
control_plane:
  max_concurrency: 1
  default_timeout: 30m
  callback_retry:
    max_attempts: 3
    backoff: 500ms
```

Runtime metrics are available from `GET /api/v1/runtime/metrics` when the runner is enabled.
Local diagnostics are available from `GET /api/v1/diagnostics`; the response is a safe summary of build info, enabled runtimes, worker counts, metrics availability, and managed-agent capability readiness. It does not return bearer tokens or kubeconfig contents.

## Docker

The generic cluster agent image can be built with:

```sh
make deploy-agent-image IMAGE_TAG=v0.1.0
```

The Hermes runner image can be built separately with:

```sh
make deploy-hermes-image IMAGE_TAG=v0.1.0
```

The release workflow publishes multi-arch Linux images (`linux/amd64`, `linux/arm64`) to Docker Hub as `yshanchui/soha-agent` and `yshanchui/soha-hermes-agent`, plus binary archives for Linux, macOS, and Windows. Each archive has a `.sha256` sidecar plus a release-level `SHA256SUMS` manifest.

## Helm

The Helm charts are published from `opensoha/soha-helm`:

```sh
helm repo add opensoha https://raw.githubusercontent.com/opensoha/soha-helm/gh-pages
helm repo update
helm install soha-agent opensoha/soha-agent \
  --namespace soha-agent \
  --create-namespace \
  --set secrets.agentBearerToken=REPLACE_WITH_AGENT_TOKEN \
  --set secrets.controlPlaneBearerToken=REPLACE_WITH_RUNNER_TOKEN

helm install soha-hermes-agent opensoha/soha-hermes-agent \
  --namespace soha-agent \
  --create-namespace \
  --set secrets.controlPlaneBearerToken=REPLACE_WITH_RUNNER_TOKEN
```

## License

This repository is licensed under the Apache License 2.0. See
[LICENSE](./LICENSE) for the full license text.
