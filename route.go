package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"tetora/internal/audit"
	dtypes "tetora/internal/dispatch"
	"tetora/internal/log"
	"tetora/internal/webhook"
)

// devQALoopResult holds the outcome of a Dev↔QA retry loop.
type devQALoopResult struct {
	Result     TaskResult
	QAApproved bool
	Attempts   int
	TotalCost  float64
}

// --- Smart Dispatch Types (aliases to internal/dispatch) ---

type RouteRequest = dtypes.RouteRequest
type RouteResult = dtypes.RouteResult
type SmartDispatchResult = dtypes.SmartDispatchResult

// --- Binding Classification (Highest Priority) ---

// checkBindings delegates to internal/dispatch.CheckBindings.
func checkBindings(cfg *Config, req RouteRequest) *RouteResult {
	return dtypes.CheckBindings(cfg, req)
}

// --- Keyword Classification (Fast Path) ---

// classifyByKeywords delegates to internal/dispatch.ClassifyByKeywords.
func classifyByKeywords(cfg *Config, prompt string) *RouteResult {
	return dtypes.ClassifyByKeywords(cfg, prompt)
}

// --- LLM Classification (Slow Path) ---

// routeSemGlobal is a dedicated semaphore for routing LLM calls.
// Routing should never compete with task execution for slots.
var routeSemGlobal = make(chan struct{}, 5)

// classifyByLLM delegates to internal/dispatch.ClassifyByLLM, wiring
// runSingleTask+routeSemGlobal as the TaskExecutor.
func classifyByLLM(ctx context.Context, cfg *Config, prompt string) (*RouteResult, error) {
	return dtypes.ClassifyByLLM(ctx, cfg, prompt, routeExecutor(cfg))
}

// parseLLMRouteResult delegates to internal/dispatch.ParseLLMRouteResult.
func parseLLMRouteResult(output, defaultAgent string) (*RouteResult, error) {
	return dtypes.ParseLLMRouteResult(output, defaultAgent)
}

// --- Multi-Tier Route ---

// routeTask delegates to internal/dispatch.RouteTask, wiring runSingleTask as the executor.
func routeTask(ctx context.Context, cfg *Config, req RouteRequest) *RouteResult {
	return dtypes.RouteTask(ctx, cfg, req, routeExecutor(cfg))
}

// routeExecutor returns a TaskExecutor backed by runSingleTask + routeSemGlobal.
func routeExecutor(cfg *Config) dtypes.TaskExecutor {
	return dtypes.TaskExecutorFunc(func(ctx context.Context, task dtypes.Task, agentName string) dtypes.TaskResult {
		return runSingleTask(ctx, cfg, task, routeSemGlobal, nil, agentName)
	})
}

// --- Full Smart Dispatch Pipeline ---

