package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// TaskBoardDispatcher auto-dispatches tasks with status=todo and a non-empty assignee.
//
// Polling rules (resource contention prevention):
//   - Scans every Interval (default 5m) via ticker.
//   - If any tasks from the previous cycle are still running (activeCount > 0),
//     the scan is skipped — no new tasks are launched until all current ones finish.
//   - When activeCount drops to zero, doneCh fires an immediate extra scan so the
//     next batch starts without waiting for the full interval.
//   - MaxConcurrentTasks caps how many tasks are started per scan cycle (0 = unlimited).
//   - resetStuckDoing() always runs at scan time to recover from daemon crashes.
type TaskBoardDispatcher struct {
	engine *TaskBoardEngine
	cfg    *Config
	sem      chan struct{}
	childSem chan struct{}
	state    *dispatchState

	mu            sync.Mutex
	wg            sync.WaitGroup // tracks in-flight dispatchTask goroutines
	activeCount   atomic.Int32   // number of currently running dispatchTask goroutines
	running       bool
	stopCh        chan struct{}
	doneCh        chan struct{} // signals when activeCount drops to 0 → immediate re-scan
	lastTriageAt  time.Time    // last backlog triage time (cooldown tracking)
	ctx           context.Context
	cancel        context.CancelFunc
}

func newTaskBoardDispatcher(engine *TaskBoardEngine, cfg *Config, sem, childSem chan struct{}, state *dispatchState) *TaskBoardDispatcher {
	ctx, cancel := context.WithCancel(context.Background())
	return &TaskBoardDispatcher{
		engine:   engine,
		cfg:      cfg,
		sem:      sem,
		childSem: childSem,
		state:    state,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}, 1), // buffered: at most one pending re-scan signal
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start begins the auto-dispatch loop.
func (d *TaskBoardDispatcher) Start() {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return
	}
	d.running = true
	d.mu.Unlock()

	// On startup, reset ALL "doing" tasks to "todo". The daemon just started so
	// no legitimate in-flight tasks can exist — these are orphans from a previous
	// crash or forced shutdown where the completion handler never ran.
	d.resetOrphanedDoing()

	interval := d.parseInterval()
	logInfo("taskboard auto-dispatch started", "interval", interval.String())

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-d.stopCh:
				logInfo("taskboard auto-dispatch stopped")
				return
			case <-ticker.C:
				d.scan()
			case <-d.doneCh:
				// All tasks from the previous cycle finished — re-scan immediately
				// instead of waiting for the next ticker tick.
				logInfo("taskboard auto-dispatch: all tasks done, re-scanning immediately")
				d.scan()
			}
		}
	}()
}

// Stop halts the dispatcher and waits for in-flight tasks to finish.
func (d *TaskBoardDispatcher) Stop() {
	d.mu.Lock()
	if !d.running {
		d.mu.Unlock()
		return
	}
	d.running = false
	close(d.stopCh)
	d.mu.Unlock()

	// Signal all in-flight tasks to cancel, then wait.
	d.cancel()
	d.wg.Wait()
	logInfo("taskboard dispatch: all in-flight tasks finished")
}

func (d *TaskBoardDispatcher) parseInterval() time.Duration {
	raw := d.engine.config.AutoDispatch.Interval
	if raw == "" {
		raw = "5m"
	}
	dur, err := time.ParseDuration(raw)
	if err != nil {
		logWarn("invalid dispatch interval, using 5m", "raw", raw, "error", err)
		return 5 * time.Minute
	}
	return dur
}

func (d *TaskBoardDispatcher) parseStuckThreshold() time.Duration {
	raw := d.engine.config.AutoDispatch.StuckThreshold
	if raw == "" {
		raw = "2h"
	}
	dur, err := time.ParseDuration(raw)
	if err != nil {
		logWarn("invalid stuck threshold, using 2h", "raw", raw, "error", err)
		return 2 * time.Hour
	}
	return dur
}

