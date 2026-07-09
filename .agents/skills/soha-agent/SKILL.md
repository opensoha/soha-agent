---
name: soha-agent
description: >-
  Implement or review the standalone Soha agent runtime in `cmd/agent/**`,
  `internal/agent/**`, `internal/domain/**`, `configs/agent.config.yaml`,
  `deploy/**`, Dockerfiles, release workflows, and agent-facing README content.
  Use when changing the agent HTTP API, Kubernetes proxy routes, Helm/resource
  mutation handlers, pod logs/terminal/port-forward streams, config loading,
  production security validation, action allowlists and audit, control-plane
  claim/heartbeat/callback runners, Docker operation runners, Agent Runtime
  provider execution, Hermes runner packaging, or Kubernetes deployment assets.
  This skill enforces standalone-agent boundaries, no imports from the core
  `soha` repository internals, explicit mutation allowlists, redacted errors,
  contract DTO compatibility, and runner idempotency around terminal states.
---

# Soha Agent

## Overview

Use this skill for the `soha-agent` repository only. The agent is a standalone
runtime that exposes local cluster and runtime APIs, claims work from the Soha
control plane, and calls back with task, Docker, or Agent Runtime results.

## Workflow

1. Keep `cmd/agent/main.go` thin. Startup belongs in `internal/agent/bootstrap`, config in `internal/agent/config`, HTTP surfaces in `internal/agent/api`, Kubernetes access in `internal/agent/kubernetes`, and control-plane execution in `internal/agent/runner`.
2. Do not import core `github.com/opensoha/soha/internal/**` packages. Share API shapes through released `github.com/opensoha/soha-contracts` types.
3. Use `SOHA_AGENT_CONFIG_FILE`, `SOHA_AGENT_*` env overrides, and `configs/agent.config.yaml` as the config path. Keep viper env keys aligned with config struct tags.
4. When adding a runtime capability, decide whether it is local HTTP API, Kubernetes proxy, execution runner, Docker runner, Agent Runtime provider work, or packaging. Put it in the existing owner.
5. Keep diagnostics safe. `/api/v1/diagnostics` may summarize readiness and counts; it must not expose bearer tokens, kubeconfig contents, command secrets, or raw internal failures.

## Security Rules

- Production config must reject demo or short bearer tokens, wildcard mutation allowlists, unknown mutation actions, wildcard Docker operation kinds, and Docker terminal access without explicit `http.allowed_origins`.
- All mutating agent API routes must pass through `actionPolicy.Require(...)` with a named action constant.
- When adding a new mutation action, update `internal/agent/api/security.go`, `internal/agent/config/config.go` production validation, `configs/agent.config.yaml`, and focused deny/allow tests.
- Public error responses must stay generic. Keep details such as IPs, kubeconfig paths, tokens, Docker command output, and provider stderr out of HTTP error bodies.
- Action audit logs may record action, decision, route, request ID, client IP, and reason; never include secrets or raw request payloads.

## Runner Rules

- Execution, Docker, and Agent Runtime work are claim/heartbeat/callback loops. Preserve idempotency around terminal states, cancel, timeout, retry, and stale callbacks.
- Respect `control_plane.max_concurrency`, `default_timeout`, `callback_retry`, and workspace roots.
- Keep workspace path handling bounded under configured roots. Do not let task payloads write arbitrary host paths.
- For Agent Runtime providers, normalize provider output into control-plane callback contracts. Do not leak provider-native state directly into external APIs.
- Metrics and active-task views should reflect claims, misses, heartbeats, callbacks, failures, cancellations, and timeouts.

## Packaging Rules

- `deploy/Dockerfile` builds the generic `soha-agent` image.
- `deploy/Dockerfile.hermes-agent-runner` builds the Hermes-derived runner image.
- `deploy/kubernetes/**` is a starter manifest; Helm charts live in `opensoha/soha-helm`.
- Docker builds may use sibling `../soha-contracts` only as build context. The committed module should stay compatible with released contract tags.

## Testing

- Run `go test ./...` for normal changes.
- Run `GOWORK=off go test ./...` before module, contracts, Docker, or release changes.
- Run `go test ./internal/agent/api` for auth, action allowlist, route, and stream changes.
- Run `go test ./internal/agent/runner` for claim/callback, Docker runner, Agent Runtime, cancellation, timeout, and metrics changes.
- Run `go test ./internal/agent/kubernetes` for Kubernetes proxy behavior, YAML, logs, terminal, Helm, port-forward, and CRD changes.
- Run `go vet ./...` and `go test -race ./...` for concurrency or runner changes.