// smartDispatch is the full pipeline: route → dispatch → memory → review → audit.
func smartDispatch(ctx context.Context, cfg *Config, prompt string, source string,
	state *dispatchState, sem, childSem chan struct{}) *SmartDispatchResult {

	// Publish task_received to dashboard.
	if state != nil && state.broker != nil {
		state.broker.Publish(SSEDashboardKey, SSEEvent{
			Type: SSETaskReceived,
			Data: map[string]any{
				"source": source,
				"prompt": truncate(prompt, 200),
			},
		})
	}

	// Step 1: Route.
	route := routeTask(ctx, cfg, RouteRequest{Prompt: prompt, Source: source})

	log.InfoCtx(ctx, "route decision",
		"prompt", truncate(prompt, 60), "role", route.Agent,
		"method", route.Method, "confidence", route.Confidence)

	// Publish task_routing to dashboard.
	if state != nil && state.broker != nil {
		state.broker.Publish(SSEDashboardKey, SSEEvent{
			Type: SSETaskRouting,
			Data: map[string]any{
				"source":     source,
				"role":       route.Agent,
				"method":     route.Method,
				"confidence": route.Confidence,
			},
		})
	}

	// Step 2: Build and run task with the selected agent.
	task := Task{
		Prompt: prompt,
		Agent:  route.Agent,
		Source: "route:" + source,
	}
	fillDefaults(cfg, &task)

	// Inject agent soul prompt + model + permission mode.
	if route.Agent != "" {
		if soulPrompt, err := loadAgentPrompt(cfg, route.Agent); err == nil && soulPrompt != "" {
			task.SystemPrompt = soulPrompt
		}
		if rc, ok := cfg.Agents[route.Agent]; ok {
			if rc.Model != "" {
				task.Model = rc.Model
			}
			if rc.PermissionMode != "" {
				task.PermissionMode = rc.PermissionMode
			}
		}
	}

	// Expand template variables.
	task.Prompt = expandPrompt(task.Prompt, "", cfg.HistoryDB, route.Agent, cfg.KnowledgeDir, cfg)

	// Step 3: Execute with optional Dev↔QA retry loop.
	taskStart := time.Now()
	var result TaskResult
	var totalCost float64
	var qaApproved bool
	var attempts int

	if cfg.SmartDispatch.ReviewLoop {
		// Dev↔QA retry loop: execute → review → retry with feedback (max N retries).
		loopResult := routeDevQALoop(ctx, cfg, task, prompt, route.Agent, sem, childSem)
		result = loopResult.Result
		totalCost = loopResult.TotalCost
		qaApproved = loopResult.QAApproved
		attempts = loopResult.Attempts
	} else {
		result = runSingleTask(ctx, cfg, task, sem, childSem, route.Agent)
		totalCost = result.CostUSD
		attempts = 1
	}

	// Record to history.
	recordHistory(cfg.HistoryDB, task.ID, task.Name, task.Source, route.Agent, task, result,
		taskStart.Format(time.RFC3339), time.Now().Format(time.RFC3339), result.OutputFile)

	// Record session activity.
	recordSessionActivity(cfg.HistoryDB, task, result, route.Agent)

	// Step 4: Store output summary in agent memory.
	if result.Status == "success" {
		setMemory(cfg, route.Agent, "last_route_output", truncate(result.Output, 500))
		setMemory(cfg, route.Agent, "last_route_prompt", truncate(prompt, 200))
		setMemory(cfg, route.Agent, "last_route_time", time.Now().Format(time.RFC3339))
	}

	sdr := &SmartDispatchResult{
		Route:    *route,
		Task:     result,
		Attempts: attempts,
	}

	// Use accumulated cost from all attempts.
	if totalCost > result.CostUSD {
		sdr.Task.CostUSD = totalCost
	}

	// Step 5: Review gate.
	if cfg.SmartDispatch.ReviewLoop {
		// Dev↔QA loop already handled review — propagate the result.
		sdr.ReviewOK = &qaApproved
		if !qaApproved && attempts > 1 {
			sdr.Review = fmt.Sprintf("Dev↔QA loop exhausted (%d attempts)", attempts)
		}
	} else if shouldReview(cfg, route, result.CostUSD) && result.Status == "success" {
		// Single-pass review (original behavior).
		reviewOK, reviewComment := reviewOutput(ctx, cfg, prompt, result.Output, route.Agent, sem, childSem)
		sdr.ReviewOK = &reviewOK
		sdr.Review = reviewComment
	}

	// Step 6: Audit log.
	audit.Log(cfg.HistoryDB, "route.dispatch", source,
		fmt.Sprintf("role=%s method=%s confidence=%s attempts=%d prompt=%s",
			route.Agent, route.Method, route.Confidence, attempts, truncate(prompt, 100)), "")

	// Webhook notifications.
	sendWebhooks(cfg, result.Status, webhook.Payload{
		JobID:    task.ID,
		Name:     task.Name,
		Source:   task.Source,
		Status:   result.Status,
		Cost:     totalCost,
		Duration: result.DurationMs,
		Model:    result.Model,
		Output:   truncate(result.Output, 500),
		Error:    truncate(result.Error, 300),
	})

	return sdr
}

// --- Route Dev↔QA Loop ---

