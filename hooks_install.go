package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// --- Claude Code Hooks Installer ---
// Manages Tetora hook entries in ~/.claude/settings.json.
// See: https://code.claude.com/docs/en/hooks

// claudeSettings represents the structure of ~/.claude/settings.json.
type claudeSettings struct {
	raw map[string]json.RawMessage
}

func loadClaudeSettings() (*claudeSettings, string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, "", fmt.Errorf("get home dir: %w", err)
	}

	path := filepath.Join(homeDir, ".claude", "settings.json")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Create empty settings.
			return &claudeSettings{raw: make(map[string]json.RawMessage)}, path, nil
		}
		return nil, "", fmt.Errorf("read settings: %w", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, "", fmt.Errorf("parse settings: %w", err)
	}

	return &claudeSettings{raw: raw}, path, nil
}

func (s *claudeSettings) save(path string) error {
	data, err := json.MarshalIndent(s.raw, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}

	// Ensure directory exists.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// hookCommand returns the curl command that Claude Code hooks will execute.
func hookCommand(listenAddr string) string {
	if listenAddr == "" {
		listenAddr = ":8991"
	}
	// Normalize address.
	addr := listenAddr
	if strings.HasPrefix(addr, ":") {
		addr = "localhost" + addr
	}

	return fmt.Sprintf(
		`curl -sf -X POST http://%s/api/hooks/event -H 'Content-Type:application/json' -d @/dev/stdin 2>/dev/null || true`,
		addr,
	)
}

// tetoraHookMatcher checks if a hook command is a Tetora hook.
const tetoraHookMarker = "/api/hooks/event"
const tetoraGateMarker = "/api/hooks/plan-gate"
const tetoraAskUserDenyMarker = "/api/hooks/deny-ask-user"

func isTetoraHook(cmd string) bool {
	return strings.Contains(cmd, tetoraHookMarker) || isTetoraGateHook(cmd) || isTetoraAskUserDenyHook(cmd)
}

func isTetoraGateHook(cmd string) bool {
	return strings.Contains(cmd, tetoraGateMarker)
}

func isTetoraAskUserDenyHook(cmd string) bool {
	return strings.Contains(cmd, tetoraAskUserDenyMarker)
}

// --- New hook format (Claude Code 2025+) ---
// Each event has an array of hookRule objects. Each hookRule has:
//   - matcher: regex string filtering when hooks fire (optional)
//   - hooks: array of hookHandler objects that run when matched

// hookHandler represents a single hook command within a hookRule.
type hookHandler struct {
	Type    string `json:"type"`              // "command", "http", "prompt", "agent"
	Command string `json:"command,omitempty"` // for type="command"
	Timeout int    `json:"timeout,omitempty"` // max seconds to wait
}

// hookRule represents a matcher group with one or more hook handlers.
type hookRule struct {
	Matcher string        `json:"matcher,omitempty"` // regex filter (tool name, etc.)
	Hooks   []hookHandler `json:"hooks"`
}

// hooksConfig represents the hooks section of Claude Code settings.
type hooksConfig struct {
	PreToolUse   []hookRule `json:"PreToolUse,omitempty"`
	PostToolUse  []hookRule `json:"PostToolUse,omitempty"`
	Stop         []hookRule `json:"Stop,omitempty"`
	Notification []hookRule `json:"Notification,omitempty"`
}

// planGateHookCommand returns the curl command for the PreToolUse:ExitPlanMode gate.
// Note: no `|| true` — response must be read by Claude Code.
func planGateHookCommand(listenAddr string) string {
	addr := listenAddr
	if strings.HasPrefix(addr, ":") {
		addr = "localhost" + addr
	}
	return fmt.Sprintf(
		`curl -sf --max-time 300 -X POST http://%s/api/hooks/plan-gate -H 'Content-Type:application/json' -d @/dev/stdin 2>/dev/null`,
		addr,
	)
}

// askUserDenyCommand returns a short script that denies AskUserQuestion and tells Claude
// to use the tetora_ask_user MCP tool instead.
// The trailing comment contains the marker for isTetoraAskUserDenyHook() detection.
func askUserDenyCommand() string {
	return `echo '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","reason":"Use tetora_ask_user MCP tool to ask the user remotely via Discord."}}' # /api/hooks/deny-ask-user`
}

// isTetoraRule checks if a hookRule contains any Tetora hook commands.
func isTetoraRule(rule hookRule) bool {
	for _, h := range rule.Hooks {
		if isTetoraHook(h.Command) {
			return true
		}
	}
	return false
}

func isTetoraGateRule(rule hookRule) bool {
	for _, h := range rule.Hooks {
		if isTetoraGateHook(h.Command) {
			return true
		}
	}
	return false
}

func isTetoraAskUserDenyRule(rule hookRule) bool {
	for _, h := range rule.Hooks {
		if isTetoraAskUserDenyHook(h.Command) {
			return true
		}
	}
	return false
}

// installHooks adds Tetora hook entries to Claude Code settings.
func installHooks(listenAddr string) error {
	settings, path, err := loadClaudeSettings()
	if err != nil {
		return err
	}

	cmd := hookCommand(listenAddr)

	// Parse existing hooks section.
	var hooks hooksConfig
	if raw, ok := settings.raw["hooks"]; ok {
		json.Unmarshal(raw, &hooks)
	}

	// Add Tetora hooks (preserving existing non-Tetora hooks).
	hooks.PostToolUse = addTetoraRule(hooks.PostToolUse, cmd, "")
	hooks.Stop = addTetoraRule(hooks.Stop, cmd, "")
	hooks.Notification = addTetoraRule(hooks.Notification, cmd, "")

	// Add PreToolUse:ExitPlanMode gate hook.
	gateCmd := planGateHookCommand(listenAddr)
	hooks.PreToolUse = addTetoraGateRule(hooks.PreToolUse, gateCmd, "ExitPlanMode", 600)

	// Add PreToolUse:AskUserQuestion deny hook (redirects to MCP tool).
	denyCmd := askUserDenyCommand()
	hooks.PreToolUse = addTetoraAskUserDenyRule(hooks.PreToolUse, denyCmd, "AskUserQuestion")

	// Serialize hooks back.
	hooksData, err := json.Marshal(hooks)
	if err != nil {
		return fmt.Errorf("marshal hooks: %w", err)
	}
	settings.raw["hooks"] = hooksData

	if err := settings.save(path); err != nil {
		return err
	}

	fmt.Printf("Hooks installed in %s\n", path)
	fmt.Printf("Hook commands:\n")
	fmt.Printf("  event:     %s\n", cmd)
	fmt.Printf("  plan-gate: %s\n", gateCmd)
	fmt.Printf("  deny-ask:  %s\n", denyCmd)
	return nil
}

// addTetoraRule adds a Tetora hook rule, replacing any existing Tetora rule.
func addTetoraRule(rules []hookRule, cmd, matcher string) []hookRule {
	// Remove existing Tetora rules and invalid entries (null hooks from old format).
	filtered := make([]hookRule, 0, len(rules))
	for _, r := range rules {
		if len(r.Hooks) == 0 || isTetoraRule(r) {
			continue
		}
		filtered = append(filtered, r)
	}

	// Add new Tetora rule.
	rule := hookRule{
		Matcher: matcher,
		Hooks: []hookHandler{
			{Type: "command", Command: cmd},
		},
	}
	return append(filtered, rule)
}

// addTetoraGateRule adds a plan-gate PreToolUse rule, replacing any existing one.
func addTetoraGateRule(rules []hookRule, cmd, matcher string, timeout int) []hookRule {
	// Remove existing gate rules and invalid entries.
	filtered := make([]hookRule, 0, len(rules))
	for _, r := range rules {
		if len(r.Hooks) == 0 || isTetoraGateRule(r) {
			continue
		}
		filtered = append(filtered, r)
	}
	rule := hookRule{
		Matcher: matcher,
		Hooks: []hookHandler{
			{Type: "command", Command: cmd, Timeout: timeout},
		},
	}
	return append(filtered, rule)
}

// addTetoraAskUserDenyRule adds a deny rule for AskUserQuestion, replacing any existing one.
func addTetoraAskUserDenyRule(rules []hookRule, cmd, matcher string) []hookRule {
	filtered := make([]hookRule, 0, len(rules))
	for _, r := range rules {
		if len(r.Hooks) == 0 || isTetoraAskUserDenyRule(r) {
			continue
		}
		filtered = append(filtered, r)
	}
	rule := hookRule{
		Matcher: matcher,
		Hooks: []hookHandler{
			{Type: "command", Command: cmd, Timeout: 10},
		},
	}
	return append(filtered, rule)
}

// removeHooks removes all Tetora hook entries from Claude Code settings.
func removeHooks() error {
	settings, path, err := loadClaudeSettings()
	if err != nil {
		return err
	}

	raw, ok := settings.raw["hooks"]
	if !ok {
		fmt.Println("No hooks configured.")
		return nil
	}

	var hooks hooksConfig
	if err := json.Unmarshal(raw, &hooks); err != nil {
		return fmt.Errorf("parse hooks: %w", err)
	}

	removed := 0
	hooks.PreToolUse, removed = removeTetoraRules(hooks.PreToolUse)
	hooks.PostToolUse, removed = removeTetoraRulesCount(hooks.PostToolUse, removed)
	hooks.Stop, removed = removeTetoraRulesCount(hooks.Stop, removed)
	hooks.Notification, removed = removeTetoraRulesCount(hooks.Notification, removed)

	if removed == 0 {
		fmt.Println("No Tetora hooks found.")
		return nil
	}

	hooksData, _ := json.Marshal(hooks)
	settings.raw["hooks"] = hooksData

	if err := settings.save(path); err != nil {
		return err
	}

	fmt.Printf("Removed %d Tetora hook(s) from %s\n", removed, path)
	return nil
}

func removeTetoraRules(rules []hookRule) ([]hookRule, int) {
	filtered := make([]hookRule, 0, len(rules))
	removed := 0
	for _, r := range rules {
		if isTetoraRule(r) {
			removed++
		} else {
			filtered = append(filtered, r)
		}
	}
	return filtered, removed
}

func removeTetoraRulesCount(rules []hookRule, prevRemoved int) ([]hookRule, int) {
	result, r := removeTetoraRules(rules)
	return result, prevRemoved + r
}

// showHooksStatus displays the current state of Tetora hooks.
func showHooksStatus() error {
	settings, path, err := loadClaudeSettings()
	if err != nil {
		return err
	}

	fmt.Printf("Settings file: %s\n\n", path)

	raw, ok := settings.raw["hooks"]
	if !ok {
		fmt.Println("No hooks configured.")
		return nil
	}

	var hooks hooksConfig
	if err := json.Unmarshal(raw, &hooks); err != nil {
		return fmt.Errorf("parse hooks: %w", err)
	}

	found := false
	checkRuleType := func(name string, rules []hookRule) {
		for _, r := range rules {
			if isTetoraRule(r) {
				for _, h := range r.Hooks {
					fmt.Printf("  %s: %s\n", name, h.Command)
				}
				if r.Matcher != "" {
					fmt.Printf("    matcher: %s\n", r.Matcher)
				}
				found = true
			}
		}
	}

	fmt.Println("Tetora hooks:")
	checkRuleType("PreToolUse", hooks.PreToolUse)
	checkRuleType("PostToolUse", hooks.PostToolUse)
	checkRuleType("Stop", hooks.Stop)
	checkRuleType("Notification", hooks.Notification)

	if !found {
		fmt.Println("  (none installed)")
	}

	return nil
}
