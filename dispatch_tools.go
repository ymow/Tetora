package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"tetora/internal/classify"
	"tetora/internal/cost"
	"tetora/internal/estimate"
	"tetora/internal/log"
	"tetora/internal/provider"
	"tetora/internal/workspace"
)

// safeToolExec wraps tool execution with panic recovery.
func safeToolExec(ctx context.Context, cfg *Config, tool *ToolDef, input json.RawMessage) (output string, err error) {
	defer func() {
		if rv := recover(); rv != nil {
			err = fmt.Errorf("tool %q panicked: %v", tool.Name, rv)
			log.Error("tool panic recovered", "tool", tool.Name, "panic", fmt.Sprintf("%v", rv))
		}
	}()
	return tool.Handler(ctx, cfg, input)
}

// --- Agentic Loop ---

// truncateToolOutput truncates tool output to the given limit.
// If limit <= 0, defaults to 10240 chars.
func truncateToolOutput(output string, limit int) string {
	if limit <= 0 {
		limit = 10240
	}
	if len(output) <= limit {
		return output
	}
	return output[:limit] + fmt.Sprintf("\n[truncated: first %d of %d chars]", limit, len(output))
}

// executeWithProviderAndTools runs a task with tool support via agentic loop.
// If the provider supports tools and the tool registry has tools, it will:
// 1. Call provider with tools
// 2. Check for tool_use in response
// 3. Execute tools via ToolRegistry
// 4. Inject tool results back as messages
// 5. Call provider again
// 6. Repeat until no more tool_use or max iterations
func executeWithProviderAndTools(ctx context.Context, cfg *Config, task Task, agentName string, registry *providerRegistry, eventCh chan<- SSEEvent, broker *sseBroker) *ProviderResult {
	// Check if tool engine is enabled and we have a tool registry.
	if cfg.Runtime.ToolRegistry == nil {
		return executeWithProvider(ctx, cfg, task, agentName, registry, eventCh)
	}

	// Resolve provider.
	providerName := resolveProviderName(cfg, task, agentName)
	p, err := registry.Get(providerName)
	if err != nil {
		return &ProviderResult{IsError: true, Error: err.Error()}
	}

	// Check if provider supports tools.
	toolProvider, supportsTools := p.(ToolCapableProvider)
	if !supportsTools {
		// Fallback to regular execution.
		return executeWithProvider(ctx, cfg, task, agentName, registry, eventCh)
	}

	// Get available tools (filtered by agent policy and complexity).
	var allowed map[string]bool
	if task.Agent != "" {
		allowed = resolveAllowedTools(cfg, task.Agent)
	}
	// Apply complexity-based tool filtering.
	complexity := classify.Classify(task.Prompt, task.Source)
	complexityProfile := ToolsForComplexity(complexity)
	if complexityProfile != "full" && complexityProfile != "none" {
		profileAllowed := ToolsForProfile(complexityProfile)
		if profileAllowed != nil {
			if allowed == nil {
				allowed = profileAllowed
			} else {
				// Intersection: only keep tools in both sets.
				for name := range allowed {
					if !profileAllowed[name] {
						delete(allowed, name)
					}
				}
			}
		}
	}
	tools := cfg.Runtime.ToolRegistry.(*ToolRegistry).ListFiltered(allowed)
	if len(tools) == 0 {
		// No tools available, use regular execution.
		return executeWithProvider(ctx, cfg, task, agentName, registry, eventCh)
	}

	// Build initial request.
	req := buildProviderRequest(cfg, task, agentName, providerName, eventCh)
	// Convert []*ToolDef to []provider.ToolDef for the provider request.
	providerTools := make([]provider.ToolDef, len(tools))
	for i, t := range tools {
		providerTools[i] = provider.ToolDef{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
	}
	req.Tools = providerTools

	// Initialize enhanced loop detector.
	detector := NewLoopDetector()

	// Max iterations.
	maxIter := cfg.Tools.MaxIterations
	if maxIter <= 0 {
		maxIter = 10
	}

	var messages []Message
	var finalResult *ProviderResult

	// Token/cost accumulators across iterations.
	var totalTokensIn, totalTokensOut int
	var totalCostUSD float64
	var totalProviderMs int64
	var taskBudgetWarnLogged bool // soft-limit: log once and continue instead of stopping

	for i := 0; i < maxIter; i++ {
		// Check context deadline before each iteration.
		if ctx.Err() != nil {
			finalResult = &ProviderResult{
				Output: "[stopped: task deadline exceeded]",
			}
			break
		}

		req.Messages = messages

		// P27.3: Send typing indicator at iteration start.
		if cfg.StreamToChannels && task.ChannelNotifier != nil {
			go task.ChannelNotifier.SendTyping(ctx)
		}

		// Call provider.
		result, execErr := toolProvider.ExecuteWithTools(ctx, req)
		if execErr != nil {
			// If context was cancelled, treat as deadline rather than hard error.
			if ctx.Err() != nil {
				finalResult = &ProviderResult{
					Output: "[stopped: task deadline exceeded]",
				}
				break
			}
			return &ProviderResult{IsError: true, Error: execErr.Error()}
		}
		if result.IsError {
			return result
		}

		// Accumulate metrics.
		totalTokensIn += result.TokensIn
		totalTokensOut += result.TokensOut
		totalCostUSD += result.CostUSD
		totalProviderMs += result.ProviderMs

		// Check stop reason.
		if result.StopReason != "tool_use" || len(result.ToolCalls) == 0 {
			// No more tool calls, we're done.
			finalResult = result
			break
		}

		// Publish SSE event for tool calls.
		if broker != nil {
			for _, tc := range result.ToolCalls {
				// Extract a one-line preview from the tool input.
				var preview string
				if len(tc.Input) > 0 {
					var inputMap map[string]any
					if err := json.Unmarshal(tc.Input, &inputMap); err == nil {
						if desc, ok := inputMap["description"].(string); ok && desc != "" {
							preview = desc
						} else if cmd, ok := inputMap["command"].(string); ok && cmd != "" {
							if idx := strings.Index(cmd, "\n"); idx != -1 {
								preview = cmd[:idx]
							} else {
								preview = cmd
							}
						}
					}
				}
				broker.PublishMulti([]string{task.ID, task.SessionID, SSEDashboardKey}, SSEEvent{
					Type:      "tool_call",
					TaskID:    task.ID,
					SessionID: task.SessionID,
					Data: map[string]any{
						"id":      tc.ID,
						"name":    tc.Name,
						"preview": preview,
					},
				})
			}
		}

		// Execute tools.
		toolResults := make([]ToolResult, 0, len(result.ToolCalls))
		for _, tc := range result.ToolCalls {
			// Check tool policy - is tool allowed for this agent?
			if task.Agent != "" && !isToolAllowed(cfg, task.Agent, tc.Name) {
				log.WarnCtx(ctx, "tool call blocked by policy", "tool", tc.Name, "agent", task.Agent)
				toolResults = append(toolResults, ToolResult{
					ToolUseID: tc.ID,
					Content:   fmt.Sprintf("error: tool %q not allowed by policy for agent %q", tc.Name, task.Agent),
					IsError:   true,
				})
				continue
			}

			// Check for loop using enhanced detector.
			isLoop, loopMsg := detector.Check(tc.Name, tc.Input)
			if isLoop {
				log.WarnCtx(ctx, "tool loop detected", "tool", tc.Name, "msg", loopMsg)
				toolResults = append(toolResults, ToolResult{
					ToolUseID: tc.ID,
					Content:   loopMsg,
					IsError:   true,
				})
				continue
			}

			// Check for repeating pattern.
			if i > 2 { // Only check after a few iterations.
				if hasPattern, patternMsg := detector.detectToolLoopPattern(); hasPattern {
					log.WarnCtx(ctx, "tool pattern detected", "msg", patternMsg)
					toolResults = append(toolResults, ToolResult{
						ToolUseID: tc.ID,
						Content:   patternMsg,
						IsError:   true,
					})
					continue
				}
			}

			// Record tool call for loop detection.
			detector.Record(tc.Name, tc.Input)

			// Apply trust-level filtering.
			rootTC := ToolCall{ID: tc.ID, Name: tc.Name, Input: tc.Input}
			if mockResult, shouldExec := filterToolCall(cfg, task.Agent, rootTC); !shouldExec {
				// Tool call filtered by trust level (observe or suggest mode).
				toolResults = append(toolResults, *mockResult)
				continue
			}

			// P28.0: Pre-execution approval gate.
			if needsApproval(cfg, tc.Name) && task.ApprovalGate != nil && !task.ApprovalGate.IsAutoApproved(tc.Name) {
				approved, gateErr := requestToolApproval(ctx, cfg, task, rootTC)
				if gateErr != nil || !approved {
					toolResults = append(toolResults, ToolResult{
						ToolUseID: tc.ID,
						Content:   fmt.Sprintf("[REJECTED: tool %s requires approval — %s]", tc.Name, gateReason(gateErr, approved)),
						IsError:   true,
					})
					continue
				}
			}

			// Get tool handler.
			tool, ok := cfg.Runtime.ToolRegistry.(*ToolRegistry).Get(tc.Name)
			if !ok {
				toolResults = append(toolResults, ToolResult{
					ToolUseID: tc.ID,
					Content:   fmt.Sprintf("error: tool %q not found", tc.Name),
					IsError:   true,
				})
				continue
			}

			// Execute tool (with panic recovery + per-tool timeout).
			toolTimeout := time.Duration(cfg.Tools.ToolTimeout) * time.Second
			if toolTimeout <= 0 {
				toolTimeout = 30 * time.Second
			}
			toolCtx, toolCancel := context.WithTimeout(ctx, toolTimeout)
			toolStart := time.Now()
			output, err := safeToolExec(toolCtx, cfg, tool, tc.Input)
			toolCancel()
			toolDuration := time.Since(toolStart)
			if toolCtx.Err() == context.DeadlineExceeded && err == nil {
				err = fmt.Errorf("tool %q timed out after %v", tc.Name, toolTimeout)
			}

			tr := ToolResult{ToolUseID: tc.ID}
			if err != nil {
				tr.Content = fmt.Sprintf("error: %v", err)
				tr.IsError = true
			} else {
				tr.Content = truncateToolOutput(output, cfg.Tools.ToolOutputLimit)
			}
			toolResults = append(toolResults, tr)

			// P27.3: Send tool status to channel.
			if cfg.StreamToChannels && task.ChannelNotifier != nil {
				statusMsg := fmt.Sprintf("%s: done (%dms)", tc.Name, toolDuration.Milliseconds())
				go task.ChannelNotifier.SendStatus(ctx, statusMsg)
			}

			// Publish SSE event for tool result.
			if broker != nil {
				broker.PublishMulti([]string{task.ID, task.SessionID, SSEDashboardKey}, SSEEvent{
					Type:      "tool_result",
					TaskID:    task.ID,
					SessionID: task.SessionID,
					Data: map[string]any{
						"id":       tc.ID,
						"name":     tc.Name,
						"duration": toolDuration.Milliseconds(),
						"isError":  tr.IsError,
					},
				})
			}
		}

		// Build assistant message with tool uses.
		var assistantContent []ContentBlock
		if result.Output != "" {
			assistantContent = append(assistantContent, ContentBlock{
				Type: "text",
				Text: result.Output,
			})
		}
		for _, tc := range result.ToolCalls {
			assistantContent = append(assistantContent, ContentBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Name,
				Input: tc.Input,
			})
		}
		assistantMsg, _ := json.Marshal(assistantContent)
		messages = append(messages, Message{
			Role:    "assistant",
			Content: assistantMsg,
		})

		// Build user message with tool results.
		var userContent []ContentBlock
		for _, tr := range toolResults {
			userContent = append(userContent, ContentBlock{
				Type:      "tool_result",
				ToolUseID: tr.ToolUseID,
				Content:   tr.Content,
				IsError:   tr.IsError,
			})
		}
		userMsg, _ := json.Marshal(userContent)
		messages = append(messages, Message{
			Role:    "user",
			Content: userMsg,
		})

		// --- Mid-loop budget + context + deadline checks ---

		// Context deadline check: stop if task timeout has expired.
		if ctx.Err() != nil {
			finalResult = &ProviderResult{
				Output: result.Output + "\n[stopped: task deadline exceeded]",
			}
			break
		}

		// Per-task budget soft limit: log once for analysis, then continue.
		if task.Budget > 0 && totalCostUSD >= task.Budget && !taskBudgetWarnLogged {
			taskBudgetWarnLogged = true
			log.WarnCtx(ctx, "task budget soft-limit exceeded (continuing)",
				"budget", task.Budget,
				"spent", totalCostUSD,
				"role", task.Agent,
				"task_id", task.ID,
				"task_prompt_preview", task.Prompt[:min(120, len(task.Prompt))],
			)
		}

		// Global budget check.
		if br := cost.CheckBudget(cfg.Budgets, cfg.HistoryDB, agentName, "", 0); br != nil && !br.Allowed {
			log.WarnCtx(ctx, "global budget exceeded mid-loop", "msg", br.Message)
			finalResult = &ProviderResult{
				Output:  result.Output + "\n[stopped: global budget exceeded]",
				IsError: true,
				Error:   "global budget exceeded",
			}
			break
		}

		// Pre-send token estimation: compress old messages if nearing context window.
		ctxWindow := estimate.ContextWindow(req.Model)
		threshold := ctxWindow * 80 / 100
		req.Messages = messages // update for estimation
		estTokens := estimateRequestTokens(req)
		if estTokens > threshold {
			// Try compression first before stopping.
			messages = compressMessages(messages, 3)
			req.Messages = messages
			estTokens = estimateRequestTokens(req)
			if estTokens > threshold {
				log.WarnCtx(ctx, "context window limit after compression", "estimatedTokens", estTokens, "threshold", threshold)
				finalResult = &ProviderResult{
					Output:  result.Output + "\n[stopped: context limit reached]",
					IsError: true,
					Error:   "context window limit reached",
				}
				break
			}
			log.InfoCtx(ctx, "compressed old messages to fit context window", "estimatedTokens", estTokens, "threshold", threshold)
		}
	}

	if finalResult == nil {
		// Max iterations reached without final answer.
		finalResult = &ProviderResult{
			IsError: true,
			Error:   fmt.Sprintf("max tool iterations (%d) reached", maxIter),
		}
	}

	// Set accumulated totals on final result.
	finalResult.TokensIn = totalTokensIn
	finalResult.TokensOut = totalTokensOut
	finalResult.CostUSD = totalCostUSD
	finalResult.ProviderMs = totalProviderMs

	return finalResult
}

// --- Workspace Content Injection ---

// injectWorkspaceContent applies the three-tier workspace injection:
// always: workspace/rules/ directory
// agent: agent-specific rules from workspace/rules/{agentName}*
// on-demand: memory only via {{memory.KEY}} template
func injectWorkspaceContent(cfg *Config, task *Task, agentName string) {
	workspace.InjectContent(cfg, &task.SystemPrompt, &task.AddDirs, agentName)
}

// estimateDirSize returns the total size of all files (non-recursive) in a directory.
func estimateDirSize(dir string) int {
	return workspace.DirSize(dir)
}
