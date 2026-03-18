package main

// mcp_host.go re-exports types and constructors from internal/mcp.
// All logic lives in internal/mcp/host.go.

import (
	"tetora/internal/mcp"
	"tetora/internal/tools"
)

// Type aliases so the rest of the root package (and tests) keep working.
type MCPHost = mcp.Host
type MCPServer = mcp.Server
type MCPServerStatus = mcp.ServerStatus

// JSON-RPC types used in tests — re-exported from internal/mcp.
type jsonRPCRequest = mcp.JSONRPCRequest
type jsonRPCResponse = mcp.JSONRPCResponse
type jsonRPCError = mcp.JSONRPCError

// Internal protocol types used in mcp_concurrent_test.go — must be accessible
// from package main tests. We re-declare them here as aliases aren't possible
// for unexported types; instead we define shim types that match the JSON shape.
// The test file uses initializeResult, toolsListResult, toolsCallResult,
// mcpProtocolVersion directly — these are package-level in mcp_host_test.go
// and mcp_concurrent_test.go (package main), so we need to expose them here.

const mcpProtocolVersion = mcp.ProtocolVersion

// initializeResult mirrors mcp.initializeResult for package main tests.
type initializeResult struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ServerInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"serverInfo"`
}

// toolsListResult mirrors mcp.toolsListResult for package main tests.
type toolsListResult struct {
	Tools []struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		InputSchema []byte          `json:"inputSchema"`
	} `json:"tools"`
}

// toolsCallResult mirrors mcp.toolsCallResult for package main tests.
type toolsCallResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text,omitempty"`
	} `json:"content"`
}

// initializeParams mirrors mcp.initializeParams for package main tests.
type initializeParams struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ClientInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"clientInfo"`
}

// toolsCallParams mirrors mcp.toolsCallParams for package main tests.
type toolsCallParams struct {
	Name      string `json:"name"`
	Arguments []byte `json:"arguments"`
}

// newMCPHost creates a new MCP host. Thin wrapper around mcp.NewHost.
func newMCPHost(cfg *Config, toolReg *tools.Registry) *MCPHost {
	return mcp.NewHost(cfg, toolReg)
}
