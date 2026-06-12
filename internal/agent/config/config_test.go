package config

import "testing"

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
				OperationKinds: []string{"project_deploy"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
