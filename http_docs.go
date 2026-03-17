package main

import (
	"embed"
	"net/http"

	"tetora/internal/httpapi"
)

//go:embed README.md README.*.md INSTALL.md CHANGELOG.md ROADMAP.md CONTRIBUTING.md docs/*.md
var docsFS embed.FS

var supportedDocsLangs = []string{"zh-TW", "ja", "ko", "id", "th", "fil", "es", "fr", "de"}

var docsList = []httpapi.DocsPageEntry{
	{Name: "README", File: "README.md", Description: "Project Overview"},
	{Name: "Configuration", File: "docs/configuration.md", Description: "Config Reference"},
	{Name: "Workflows", File: "docs/workflow.md", Description: "Workflow Engine"},
	{Name: "Taskboard", File: "docs/taskboard.md", Description: "Kanban & Auto-Dispatch"},
	{Name: "Hooks", File: "docs/hooks.md", Description: "Claude Code Hooks"},
	{Name: "MCP", File: "docs/mcp.md", Description: "Model Context Protocol"},
	{Name: "Discord Multitasking", File: "docs/discord-multitasking.md", Description: "Thread & Focus"},
	{Name: "Troubleshooting", File: "docs/troubleshooting.md", Description: "Common Issues"},
	{Name: "Changelog", File: "CHANGELOG.md", Description: "Release History"},
	{Name: "Roadmap", File: "ROADMAP.md", Description: "Future Plans"},
	{Name: "Contributing", File: "CONTRIBUTING.md", Description: "Contributor Guide"},
	{Name: "Installation", File: "INSTALL.md", Description: "Setup Guide"},
}

// registerDocsRoutesVia delegates to httpapi.RegisterDocsRoutes with embedded FS.
func registerDocsRoutesVia(mux *http.ServeMux) {
	httpapi.RegisterDocsRoutes(mux, docsFS, docsList, supportedDocsLangs)
}
