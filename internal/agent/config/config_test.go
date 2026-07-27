package config

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	productionAgentToken        = "agent-token-32-characters-minimum"
	productionControlPlaneToken = "runner-token-32-characters-minimum"
)

func TestValidateRequiresProductionAgentToken(t *testing.T) {
	err := Validate(Config{
		App: AppConfig{Env: "production"},
	})
	if err == nil {
		t.Fatal("Validate() succeeded, want production token error")
	}
}

func TestValidateListenSecurity(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		token   string
		wantErr string
	}{
		{name: "default wildcard requires token", addr: ":18080", wantErr: "listens beyond loopback"},
		{name: "IPv4 wildcard requires token", addr: "0.0.0.0:18080", wantErr: "listens beyond loopback"},
		{name: "IPv6 wildcard requires token", addr: "[::]:18080", wantErr: "listens beyond loopback"},
		{name: "non-loopback development requires token", addr: "192.0.2.10:18080", wantErr: "listens beyond loopback"},
		{
			name:    "non-loopback rejects short token",
			addr:    "192.0.2.10:18080",
			token:   "short-token",
			wantErr: "at least 32 characters",
		},
		{
			name:    "non-loopback rejects long demo token",
			addr:    "192.0.2.10:18080",
			token:   "demo-token-that-is-longer-than-thirty-two-characters",
			wantErr: "unsafe placeholder token",
		},
		{
			name:    "non-loopback rejects known public token",
			addr:    "192.0.2.10:18080",
			token:   knownPublicProjectToken,
			wantErr: "unsafe placeholder token",
		},
		{name: "non-loopback accepts strong token", addr: "192.0.2.10:18080", token: productionAgentToken},
		{name: "IPv4 loopback allows empty token", addr: "127.0.0.1:18080"},
		{name: "IPv4 loopback range allows empty token", addr: "127.0.0.2:18080"},
		{name: "IPv6 loopback allows empty token", addr: "[::1]:18080"},
		{name: "localhost allows empty token", addr: "localhost:18080"},
		{name: "invalid address rejected", addr: "127.0.0.1", wantErr: "valid host:port"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(Config{
				App:  AppConfig{Env: "development"},
				HTTP: HTTPConfig{Addr: tt.addr},
				Auth: AuthConfig{BearerToken: tt.token},
			})
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateRejectsProductionDemoTokens(t *testing.T) {
	err := Validate(Config{
		App:  AppConfig{Env: "production"},
		Auth: AuthConfig{BearerToken: "demo-agent-token"},
	})
	if err == nil {
		t.Fatal("Validate() succeeded, want demo token error")
	}
}

func TestValidateRejectsShortProductionAgentToken(t *testing.T) {
	err := Validate(Config{
		App:  AppConfig{Env: "production"},
		Auth: AuthConfig{BearerToken: "agent-token"},
	})
	if err == nil {
		t.Fatal("Validate() succeeded, want short token error")
	}
}

func TestValidateRequiresProductionControlPlaneToken(t *testing.T) {
	err := Validate(Config{
		App:  AppConfig{Env: "production"},
		Auth: AuthConfig{BearerToken: productionAgentToken},
		ControlPlane: ControlPlaneConfig{
			Enabled: true,
		},
	})
	if err == nil {
		t.Fatal("Validate() succeeded, want control plane token error")
	}
}

func TestValidateRequiresOutpostTrustAnchor(t *testing.T) {
	err := Validate(Config{HTTP: HTTPConfig{Addr: "127.0.0.1:18080"}, ControlPlane: ControlPlaneConfig{
		Enabled: true, BaseURL: "https://soha.example.com", BearerToken: "token",
		Outpost: OutpostConfig{Enabled: true, AgentID: "agent"},
	}})
	if err == nil || !strings.Contains(err.Error(), "trust_key_id") {
		t.Fatalf("Validate() error = %v, want trust anchor error", err)
	}
}

func TestValidateAcceptsOutpostTrustAnchor(t *testing.T) {
	err := Validate(Config{HTTP: HTTPConfig{Addr: "127.0.0.1:18080"}, ControlPlane: ControlPlaneConfig{
		Enabled: true, BaseURL: "https://soha.example.com", BearerToken: "token",
		Outpost: OutpostConfig{Enabled: true, AgentID: "agent", ProtocolVersion: "v1", TrustKeyID: "key-1", TrustPublicKey: base64.StdEncoding.EncodeToString(make([]byte, 32))},
	}})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRequiresProductionDockerOperationAllowlist(t *testing.T) {
	err := Validate(Config{
		App:  AppConfig{Env: "production"},
		Auth: AuthConfig{BearerToken: productionAgentToken},
		ControlPlane: ControlPlaneConfig{
			Enabled:     true,
			BearerToken: productionControlPlaneToken,
			Docker: DockerRunnerConfig{
				Enabled: true,
			},
		},
	})
	if err == nil {
		t.Fatal("Validate() succeeded, want docker operation allowlist error")
	}
}

func TestValidateRejectsProductionWildcardActionAllowlist(t *testing.T) {
	err := Validate(Config{
		App:      AppConfig{Env: "production"},
		Auth:     AuthConfig{BearerToken: productionAgentToken},
		Security: SecurityConfig{AllowedActions: []string{"*"}},
	})
	if err == nil {
		t.Fatal("Validate() succeeded, want wildcard action allowlist error")
	}
}

func TestValidateRejectsUnknownProductionActionAllowlistEntry(t *testing.T) {
	err := Validate(Config{
		App:      AppConfig{Env: "production"},
		Auth:     AuthConfig{BearerToken: productionAgentToken},
		Security: SecurityConfig{AllowedActions: []string{"platform.deployments.recreate"}},
	})
	if err == nil {
		t.Fatal("Validate() succeeded, want unknown action allowlist error")
	}
}

func TestValidateAllowsProductionRuntimeParityActionAllowlist(t *testing.T) {
	err := Validate(Config{
		App:  AppConfig{Env: "production"},
		Auth: AuthConfig{BearerToken: productionAgentToken},
		Security: SecurityConfig{AllowedActions: []string{
			"platform.resources.apply",
			"platform.resources.create",
			"platform.resources.delete",
			"platform.custom_resources.list",
			"platform.custom_resources.create",
			"platform.custom_resources.apply",
			"platform.custom_resources.delete",
			"platform.port_forwards.create",
			"platform.port_forwards.tunnel",
			"platform.port_forwards.delete",
			"platform.helm_releases.install",
			"platform.helm_releases.values_update",
			"platform.helm_releases.delete",
		}},
	})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRequiresProductionDockerTerminalOriginAllowlist(t *testing.T) {
	err := Validate(Config{
		App:      AppConfig{Env: "production"},
		Auth:     AuthConfig{BearerToken: productionAgentToken},
		Security: SecurityConfig{AllowedActions: []string{"docker.runtime.terminal"}},
	})
	if err == nil {
		t.Fatal("Validate() succeeded, want docker terminal origin allowlist error")
	}
}

func TestValidateRejectsProductionDockerOperationWildcard(t *testing.T) {
	err := Validate(Config{
		App:  AppConfig{Env: "production"},
		Auth: AuthConfig{BearerToken: productionAgentToken},
		ControlPlane: ControlPlaneConfig{
			Enabled:     true,
			BearerToken: productionControlPlaneToken,
			Docker: DockerRunnerConfig{
				Enabled:        true,
				OperationKinds: []string{"*"},
			},
		},
	})
	if err == nil {
		t.Fatal("Validate() succeeded, want docker operation wildcard error")
	}
}

func TestValidateRejectsUnknownProductionDockerOperationKind(t *testing.T) {
	err := Validate(Config{
		App:  AppConfig{Env: "production"},
		Auth: AuthConfig{BearerToken: productionAgentToken},
		ControlPlane: ControlPlaneConfig{
			Enabled:     true,
			BearerToken: productionControlPlaneToken,
			Docker: DockerRunnerConfig{
				Enabled:        true,
				OperationKinds: []string{"project_destroy"},
			},
		},
	})
	if err == nil {
		t.Fatal("Validate() succeeded, want unknown docker operation kind error")
	}
}

func TestValidateAllowsProductionWithExplicitTokensAndDockerAllowlist(t *testing.T) {
	err := Validate(Config{
		App:  AppConfig{Env: "production"},
		HTTP: HTTPConfig{AllowedOrigins: []string{"https://console.example"}},
		Auth: AuthConfig{BearerToken: productionAgentToken},
		Security: SecurityConfig{AllowedActions: []string{
			"platform.deployments.restart",
			"docker.runtime.terminal",
		}},
		ControlPlane: ControlPlaneConfig{
			Enabled:     true,
			BearerToken: productionControlPlaneToken,
			Docker: DockerRunnerConfig{
				Enabled:        true,
				OperationKinds: []string{"host_provision", "project_deploy"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestLoadUsesExplicitConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.config.yaml")
	if err := os.WriteFile(path, []byte("app:\n  name: custom-agent\n"), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	t.Setenv("SOHA_AGENT_CONFIG_FILE", path)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.App.Name != "custom-agent" {
		t.Fatalf("App.Name = %q, want custom-agent", cfg.App.Name)
	}
}

func TestLoadUsesLoopbackDefaultAddress(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.config.yaml")
	if err := os.WriteFile(path, []byte("app:\n  name: custom-agent\n"), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	t.Setenv("SOHA_AGENT_CONFIG_FILE", path)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTP.Addr != "127.0.0.1:18080" {
		t.Fatalf("HTTP.Addr = %q, want loopback default", cfg.HTTP.Addr)
	}
}
