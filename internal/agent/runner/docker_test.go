package runner

import (
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

func TestDockerOperationKindAllowedRequiresExplicitAllowlist(t *testing.T) {
	cases := []struct {
		name      string
		allowed   []string
		kind      string
		wantAllow bool
	}{
		{name: "empty list denies", kind: "project_deploy", wantAllow: false},
		{name: "exact match allows", allowed: []string{"project_deploy"}, kind: "project_deploy", wantAllow: true},
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
