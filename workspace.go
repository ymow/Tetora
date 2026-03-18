package main

import "tetora/internal/workspace"

// --- Type Aliases ---

// SessionScope defines trust and tool constraints per session type.
type SessionScope = workspace.SessionScope

// --- Workspace Resolution ---

func resolveWorkspace(cfg *Config, agentName string) WorkspaceConfig {
	return workspace.ResolveWorkspace(cfg, agentName)
}

func defaultWorkspace(cfg *Config) WorkspaceConfig {
	return workspace.DefaultWorkspace(cfg)
}

func initDirectories(cfg *Config) error {
	return workspace.InitDirectories(cfg)
}

// --- Session Scope Resolution ---

func resolveSessionScope(cfg *Config, agentName string, sessionType string) SessionScope {
	return workspace.ResolveSessionScope(cfg, agentName, sessionType)
}

func defaultToolProfile(cfg *Config) string {
	return workspace.DefaultToolProfile(cfg)
}

func minTrust(a, b string) string {
	return workspace.MinTrust(a, b)
}

// --- MCP Server Scoping ---

func resolveMCPServers(cfg *Config, agentName string) []string {
	return workspace.ResolveMCPServers(cfg, agentName)
}

// --- Soul File Loading ---

func loadSoulFile(cfg *Config, agentName string) string {
	return workspace.LoadSoulFile(cfg, agentName)
}

// --- Workspace Memory Scope ---

func getWorkspaceMemoryPath(cfg *Config) string {
	return workspace.GetWorkspaceMemoryPath(cfg)
}

func getWorkspaceSkillsPath(cfg *Config) string {
	return workspace.GetWorkspaceSkillsPath(cfg)
}
