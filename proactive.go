package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	iproactive "tetora/internal/proactive"
)

// --- Type aliases ---

// ProactiveEngine is an alias for the canonical engine in internal/proactive.
type ProactiveEngine = iproactive.Engine

// ProactiveRuleInfo is an alias for the public rule view used by the API.
type ProactiveRuleInfo = iproactive.RuleInfo

// --- Constructor ---

// newProactiveEngine creates a ProactiveEngine wired to root-level globals.
func newProactiveEngine(cfg *Config, broker *sseBroker, sem, childSem chan struct{}) *ProactiveEngine {
	deps := iproactive.Deps{
		RunTask: func(ctx context.Context, task Task, sem, childSem chan struct{}, agentName string) TaskResult {
			return runSingleTask(ctx, cfg, task, sem, childSem, agentName)
		},
		RecordHistory: func(dbPath string, task Task, result TaskResult, agentName, startedAt, finishedAt, outputFile string) {
			recordHistory(dbPath, task.ID, task.Name, task.Source, agentName, task, result, startedAt, finishedAt, outputFile)
		},
		FillDefaults: func(c *Config, t *Task) {
			fillDefaults(c, t)
		},
	}
	return iproactive.New(cfg, broker, sem, childSem, deps)
}

// --- CLI Handler ---

// runProactive handles the `tetora proactive` CLI command.
func runProactive(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: tetora proactive <subcommand>")
		fmt.Println()
		fmt.Println("Subcommands:")
		fmt.Println("  list          List all proactive rules")
		fmt.Println("  trigger <name> Manually trigger a rule")
		fmt.Println("  status        Show engine status")
		return
	}

	cfg := loadConfig("")

	switch args[0] {
	case "list":
		iproactive.CmdList(cfg)
	case "trigger":
		if len(args) < 2 {
			fmt.Println("Usage: tetora proactive trigger <rule-name>")
			return
		}
		iproactive.CmdTrigger(cfg, args[1])
	case "status":
		iproactive.CmdStatus(cfg)
	default:
		fmt.Printf("Unknown subcommand: %s\n", args[0])
	}
}

// cmdProactiveTrigger delegates to the daemon API (kept for backward compat with main.go if referenced).
func cmdProactiveTrigger(cfg *Config, ruleName string) {
	apiURL := fmt.Sprintf("http://%s/api/proactive/rules/%s/trigger", cfg.ListenAddr, ruleName)

	req, err := http.NewRequest("POST", apiURL, nil)
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}

	if cfg.APIToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIToken)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		fmt.Printf("Rule %q triggered successfully.\n", ruleName)
		return
	}
	fmt.Printf("Error: HTTP %d\n", resp.StatusCode)
}