// routeDevQALoop runs the Dev↔QA retry loop for smart dispatch.
// Unlike the taskboard version, this operates without a TaskBoard record.
//
// Flow: Dev execute → QA review → (pass → done) | (fail → record failure → inject feedback → retry)
func routeDevQALoop(ctx context.Context, cfg *Config, task Task, originalPrompt, agentName string, sem, childSem chan struct{}) devQALoopResult {
	maxRetries := cfg.SmartDispatch.MaxRetriesOrDefault() // default 3

	var accumulated float64

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Step 1: Dev execution.
		result := runSingleTask(ctx, cfg, task, sem, childSem, agentName)
		accumulated += result.CostUSD

		// If execution itself failed, exit loop immediately.
		if result.Status != "success" || strings.TrimSpace(result.Output) == "" {
			return devQALoopResult{Result: result, Attempts: attempt + 1, TotalCost: accumulated}
		}

		// Step 2: QA review.
		reviewOK, reviewComment := reviewOutput(ctx, cfg, originalPrompt, result.Output, agentName, sem, childSem)
		if reviewOK {
			log.InfoCtx(ctx, "routeDevQA: review passed", "agent", agentName, "attempt", attempt+1)
			return devQALoopResult{Result: result, QAApproved: true, Attempts: attempt + 1, TotalCost: accumulated}
		}

		// QA failed.
		log.InfoCtx(ctx, "routeDevQA: review failed, injecting feedback",
			"agent", agentName, "attempt", attempt+1, "maxAttempts", maxRetries+1,
			"comment", truncate(reviewComment, 200))

		// Record QA rejection as skill failure for future context injection.
		qaFailMsg := fmt.Sprintf("[QA rejection attempt %d] %s", attempt+1, reviewComment)
		skills := selectSkills(cfg, task)
		for _, s := range skills {
			appendSkillFailure(cfg, s.Name, task.Name, agentName, qaFailMsg)
		}

		if attempt == maxRetries {
			log.WarnCtx(ctx, "routeDevQA: max retries exhausted, escalating",
				"agent", agentName, "attempts", maxRetries+1)
			return devQALoopResult{Result: result, Attempts: attempt + 1, TotalCost: accumulated}
		}

		// Step 3: Rebuild prompt with failure context + QA feedback for retry.
		task.Prompt = originalPrompt

		// Inject accumulated skill failures.
		for _, s := range skills {
			failures := loadSkillFailuresByName(cfg, s.Name)
			if failures != "" {
				task.Prompt += fmt.Sprintf("\n\n<skill-failures name=\"%s\">\n%s\n</skill-failures>", s.Name, failures)
			}
		}

		// Inject QA reviewer's specific feedback.
		task.Prompt += fmt.Sprintf("\n\n## QA Review Feedback (Attempt %d)\n", attempt+1)
		task.Prompt += "The QA reviewer rejected the output. Issues found:\n"
		task.Prompt += reviewComment
		task.Prompt += fmt.Sprintf("\n\nAddress ALL issues above. This is retry %d of %d.\n", attempt+2, maxRetries+1)

		// Fresh session for retry.
		task.ID = newUUID()
		task.SessionID = newUUID()
	}

	return devQALoopResult{}
}

// --- Coordinator Review ---

// reviewOutput asks the review agent (or coordinator) to review the agent's output.
func reviewOutput(ctx context.Context, cfg *Config, originalPrompt, output, agentRole string, sem, childSem chan struct{}) (bool, string) {
	// Use dedicated review agent if configured, otherwise fall back to coordinator.
	reviewer := cfg.SmartDispatch.Coordinator
	if cfg.SmartDispatch.ReviewAgent != "" {
		reviewer = cfg.SmartDispatch.ReviewAgent
	}

	reviewPrompt := fmt.Sprintf(
		`Review this agent output for quality and correctness.

Original request: %s

Agent (%s) output:
%s

Reply with ONLY a JSON object:
{"ok":true,"comment":"brief comment"} or {"ok":false,"comment":"what's wrong and what evidence is missing"}`,
		truncate(originalPrompt, 300),
		agentRole,
		truncate(output, 2000),
	)

	task := Task{
		Prompt:  reviewPrompt,
		Timeout: cfg.SmartDispatch.ClassifyTimeout,
		Budget:  cfg.SmartDispatch.ReviewBudget,
		Source:  "route-review",
	}
	fillDefaults(cfg, &task)

	// Inject review agent's SOUL prompt and model.
	if soulPrompt, err := loadAgentPrompt(cfg, reviewer); err == nil && soulPrompt != "" {
		task.SystemPrompt = soulPrompt
	}
	if rc, ok := cfg.Agents[reviewer]; ok {
		if rc.Model != "" {
			task.Model = rc.Model
		}
		if rc.PermissionMode != "" {
			task.PermissionMode = rc.PermissionMode
		}
	}

	result := runSingleTask(ctx, cfg, task, sem, childSem, reviewer)
	if result.Status != "success" {
		return true, "review skipped (error)"
	}

	// Parse review JSON.
	start := strings.Index(result.Output, "{")
	end := strings.LastIndex(result.Output, "}")
	if start >= 0 && end > start {
		var review struct {
			OK      bool   `json:"ok"`
			Comment string `json:"comment"`
		}
		if json.Unmarshal([]byte(result.Output[start:end+1]), &review) == nil {
			return review.OK, review.Comment
		}
	}

	return true, "review parse error"
}

// --- Conditional Review Trigger ---

// shouldReview delegates to internal/dispatch.ShouldReview.
func shouldReview(cfg *Config, routeResult *RouteResult, taskCost float64) bool {
	return dtypes.ShouldReview(cfg, routeResult, taskCost)
}
