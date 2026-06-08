# Soha Agent

Standalone agent runtime for OpenSoha.

This repository builds the `soha-agent` binary from `./cmd/agent`. It is split from the open-source Soha core and must not import core repository internal packages.

## Development

```sh
go mod tidy
go test ./...
go build ./cmd/agent
```

The default config is `configs/agent.config.yaml`. Override it with `SOHA_AGENT_CONFIG_FILE` when running the binary.

```sh
SOHA_AGENT_CONFIG_FILE=configs/agent.config.yaml go run ./cmd/agent
```

## Docker

The Hermes runner image can be built with:

```sh
docker build -f deploy/Dockerfile.hermes-agent-runner -t soha-agent:hermes .
```

## License

This repository is licensed under the Apache License 2.0. See
[LICENSE](./LICENSE) for the full license text.
