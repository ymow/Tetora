package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	iplugin "tetora/internal/plugin"
)

// --- Type Aliases ---

type PluginHost = iplugin.Host

// --- Constructor ---

// NewPluginHost creates a new PluginHost. Tool plugins are registered via the root ToolRegistry.
func NewPluginHost(cfg *Config) *PluginHost {
	return iplugin.NewHost(cfg, &pluginToolRegistrar{cfg: cfg})
}

// pluginToolRegistrar implements iplugin.ToolRegistrar using the root ToolRegistry.
type pluginToolRegistrar struct {
	cfg *Config
}

func (r *pluginToolRegistrar) RegisterPluginTool(toolName, pluginName string, call func(method string, params any) (json.RawMessage, error)) {
	if r.cfg.Runtime.ToolRegistry == nil {
		return
	}
	r.cfg.Runtime.ToolRegistry.(*ToolRegistry).Register(&ToolDef{
		Name:        toolName,
		Description: fmt.Sprintf("Plugin tool (%s) provided by plugin %q", toolName, pluginName),
		InputSchema: json.RawMessage(`{"type": "object", "properties": {"input": {"type": "object", "description": "Tool input"}}, "required": []}`),
		Handler: func(ctx context.Context, cfg *Config, input json.RawMessage) (string, error) {
			result, err := call("tool/execute", map[string]any{
				"name":  toolName,
				"input": json.RawMessage(input),
			})
			if err != nil {
				return "", err
			}
			return string(result), nil
		},
		Builtin: false,
	})
}

// --- Code Mode Meta-Tools ---

var codeModeCoreTools = map[string]bool{
	"exec":           true,
	"read":           true,
	"write":          true,
	"web_search":     true,
	"web_fetch":      true,
	"memory_search":  true,
	"agent_dispatch": true,
	"search_tools":   true,
	"execute_tool":   true,
}

const codeModeTotalThreshold = 10

func shouldUseCodeMode(registry *ToolRegistry) bool {
	if registry == nil {
		return false
	}
	return len(registry.List()) > codeModeTotalThreshold
}

func toolSearchTools(ctx context.Context, cfg *Config, input json.RawMessage) (string, error) {
	var args struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if args.Query == "" {
		return "", fmt.Errorf("query is required")
	}
	if args.Limit <= 0 {
		args.Limit = 10
	}

	if cfg.Runtime.ToolRegistry == nil {
		return "[]", nil
	}

	query := strings.ToLower(args.Query)
	var results []map[string]string

	for _, tool := range cfg.Runtime.ToolRegistry.(*ToolRegistry).List() {
		nameMatch := strings.Contains(strings.ToLower(tool.Name), query)
		descMatch := strings.Contains(strings.ToLower(tool.Description), query)
		if nameMatch || descMatch {
			results = append(results, map[string]string{
				"name":        tool.Name,
				"description": tool.Description,
			})
			if len(results) >= args.Limit {
				break
			}
		}
	}

	b, _ := json.Marshal(results)
	return string(b), nil
}

func toolExecuteTool(ctx context.Context, cfg *Config, input json.RawMessage) (string, error) {
	var args struct {
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if args.Name == "" {
		return "", fmt.Errorf("name is required")
	}

	if cfg.Runtime.ToolRegistry == nil {
		return "", fmt.Errorf("tool registry not initialized")
	}

	tool, ok := cfg.Runtime.ToolRegistry.(*ToolRegistry).Get(args.Name)
	if !ok {
		return "", fmt.Errorf("tool %q not found", args.Name)
	}

	if tool.Handler == nil {
		return "", fmt.Errorf("tool %q has no handler", args.Name)
	}

	return tool.Handler(ctx, cfg, args.Input)
}

// --- Plugin CLI ---

func cmdPlugin(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: tetora plugin <list|start|stop> [name]")
		fmt.Println()
		fmt.Println("Manage external plugins.")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  list          List configured plugins and their status")
		fmt.Println("  start <name>  Start a plugin")
		fmt.Println("  stop <name>   Stop a running plugin")
		return
	}

	cfg := loadConfig("")

	switch args[0] {
	case "list":
		if len(cfg.Plugins) == 0 {
			fmt.Println("No plugins configured.")
			return
		}
		fmt.Printf("%-20s %-10s %-10s %-30s %s\n", "NAME", "TYPE", "AUTOSTART", "COMMAND", "TOOLS")
		for name, pcfg := range cfg.Plugins {
			tools := "-"
			if len(pcfg.Tools) > 0 {
				tools = strings.Join(pcfg.Tools, ", ")
			}
			autoStart := "no"
			if pcfg.AutoStart {
				autoStart = "yes"
			}
			fmt.Printf("%-20s %-10s %-10s %-30s %s\n", name, pcfg.Type, autoStart, pcfg.Command, tools)
		}

	case "start":
		if len(args) < 2 {
			fmt.Println("Usage: tetora plugin start <name>")
			return
		}
		name := args[1]
		pcfg, ok := cfg.Plugins[name]
		if !ok {
			fmt.Printf("Plugin %q not found in config.\n", name)
			return
		}
		fmt.Printf("Starting plugin %q (type=%s, command=%s)...\n", name, pcfg.Type, pcfg.Command)
		fmt.Println("Note: plugins are managed by the daemon. Use the HTTP API to start plugins at runtime.")

	case "stop":
		if len(args) < 2 {
			fmt.Println("Usage: tetora plugin stop <name>")
			return
		}
		name := args[1]
		if _, ok := cfg.Plugins[name]; !ok {
			fmt.Printf("Plugin %q not found in config.\n", name)
			return
		}
		fmt.Printf("Note: plugins are managed by the daemon. Use the HTTP API to stop plugins at runtime.\n")

	default:
		fmt.Printf("Unknown plugin command: %s\n", args[0])
		fmt.Println("Use: tetora plugin list|start|stop")
	}
}