// resetOrphanedDoing resets ALL tasks in "doing" back to "todo" unconditionally.
// Called once at startup — since the daemon just started, any task in "doing" is
// an orphan from a previous crash or forced shutdown.
func (d *TaskBoardDispatcher) resetOrphanedDoing() {
	sql := `SELECT id, title FROM tasks WHERE status = 'doing'`
	rows, err := queryDB(d.engine.dbPath, sql)
	if err != nil {
		logWarn("taskboard dispatch: resetOrphanedDoing query failed", "error", err)
		return
	}
	if len(rows) == 0 {
		return
	}

	now := escapeSQLite(time.Now().UTC().Format(time.RFC3339))
	for _, row := range rows {
		id := fmt.Sprintf("%v", row["id"])
		title := fmt.Sprintf("%v", row["title"])

		updateSQL := fmt.Sprintf(
			`UPDATE tasks SET status = 'todo', updated_at = '%s' WHERE id = '%s' AND status = 'doing'`,
			now, escapeSQLite(id),
		)
		if err := execDB(d.engine.dbPath, updateSQL); err != nil {
			logWarn("taskboard dispatch: failed to reset orphaned task", "id", id, "error", err)
			continue
		}

		comment := "[auto-reset] Orphaned in 'doing' at daemon startup. Reset to 'todo' for re-dispatch."
		if _, err := d.engine.AddComment(id, "system", comment); err != nil {
			logWarn("taskboard dispatch: failed to add orphan reset comment", "id", id, "error", err)
		}

		logInfo("taskboard dispatch: reset orphaned doing task", "id", id, "title", title)
	}
}

// resetStuckDoing resets tasks that have been stuck in "doing" longer than StuckThreshold
// back to "todo" so they can be re-dispatched. This handles daemon crash/restart scenarios
// where in-flight tasks never received their completion callback.
func (d *TaskBoardDispatcher) resetStuckDoing() {
	threshold := d.parseStuckThreshold()
	cutoff := time.Now().Add(-threshold).UTC().Format(time.RFC3339)

	sql := fmt.Sprintf(`SELECT id, title FROM tasks WHERE status = 'doing' AND updated_at < '%s'`, escapeSQLite(cutoff))
	rows, err := queryDB(d.engine.dbPath, sql)
	if err != nil {
		logWarn("taskboard dispatch: resetStuckDoing query failed", "error", err)
		return
	}

	for _, row := range rows {
		id := fmt.Sprintf("%v", row["id"])
		title := fmt.Sprintf("%v", row["title"])

		updateSQL := fmt.Sprintf(
			`UPDATE tasks SET status = 'todo', updated_at = '%s' WHERE id = '%s' AND status = 'doing'`,
			escapeSQLite(time.Now().UTC().Format(time.RFC3339)),
			escapeSQLite(id),
		)
		if err := execDB(d.engine.dbPath, updateSQL); err != nil {
			logWarn("taskboard dispatch: failed to reset stuck task", "id", id, "error", err)
			continue
		}

		comment := fmt.Sprintf("[auto-reset] Stuck in 'doing' for >%s (likely daemon restart). Reset to 'todo' for re-dispatch.", threshold)
		if _, err := d.engine.AddComment(id, "system", comment); err != nil {
			logWarn("taskboard dispatch: failed to add reset comment", "id", id, "error", err)
		}

		logInfo("taskboard dispatch: reset stuck doing task", "id", id, "title", title, "threshold", threshold)
	}
}

