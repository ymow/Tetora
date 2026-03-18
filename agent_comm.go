package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	dtypes "tetora/internal/dispatch"
	"tetora/internal/log"
)

// --- Agent Communication Tools ---
// These are registered as built-in tools in the tool registry.

// spawnTracker is a type alias for internal/dispatch.SpawnTracker.
// Root code that references the concrete type (App.SpawnTracker, tests) continues to compile.
type spawnTracker = dtypes.SpawnTracker

// globalSpawnTracker is the package-level spawn tracker instance.
var globalSpawnTracker = dtypes.NewSpawnTracker()

// newSpawnTracker creates a new SpawnTracker (used in tests).
func newSpawnTracker() *spawnTracker { return dtypes.NewSpawnTracker() }

// childSemConcurrentOrDefault delegates to internal/dispatch.
func childSemConcurrentOrDefault(cfg *Config) int {
	return dtypes.ChildSemConcurrentOrDefault(cfg)
}

// maxDepthOrDefault delegates to internal/dispatch.
func maxDepthOrDefault(cfg *Config) int {
	return dtypes.MaxDepthOrDefault(cfg)
}

// maxChildrenPerTaskOrDefault delegates to internal/dispatch.
func maxChildrenPerTaskOrDefault(cfg *Config) int {
	return dtypes.MaxChildrenPerTaskOrDefault(cfg)
}

// toolAgentList delegates to internal/dispatch.ToolAgentList.
func toolAgentList(ctx context.Context, cfg *Config, input json.RawMessage) (string, error) {
	return dtypes.ToolAgentList(ctx, cfg, input)
}

// toolAgentMessage delegates to internal/dispatch.ToolAgentMessage.
func toolAgentMessage(ctx context.Context, cfg *Config, input json.RawMessage) (string, error) {
	return dtypes.ToolAgentMessage(ctx, cfg, input)
}

// generateMessageID delegates to internal/dispatch.GenerateMessageID.
func generateMessageID() string {
	return dtypes.GenerateMessageID()
}

// initAgentCommDB delegates to internal/dispatch.InitAgentCommDB.
func initAgentCommDB(dbPath string) error {
	return dtypes.InitAgentCommDB(dbPath)
}

// getAgentMessages delegates to internal/dispatch.GetAgentMessages.
func getAgentMessages(dbPath, role string, markAsRead bool) ([]map[string]any, error) {
	return dtypes.GetAgentMessages(dbPath, role, markAsRead)
}

// toolAgentDispatch dispatches a sub-task to another agent and waits for the result.
// This implementation calls the local HTTP API to avoid needing direct access to dispatchState.
// --- P13.3: Nested Sub-Agents --- Added depth tracking, max depth enforcement, and spawn control.
func toolAgentDispatch(ctx context.Context, cfg *Config, input json.RawMessage) (string, error) {
	var args struct {
		Agent    string  `json:"agent"`
		Role     string  `json:"role"` // backward compat
		Prompt   string  `json:"prompt"`
		Timeout  float64 `json:"timeout"`
		Depth    int     `json:"depth"`    // --- P13.3: current depth (passed by parent)
		ParentID string  `json:"parentId"` // --- P13.3: parent task ID
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if args.Agent == "" {
		args.Agent = args.Role
	}
	if args.Agent == "" {
		return "", fmt.Errorf("agent is required")
	}
	if args.Prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}
	if args.Timeout <= 0 {
		if cfg.AgentComm.DefaultTimeout > 0 {
			args.Timeout = float64(cfg.AgentComm.DefaultTimeout)
		} else {
			// Use smart estimation from prompt; default 1h if no prompt.
			estimated, err := time.ParseDuration(estimateTimeout(args.Prompt))
			if err != nil {
				estimated = time.Hour
			}
			args.Timeout = estimated.Seconds()
		}
	}

	// --- P13.3: Enforce max nesting depth.
	childDepth := args.Depth + 1
	maxDepth := maxDepthOrDefault(cfg)
	if args.Depth >= maxDepth {
		return "", fmt.Errorf("max nesting depth exceeded: current depth %d >= maxDepth %d", args.Depth, maxDepth)
	}

	// --- P13.3: Enforce max children per parent task.
	app := appFromCtx(ctx)
	maxChildren := maxChildrenPerTaskOrDefault(cfg)
	if args.ParentID != "" {
		tracker := globalSpawnTracker
		if app != nil && app.SpawnTracker != nil {
			tracker = app.SpawnTracker
		}
		if !tracker.TrySpawn(args.ParentID, maxChildren) {
			return "", fmt.Errorf("max children per task exceeded: parent %s already has %d active children (limit %d)",
				args.ParentID, tracker.Count(args.ParentID), maxChildren)
		}
		// Release when done (deferred).
		defer tracker.Release(args.ParentID)
	}

	// Check if agent exists.
	if _, ok := cfg.Agents[args.Agent]; !ok {
		return "", fmt.Errorf("agent %q not found", args.Agent)
	}

	// Build task request.
	task := Task{
		Prompt:   args.Prompt,
		Agent:    args.Agent,
		Timeout:  fmt.Sprintf("%.0fs", args.Timeout),
		Source:   "agent_dispatch",
		Depth:    childDepth,   // --- P13.3: propagate depth
		ParentID: args.ParentID, // --- P13.3: propagate parent ID
	}
	fillDefaults(cfg, &task)

	log.Debug("agent_dispatch", "agent", args.Agent, "depth", childDepth, "parentId", args.ParentID)

	// Call local HTTP API.
	requestBody, _ := json.Marshal([]Task{task})

	// Determine listen address.
	addr := cfg.ListenAddr
	if addr == "" {
		addr = "127.0.0.1:7777"
	}

	url := fmt.Sprintf("http://%s/dispatch", addr)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(requestBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tetora-Source", "agent_dispatch")

	// Add auth token if configured.
	if cfg.APIToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIToken)
	}

	// Execute request with timeout.
	client := &http.Client{
		Timeout: time.Duration(args.Timeout+10) * time.Second, // add buffer
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("dispatch request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("dispatch failed: HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Parse result.
	var dispatchResult DispatchResult
	if err := json.Unmarshal(body, &dispatchResult); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if len(dispatchResult.Tasks) == 0 {
		return "", fmt.Errorf("no task result returned")
	}

	taskResult := dispatchResult.Tasks[0]

	// Build result summary.
	result := map[string]any{
		"role":       args.Agent,
		"status":     taskResult.Status,
		"output":     taskResult.Output,
		"durationMs": taskResult.DurationMs,
		"costUsd":    taskResult.CostUSD,
	}
	if taskResult.Error != "" {
		result["error"] = taskResult.Error
	}

	b, _ := json.Marshal(result)
	return string(b), nil
}
