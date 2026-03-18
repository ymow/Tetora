package main

import "tetora/internal/roles"

// --- Type Aliases ---

type AgentArchetype = roles.AgentArchetype

// builtinArchetypes is the package-level slice used by HTTP handlers and CLI.
var builtinArchetypes = roles.BuiltinArchetypes

// --- Wrapper Functions ---

func loadAgentPrompt(cfg *Config, agentName string) (string, error) {
	return roles.LoadAgentPrompt(cfg, agentName)
}

func generateSoulContent(archetype *AgentArchetype, agentName string) string {
	return roles.GenerateSoulContent(archetype, agentName)
}

func getArchetypeByName(name string) *AgentArchetype {
	return roles.GetArchetypeByName(name)
}

func writeSoulFile(cfg *Config, agentName, content string) error {
	return roles.WriteSoulFile(cfg, agentName, content)
}