// scan finds todo tasks with an assignee and dispatches them.
// If any tasks are still running from a previous cycle, the dispatch is skipped
// to avoid resource contention. resetStuckDoing always runs to handle crash recovery.
func (d *TaskBoardDispatcher) scan() {
	// Always reset stuck tasks first (handles crash/restart scenarios regardless of active count).
	d.resetStuckDoing()

	// Skip dispatch if tasks from the previous cycle are still running.
	if n := d.activeCount.Load(); n > 0 {
		logInfo("taskboard dispatch: scan skipped, waiting for running tasks", "active", n)
		return
	}

	tasks, err := d.engine.ListTasks("todo", "", "")
	if err != nil {
		logWarn("taskboard dispatch scan error", "error", err)
		return
	}
	if len(tasks) == 0 {
		logDebug("taskboard dispatch: scan found no todo tasks")
		d.triageBacklog()
		return
	}

	maxTasks := d.engine.config.AutoDispatch.MaxConcurrentTasks
	if maxTasks <= 0 {
		maxTasks = 3
	}
	dispatched := 0

	for _, t := range tasks {
		if t.Assignee == "" {
			logInfo("taskboard dispatch: skipping unassigned task", "id", t.ID, "title", t.Title)
			continue
		}
		if dispatched >= maxTasks {
			logInfo("taskboard dispatch: maxConcurrentTasks reached, deferring remaining tasks", "limit", maxTasks)
			break
		}

		logInfo("taskboard dispatch: picking up task", "id", t.ID, "title", t.Title, "assignee", t.Assignee)
		dispatched++

		// Dispatch in a goroutine with panic recovery.
		d.wg.Add(1)
		d.activeCount.Add(1)
		go func(task TaskBoard) {
			defer d.wg.Done()
			defer func() {
				// Decrement active count; signal for immediate re-scan if last task done.
				if remaining := d.activeCount.Add(-1); remaining == 0 {
					select {
					case d.doneCh <- struct{}{}:
					default: // already signaled, skip
					}
				}
			}()
			defer func() {
				if r := recover(); r != nil {
					logError("taskboard dispatch: panic in dispatchTask", "id", task.ID, "recover", r)
					if _, err := d.engine.MoveTask(task.ID, "failed"); err != nil {
						logWarn("taskboard dispatch: failed to move panicked task to failed", "id", task.ID, "error", err)
					}
				}
			}()
			d.dispatchTask(task)
		}(t)
	}
}

