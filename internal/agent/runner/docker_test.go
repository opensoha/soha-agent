package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cfgpkg "github.com/opensoha/soha-agent/internal/agent/config"
	"go.uber.org/zap"
)

func TestPrepareComposeWorkspaceRemovesStaleEnvFileWhenEnvContentIsCleared(t *testing.T) {
	root := t.TempDir()
	runner := New(cfgpkg.ControlPlaneConfig{
		Docker: cfgpkg.DockerRunnerConfig{ComposeRoot: root},
	}, zap.NewNop())
	operation := DockerOperation{
		ID:        "operation-1",
		ProjectID: "project-1",
		Payload: map[string]any{
			"projectSlug":    "preview-api",
			"composeContent": "services:\n  api:\n    image: nginx:alpine\n",
			"envContent":     "APP_ENV=test",
		},
	}

	dir, _, err := runner.prepareComposeWorkspace(operation)
	if err != nil {
		t.Fatalf("prepareComposeWorkspace() error = %v", err)
	}
	envPath := filepath.Join(dir, ".env")
	if content, err := os.ReadFile(envPath); err != nil || !strings.Contains(string(content), "APP_ENV=test") {
		t.Fatalf(".env content = %q err=%v, want APP_ENV=test", content, err)
	}

	operation.Payload["envContent"] = ""
	if _, _, err := runner.prepareComposeWorkspace(operation); err != nil {
		t.Fatalf("prepareComposeWorkspace() clearing env error = %v", err)
	}
	if _, err := os.Stat(envPath); !os.IsNotExist(err) {
		t.Fatalf(".env stat err = %v, want not exist", err)
	}
}

func TestValidateDockerHostProvisionRequiresDockerRuntime(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	runner := New(cfgpkg.ControlPlaneConfig{}, zap.NewNop())

	logs, err := runner.validateDockerHostProvision(context.Background())
	if err == nil {
		t.Fatalf("validateDockerHostProvision() err = nil, want docker runtime error; logs=%v", logs)
	}
	if !strings.Contains(err.Error(), "docker runtime unavailable") {
		t.Fatalf("validateDockerHostProvision() err = %v, want docker runtime unavailable", err)
	}
}

func TestValidateDockerHostProvisionChecksDockerAndCompose(t *testing.T) {
	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	script := `#!/bin/sh
if [ "$1" = "info" ]; then
  echo "24.0.0 x86_64"
  exit 0
fi
if [ "$1" = "compose" ] && [ "$2" = "version" ]; then
  echo "Docker Compose version v2.27.0"
  exit 0
fi
exit 1
`
	//nolint:gosec // test creates a temporary executable fake docker binary
	if err := os.WriteFile(dockerPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake docker: %v", err)
	}
	t.Setenv("PATH", binDir)
	runner := New(cfgpkg.ControlPlaneConfig{}, zap.NewNop())

	logs, err := runner.validateDockerHostProvision(context.Background())
	if err != nil {
		t.Fatalf("validateDockerHostProvision() err = %v, logs=%v", err, logs)
	}
	joined := strings.Join(logs, "\n")
	if !strings.Contains(joined, "24.0.0 x86_64") || !strings.Contains(joined, "Docker Compose version") || !strings.Contains(joined, "docker runtime validated") {
		t.Fatalf("validateDockerHostProvision() logs = %v", logs)
	}
}

func TestDockerOperationKindAllowedRequiresExplicitAllowlist(t *testing.T) {
	cases := []struct {
		name      string
		allowed   []string
		kind      string
		wantAllow bool
	}{
		{name: "empty list denies", kind: "project_deploy", wantAllow: false},
		{name: "exact match allows", allowed: []string{"project_deploy"}, kind: "project_deploy", wantAllow: true},
		{name: "host provision allows quick-created docker hosts", allowed: []string{"host_provision"}, kind: "host_provision", wantAllow: true},
		{name: "different kind denies", allowed: []string{"host_sync"}, kind: "project_deploy", wantAllow: false},
		{name: "wildcard allows", allowed: []string{"*"}, kind: "service_action", wantAllow: true},
		{name: "empty kind denies", allowed: []string{"*"}, kind: "", wantAllow: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := dockerOperationKindAllowed(tc.allowed, tc.kind); got != tc.wantAllow {
				t.Fatalf("dockerOperationKindAllowed(%v, %q) = %t, want %t", tc.allowed, tc.kind, got, tc.wantAllow)
			}
		})
	}
}
