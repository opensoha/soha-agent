# Codex CLI contract adapter

This optional adapter verifies that another external runtime can consume the existing
`soha.agentRuntime.v1` analysis prompt and return a normalized final JSON result.
Codex owns model execution. Soha does not introduce another model or tool loop.

Requirements: Python 3 and an already installed, authenticated `codex` CLI. The adapter
uses an empty temporary working directory, read-only sandbox, no user config, no shell,
local-image, web-search or MCP tools, and ephemeral sessions. It never copies credentials.
Unexpected command, file-change, MCP or web-search events fail the run.

Configure an existing CLI provider with `command: python3`, `args` containing the
absolute path to `adapter.py`, and an empty `prompt_arg`. This adapter accepts supplied
context for analysis capabilities. It does not advertise `general`, incremental chat,
or the Soha tool bridge; Hermes remains the interactive chat runtime.

Run the explicit live compatibility check from this repository:

```sh
SOHA_LIVE_CODEX=1 go test ./internal/agent/runner -run TestCodexRuntimeContractLive -v -count=1
```

Ordinary tests skip this check. It makes a real model request using local CLI auth;
no secrets or private conversation data are part of the test fixture.

Protocol references: [Codex non-interactive mode](https://learn.chatgpt.com/docs/non-interactive-mode)
and [configuration reference](https://learn.chatgpt.com/docs/config-file/config-reference).