func (d *TaskBoardDispatcher) dispatchTask(t TaskBoard) {
	ctx := d.ctx // use dispatcher context (cancelled on Stop)

	// Build the dispatch task.
	prompt := t.Title
	if t.Description != "" {
		prompt = t.Title + "\n\n" + t.Description
	}

	// Inject dependency context from completed upstream tasks.
	if len(t.DependsOn) > 0 {
		depContext := d.buildDependencyContext(t.DependsOn)
		if depContext != "" {
			prompt += "\n\n## Previous Task Results\n" + depContext
		}
	}

	taskID := t.ID // capture for closure
	task := Task{
		Name:   "board:" + t.ID,
		Prompt: prompt,
		Agent:  t.Assignee,
		Source: "taskboard",
		onStart: func() {
			if _, err := d.engine.MoveTask(taskID, "doing"); err != nil {
				logWarn("taskboard dispatch: failed to move task to doing on start", "id", taskID, "error", err)
			}
		},
	}
	fillDefaults(d.cfg, &task)

	// Apply taskboard-specific cost controls.
	// Priority: per-task model > dispatch defaultModel > agent model > global defaultModel.
	dispatchCfg := d.engine.config.AutoDispatch
	if t.Model != "" {
		task.Model = t.Model
	} else if dispatchCfg.DefaultModel != "" {
		task.Model = dispatchCfg.DefaultModel
	}
	if dispatchCfg.MaxBudget > 0 && (task.Budget == 0 || task.Budget > dispatchCfg.MaxBudget) {
		task.Budget = dispatchCfg.MaxBudget
	}

	// Look up project workdir.
	if t.Project != "" && t.Project != "default" {
		p, err := getProject(d.cfg.HistoryDB, t.Project)
		if err == nil && p != nil && p.Workdir != "" {
			task.Workdir = p.Workdir
		}
	}

	start := time.Now()
	result := runSingleTask(ctx, d.cfg, task, d.sem, d.childSem, t.Assignee)
	duration := time.Since(start)

	// Determine target status.
	newStatus := "done"
	if result.Status != "success" {
		newStatus = "failed"
	}

	// Review gate: if requireReview is enabled, route to "review" instead of "done".
	if d.engine.config.RequireReview && newStatus == "done" && t.Status != "review" {
		newStatus = "review"
	}

	// Atomic status + cost update in a single SQL statement.
	nowISO := time.Now().UTC().Format(time.RFC3339)
	completedAt := ""
	if newStatus == "done" {
		completedAt = nowISO
	}
	combinedSQL := fmt.Sprintf(`
		UPDATE tasks SET status = '%s', cost_usd = %.6f, duration_ms = %d,
		session_id = '%s', updated_at = '%s', completed_at = '%s'
		WHERE id = '%s'
	`,
		escapeSQLite(newStatus),
		result.CostUSD,
		result.DurationMs,
		escapeSQLite(result.SessionID),
		escapeSQLite(nowISO),
		escapeSQLite(completedAt),
		escapeSQLite(t.ID),
	)
	if err := execDB(d.engine.dbPath, combinedSQL); err != nil {
		logError("taskboard dispatch: failed to update task status+cost", "id", t.ID, "error", err)
	} else {
		// Fire webhook only if DB update succeeded.
		updatedTask := t
		updatedTask.Status = newStatus
		updatedTask.UpdatedAt = nowISO
		updatedTask.CompletedAt = completedAt
		go d.engine.fireWebhook("task.moved", updatedTask)
	}

	if newStatus == "done" || newStatus == "review" {

		// Add result as comment.
		output := result.Output
		if len(output) > 2000 {
			output = output[:2000] + "\n... (truncated)"
		}
		comment := fmt.Sprintf("Task completed in %s (cost: $%.4f)\n\n%s", duration.Round(time.Second), result.CostUSD, output)
		if _, err := d.engine.AddComment(t.ID, t.Assignee, comment); err != nil {
			logWarn("taskboard dispatch: failed to add completion comment", "id", t.ID, "error", err)
		}

		// Check for auto-delegations in the output.
		delegations := parseAutoDelegate(result.Output)
		if len(delegations) > 0 {
			processAutoDelegations(ctx, d.cfg, delegations, result.Output,
				"", t.Assignee, "", d.state, d.sem, d.childSem, nil)
		}

		logInfo("taskboard dispatch: task completed", "id", t.ID, "cost", result.CostUSD, "duration", duration.Round(time.Second))
	} else {
		errMsg := result.Error
		if errMsg == "" {
			errMsg = result.Output
		}
		if len(errMsg) > 2000 {
			errMsg = errMsg[:2000] + "\n... (truncated)"
		}
		comment := fmt.Sprintf("Task failed (exit code: %d, duration: %s)\n\n%s", result.ExitCode, duration.Round(time.Second), errMsg)
		if _, err := d.engine.AddComment(t.ID, t.Assignee, comment); err != nil {
			logWarn("taskboard dispatch: failed to add failure comment", "id", t.ID, "error", err)
		}

		logWarn("taskboard dispatch: task failed", "id", t.ID, "error", result.Error)

		// Auto-retry if enabled.
		d.engine.AutoRetryFailed()
	}
}

