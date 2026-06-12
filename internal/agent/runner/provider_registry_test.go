package runner

import (
	"path/filepath"
	"testing"

	cfgpkg "github.com/opensoha/soha-agent/internal/agent/config"
	"go.uber.org/zap"
)

func TestDefaultAgentProviderRegistryDeclaresHermesCapabilities(t *testing.T) {
	registry := DefaultAgentProviderRegistry()
	if err := validateAgentProviderRegistry(registry); err != nil {
		t.Fatalf("validateAgentProviderRegistry() error = %v", err)
	}
	if registry.SchemaVersion != AgentProviderRegistrySchemaVersion {
		t.Fatalf("SchemaVersion = %q, want %q", registry.SchemaVersion, AgentProviderRegistrySchemaVersion)
	}

	provider, ok := defaultAgentProviderDefinition("hermes")
	if !ok {
		t.Fatal("defaultAgentProviderDefinition(\"hermes\") not found")
	}
	if provider.ID != "hermes" || provider.Kind != "hermes" {
		t.Fatalf("provider identity = %#v", provider)
	}
	if provider.DefaultCommand.Command != "hermes" {
		t.Fatalf("default command = %#v, want hermes", provider.DefaultCommand)
	}

	capabilities := providerCapabilityIDs(provider)
	want := []string{"incident_handoff", "release_verification", "root_cause"}
	if len(capabilities) != len(want) {
		t.Fatalf("capabilities = %#v, want %#v", capabilities, want)
	}
	for index := range want {
		if capabilities[index] != want[index] {
			t.Fatalf("capabilities = %#v, want %#v", capabilities, want)
		}
	}
}

func TestAgentProviderCommandSpecUsesRegistryDefaultAndHermesOverride(t *testing.T) {
	customHermes := filepath.Join(t.TempDir(), "custom-hermes")
	runner := New(cfgpkg.ControlPlaneConfig{
		AgentRuntime: cfgpkg.AgentRuntimeConfig{
			HermesCommand: customHermes,
		},
	}, zap.NewNop())

	spec := runner.agentProviderCommandSpec("hermes")
	if spec.Command != customHermes {
		t.Fatalf("Command = %q, want %q", spec.Command, customHermes)
	}
	if len(spec.Args) != 2 || spec.Args[0] != "chat" || spec.Args[1] != "-Q" {
		t.Fatalf("Args = %#v, want chat -Q", spec.Args)
	}
	if spec.PromptArg != "-q" || spec.ProviderSkillArg != "-s" {
		t.Fatalf("prompt/provider skill args = %#v", spec)
	}
}
