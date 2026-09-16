package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	cfgpkg "github.com/opensoha/soha-agent/internal/agent/config"
	"go.uber.org/zap"
)

// Explicit opt-in only: uses the operator's already authenticated Codex CLI.
func TestCodexRuntimeContractLive(t *testing.T) {
	if os.Getenv("SOHA_LIVE_CODEX") != "1" {
		t.Skip("set SOHA_LIVE_CODEX=1 for a live second-runtime contract check")
	}
	if _, err := exec.LookPath("codex"); err != nil {
		t.Fatal(err)
	}
	adapter, err := filepath.Abs("../../../deploy/codex-runner/adapter.py")
	if err != nil {
		t.Fatal(err)
	}
	runner := New(cfgpkg.ControlPlaneConfig{AgentRuntime: cfgpkg.AgentRuntimeConfig{WorkspaceRoot: t.TempDir()}}, zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	output, _, err := runner.executeCLIAgentRun(ctx, AgentRun{ID: "second-runtime-proof", ProviderID: "codex", ProviderKind: "codex", CapabilityID: "root_cause", Input: map[string]any{"question": "Summarize the supplied observation without tools: the service health endpoint returned HTTP 200. State the limited conclusion; do not infer database health.", "marker": "soha-second-runtime-0910"}}, agentProviderCommandSpec{Command: "python3", Args: []string{adapter}})
	if err != nil {
		t.Fatalf("Codex contract adapter failed: %v; %v", err, output["summary"])
	}
	if output["contractEcho"] != "soha.agentRuntime.v1" || output["markerEcho"] != "soha-second-runtime-0910" || output["externalRunId"] == "" || output["summary"] == "" {
		t.Fatalf("invalid second-runtime output: %+v", output)
	}
	t.Logf("second runtime returned structured summary, contract and marker; external run=%v, usage=%v", output["externalRunId"], output["usage"])
}