// triageBacklog evaluates backlog tasks and promotes ready ones to todo.
// Runs at most once per BacklogTriageInterval (default 1h) to avoid excessive LLM calls.
func (d *TaskBoardDispatcher) triageBacklog() {
	triageInterval := d.parseTriageInterval()
	if time.Since(d.lastTriageAt) < triageInterval {
		return
	}

	backlog, err := d.engine.ListTasks("backlog", "", "")
	if err != nil {
		logWarn("taskboard dispatch: backlog triage query failed", "error", err)
		return
	}
	if len(backlog) == 0 {
		logDebug("taskboard dispatch: no backlog tasks to triage")
		d.lastTriageAt = time.Now()
		return
	}

	d.lastTriageAt = time.Now()

	// Build triage prompt with all backlog tasks.
	var sb strings.Builder
	sb.WriteString("你是 backlog triage agent。以下是目前所有 backlog 狀態的任務，請逐一評估是否可以推進到 todo：\n\n")
	for _, t := range backlog {
		sb.WriteString(fmt.Sprintf("- **%s** (ID: %s, assignee: %s, priority: %s)", t.Title, t.ID, t.Assignee, t.Priority))
		if len(t.DependsOn) > 0 {
			sb.WriteString(fmt.Sprintf(", depends_on: %s", strings.Join(t.DependsOn, ", ")))
		}
		sb.WriteString("\n")
		if t.Description != "" {
			sb.WriteString(fmt.Sprintf("  %s\n", t.Description))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("## 評估標準\n")
	sb.WriteString("1. 目標明確，不需要主人額外決策即可開始執行\n")
	sb.WriteString("2. 有 assignee（沒有的話判斷誰適合並 assign）\n")
	sb.WriteString("3. 依賴的上游任務都已完成（或無依賴）\n")
	sb.WriteString("4. 專案存在且可存取\n\n")
	sb.WriteString("符合條件的任務請用 taskboard_move 推進到 todo。\n")
	sb.WriteString("不符合的說明原因即可，不要動它。\n")
	sb.WriteString("最後回報：推進了幾張票、哪些、以及跳過的原因。")

	agent := d.engine.config.AutoDispatch.BacklogAgent
	if agent == "" {
		agent = "ruri"
	}

	task := Task{
		Name:   "backlog-triage",
		Prompt: sb.String(),
		Agent:  agent,
		Source: "taskboard",
	}
	fillDefaults(d.cfg, &task)

	dispatchCfg := d.engine.config.AutoDispatch
	if dispatchCfg.DefaultModel != "" {
		task.Model = dispatchCfg.DefaultModel
	}
	if dispatchCfg.MaxBudget > 0 && (task.Budget == 0 || task.Budget > dispatchCfg.MaxBudget) {
		task.Budget = dispatchCfg.MaxBudget
	}

	logInfo("taskboard dispatch: starting backlog triage", "backlogCount", len(backlog), "agent", agent)

	d.wg.Add(1)
	d.activeCount.Add(1)
	go func() {
		defer d.wg.Done()
		defer func() {
			if remaining := d.activeCount.Add(-1); remaining == 0 {
				select {
				case d.doneCh <- struct{}{}:
				default:
				}
			}
		}()
		defer func() {
			if r := recover(); r != nil {
				logError("taskboard dispatch: panic in backlog triage", "recover", r)
			}
		}()

		result := runSingleTask(d.ctx, d.cfg, task, d.sem, d.childSem, agent)
		if result.Status == "success" {
			logInfo("taskboard dispatch: backlog triage completed", "cost", result.CostUSD)
		} else {
			logWarn("taskboard dispatch: backlog triage failed", "error", result.Error)
		}
	}()
}

func (d *TaskBoardDispatcher) parseTriageInterval() time.Duration {
	raw := d.engine.config.AutoDispatch.BacklogTriageInterval
	if raw == "" {
		raw = "1h"
	}
	dur, err := time.ParseDuration(raw)
	if err != nil {
		logWarn("invalid backlog triage interval, using 1h", "raw", raw, "error", err)
		return time.Hour
	}
	return dur
}

// buildDependencyContext fetches the latest completion comment from each dependency task
// and concatenates them into context for the downstream task.
func (d *TaskBoardDispatcher) buildDependencyContext(depIDs []string) string {
	maxCtx := d.cfg.PromptBudget.contextMaxOrDefault()
	var parts []string
	totalLen := 0

	for _, depID := range depIDs {
		depTask, err := d.engine.GetTask(depID)
		if err != nil {
			continue
		}

		comments, err := d.engine.GetThread(depID)
		if err != nil || len(comments) == 0 {
			continue
		}

		// Use the last comment (most likely the completion output).
		lastComment := comments[len(comments)-1].Content
		entry := fmt.Sprintf("### %s (task %s)\n%s", depTask.Title, depID, lastComment)

		if totalLen+len(entry) > maxCtx {
			remaining := maxCtx - totalLen
			if remaining > 200 {
				parts = append(parts, truncateToChars(entry, remaining))
			}
			break
		}
		parts = append(parts, entry)
		totalLen += len(entry)
	}

	return strings.Join(parts, "\n\n")
}
