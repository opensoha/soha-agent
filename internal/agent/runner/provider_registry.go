package runner

import (
	"fmt"
	"sort"
	"strings"
)

const AgentProviderRegistrySchemaVersion = "opensoha.dev/agent-provider-registry/v1"

type AgentProviderRegistry struct {
	SchemaVersion string                    `json:"schemaVersion"`
	Providers     []AgentProviderDefinition `json:"providers"`
}

type AgentProviderDefinition struct {
	ID             string                    `json:"id"`
	Kind           string                    `json:"kind"`
	DisplayName    string                    `json:"displayName"`
	DefaultCommand agentProviderCommandSpec  `json:"-"`
	Capabilities   []AgentProviderCapability `json:"capabilities"`
}

type AgentProviderCapability struct {
	ID             string   `json:"id"`
	Description    string   `json:"description"`
	RiskLevel      string   `json:"riskLevel"`
	RequiredScopes []string `json:"requiredScopes"`
}

func DefaultAgentProviderRegistry() AgentProviderRegistry {
	return AgentProviderRegistry{
		SchemaVersion: AgentProviderRegistrySchemaVersion,
		Providers: []AgentProviderDefinition{
			{
				ID:          "hermes",
				Kind:        "hermes",
				DisplayName: "Hermes Agent",
				DefaultCommand: agentProviderCommandSpec{
					Command:          "hermes",
					Args:             []string{"chat", "-Q"},
					PromptArg:        "-q",
					ProviderSkillArg: "-s",
				},
				Capabilities: []AgentProviderCapability{
					{
						ID:             "root_cause",
						Description:    "Investigate incidents or failed releases with Soha-provided read-only context.",
						RiskLevel:      "analyze",
						RequiredScopes: []string{"application", "environment", "cluster", "namespace"},
					},
					{
						ID:             "release_verification",
						Description:    "Review release evidence, task logs, and rollout state before promotion.",
						RiskLevel:      "analyze",
						RequiredScopes: []string{"application", "environment"},
					},
					{
						ID:             "incident_handoff",
						Description:    "Prepare an incident summary and next-step handoff from Soha evidence.",
						RiskLevel:      "analyze",
						RequiredScopes: []string{"application", "environment", "cluster", "namespace"},
					},
				},
			},
		},
	}
}

func validateAgentProviderRegistry(registry AgentProviderRegistry) error {
	if registry.SchemaVersion != AgentProviderRegistrySchemaVersion {
		return fmt.Errorf("unsupported provider registry schema version %q", registry.SchemaVersion)
	}
	providerIDs := map[string]struct{}{}
	providerKinds := map[string]struct{}{}
	for _, provider := range registry.Providers {
		id := strings.ToLower(strings.TrimSpace(provider.ID))
		kind := strings.ToLower(strings.TrimSpace(provider.Kind))
		if id == "" {
			return fmt.Errorf("provider id is required")
		}
		if kind == "" {
			return fmt.Errorf("provider %q kind is required", provider.ID)
		}
		if _, ok := providerIDs[id]; ok {
			return fmt.Errorf("duplicate provider id %q", id)
		}
		providerIDs[id] = struct{}{}
		if _, ok := providerKinds[kind]; ok {
			return fmt.Errorf("duplicate provider kind %q", kind)
		}
		providerKinds[kind] = struct{}{}
		if strings.TrimSpace(provider.DefaultCommand.Command) == "" {
			return fmt.Errorf("provider %q default command is required", provider.ID)
		}
		capabilityIDs := map[string]struct{}{}
		for _, capability := range provider.Capabilities {
			capabilityID := strings.ToLower(strings.TrimSpace(capability.ID))
			if capabilityID == "" {
				return fmt.Errorf("provider %q capability id is required", provider.ID)
			}
			if _, ok := capabilityIDs[capabilityID]; ok {
				return fmt.Errorf("provider %q duplicate capability %q", provider.ID, capabilityID)
			}
			capabilityIDs[capabilityID] = struct{}{}
			if strings.TrimSpace(capability.RiskLevel) == "" {
				return fmt.Errorf("provider %q capability %q risk level is required", provider.ID, capability.ID)
			}
			if len(compactStringSlice(capability.RequiredScopes)) == 0 {
				return fmt.Errorf("provider %q capability %q required scopes are required", provider.ID, capability.ID)
			}
		}
	}
	return nil
}

func defaultAgentProviderDefinition(providerKey string) (AgentProviderDefinition, bool) {
	providerKey = strings.ToLower(strings.TrimSpace(providerKey))
	if providerKey == "" {
		return AgentProviderDefinition{}, false
	}
	for _, provider := range DefaultAgentProviderRegistry().Providers {
		if strings.EqualFold(provider.ID, providerKey) || strings.EqualFold(provider.Kind, providerKey) {
			return provider, true
		}
	}
	return AgentProviderDefinition{}, false
}

func providerCapabilityIDs(provider AgentProviderDefinition) []string {
	out := make([]string, 0, len(provider.Capabilities))
	for _, capability := range provider.Capabilities {
		if capabilityID := strings.TrimSpace(capability.ID); capabilityID != "" {
			out = append(out, capabilityID)
		}
	}
	sort.Strings(out)
	return out
}
