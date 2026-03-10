package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// --- Cron Expression Parser ---

// cronExpr represents a parsed 5-field cron expression.
// Fields: minute(0-59) hour(0-23) dom(1-31) month(1-12) dow(0-6, 0=Sunday)
type cronExpr struct {
	minutes []bool // [60]
	hours   []bool // [24]
	doms    []bool // [32] (index 0 unused)
	months  []bool // [13] (index 0 unused)
	dows    []bool // [7]
}

func (e cronExpr) matches(t time.Time) bool {
	return e.minutes[t.Minute()] &&
		e.hours[t.Hour()] &&
		e.doms[t.Day()] &&
		e.months[int(t.Month())] &&
		e.dows[int(t.Weekday())]
}

func parseCronExpr(s string) (cronExpr, error) {
	fields := strings.Fields(s)
	if len(fields) != 5 {
		return cronExpr{}, fmt.Errorf("expected 5 fields, got %d", len(fields))
	}

	minutes, err := parseField(fields[0], 0, 59)
	if err != nil {
		return cronExpr{}, fmt.Errorf("minute: %w", err)
	}
	hours, err := parseField(fields[1], 0, 23)
	if err != nil {
		return cronExpr{}, fmt.Errorf("hour: %w", err)
	}
	doms, err := parseField(fields[2], 1, 31)
	if err != nil {
		return cronExpr{}, fmt.Errorf("dom: %w", err)
	}
	months, err := parseField(fields[3], 1, 12)
	if err != nil {
		return cronExpr{}, fmt.Errorf("month: %w", err)
	}
	dows, err := parseField(fields[4], 0, 6)
	if err != nil {
		return cronExpr{}, fmt.Errorf("dow: %w", err)
	}

	e := cronExpr{
		minutes: make([]bool, 60),
		hours:   make([]bool, 24),
		doms:    make([]bool, 32),
		months:  make([]bool, 13),
		dows:    make([]bool, 7),
	}
	for _, v := range minutes {
		e.minutes[v] = true
	}
	for _, v := range hours {
		e.hours[v] = true
	}
	for _, v := range doms {
		e.doms[v] = true
	}
	for _, v := range months {
		e.months[v] = true
	}
	for _, v := range dows {
		e.dows[v] = true
	}
	return e, nil
}

// parseField parses a single cron field. Supports: *, N, N-M, */N, N-M/S, N,M,O
func parseField(field string, min, max int) ([]int, error) {
	var result []int

	for _, part := range strings.Split(field, ",") {
		vals, err := parsePart(part, min, max)
		if err != nil {
			return nil, err
		}
		result = append(result, vals...)
	}

	return result, nil
}

func parsePart(part string, min, max int) ([]int, error) {
	// Handle step: */N or N-M/S
	step := 1
	if idx := strings.Index(part, "/"); idx != -1 {
		s, err := strconv.Atoi(part[idx+1:])
		if err != nil || s <= 0 {
			return nil, fmt.Errorf("bad step in %q", part)
		}
		step = s
		part = part[:idx]
	}

	var lo, hi int

	switch {
	case part == "*":
		lo, hi = min, max

	case strings.Contains(part, "-"):
		parts := strings.SplitN(part, "-", 2)
		var err error
		lo, err = strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("bad range start in %q", part)
		}
		hi, err = strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("bad range end in %q", part)
		}

	default:
		v, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("bad value %q", part)
		}
		if v < min || v > max {
			return nil, fmt.Errorf("value %d out of bounds [%d,%d]", v, min, max)
		}
		if step == 1 {
			return []int{v}, nil
		}
		lo, hi = v, max
	}

	if lo < min || hi > max || lo > hi {
		return nil, fmt.Errorf("range %d-%d out of bounds [%d,%d]", lo, hi, min, max)
	}

	var vals []int
	for v := lo; v <= hi; v += step {
		vals = append(vals, v)
	}
	return vals, nil
}

// nextRunAfter finds the next time after `after` that matches the cron expression.
func nextRunAfter(expr cronExpr, loc *time.Location, after time.Time) time.Time {
	// Start from the next minute.
	t := after.Truncate(time.Minute).Add(time.Minute)

	// Search up to 366 days ahead.
	limit := t.Add(366 * 24 * time.Hour)
	for t.Before(limit) {
		if expr.matches(t) {
			return t
		}
		// Skip ahead intelligently.
		if !expr.months[int(t.Month())] {
			// Skip to next month.
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, loc)
			continue
		}
		if !expr.doms[t.Day()] || !expr.dows[int(t.Weekday())] {
			// Skip to next day.
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, loc)
			continue
		}
		if !expr.hours[t.Hour()] {
			// Skip to next hour.
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, loc)
			continue
		}
		// Skip to next minute.
		t = t.Add(time.Minute)
	}
	return time.Time{} // no match found
}

// --- Helpers ---

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// seedDefaultJobs returns the default cron jobs for new installations.
func seedDefaultJobs() []CronJobConfig {
	return []CronJobConfig{
		{
			ID:           "self-improve",
			Name:         "Self-Improvement",
			Enabled:      true,
			Schedule:     "0 3 */2 * *",
			TZ:           "Asia/Taipei",
			IdleMinHours: 2,
			Task: CronTaskConfig{
				Prompt: `You are a self-improvement agent for the Tetora AI orchestration system.

Analyze the activity digest below. The digest includes existing Skills, Rules, and Memory —
do NOT create anything that already exists.

## Instructions
1. Identify repeated patterns (3+ occurrences), low-score reflections, recurring failures
2. For each actionable improvement, CREATE the file directly:
   - **Rule**: Create ` + "`rules/{name}.md`" + ` — governance rules auto-injected into all agents
   - **Memory**: Create/update ` + "`memory/{key}.md`" + ` — shared observations
   - **Skill**: Create ` + "`skills/{name}/metadata.json`" + ` with ` + "`\"approved\": false`" + ` — requires human review
3. Only apply HIGH and MEDIUM priority improvements
4. Keep files concise and actionable
5. Report what you created and why

If insufficient data for improvements, say so and exit.

---

{{review.digest:7}}`,
				Model:          "sonnet",
				Timeout:        "5m",
				Budget:         1.5,
				PermissionMode: "autoEdit",
			},
			Notify:     true,
			MaxRetries: 1,
			RetryDelay: "2m",
		},
		{
			ID:       "backlog-triage",
			Name:     "Backlog Triage",
			Enabled:  true,
			Schedule: "50 9 * * *",
			TZ:       "Asia/Taipei",
			Task:     CronTaskConfig{},
			Notify:   true,
		},
	}
}

// cronDiscordSendBotChannel sends a message to a Discord channel via bot token.
func cronDiscordSendBotChannel(botToken, channelID, msg string) error {
	if len(msg) > 2000 {
		msg = msg[:1997] + "..."
	}
	payload, err := json.Marshal(map[string]string{"content": msg})
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://discord.com/api/v10/channels/%s/messages", channelID)
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+botToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("Discord returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// --- Startup Replay ---

// startupReplay detects cron jobs that should have run while the daemon was down
// and schedules a single catch-up run (the most recent missed slot only) after a
// 30-second delay to avoid startup conflicts.
//
// A job is considered "missed" when:
//   - Its schedule expression places at least one trigger in [now-replayHours, now]
//   - cron_execution_log has no entry for that job near the computed scheduled time
//   - job_runs also has no entry (backward-compat fallback for existing DBs)
//
// Jobs with RequireApproval, IdleMinHours>0, or the built-in special jobs
// (daily_notes, backlog-triage) are excluded from replay.
func (ce *CronEngine) startupReplay(ctx context.Context) {
	hours := ce.cfg.CronReplayHours
	if hours <= 0 {
		hours = 2
	}
	if ce.cfg.HistoryDB == "" {
		return
	}

	now := time.Now()
	from := now.Add(-time.Duration(hours) * time.Hour)

	ce.mu.RLock()
	jobs := make([]*cronJob, len(ce.jobs))
	copy(jobs, ce.jobs)
	ce.mu.RUnlock()

	var missed []*cronJob
	for _, j := range jobs {
		if !j.Enabled {
			continue
		}
		// Skip jobs that shouldn't run unsupervised.
		if j.IdleMinHours > 0 || j.RequireApproval || j.Trigger == "idle" {
			continue
		}
		// Skip built-in special jobs with custom dispatch logic.
		if j.ID == "daily_notes" || j.ID == "backlog-triage" {
			continue
		}

		scheduledAt := ce.mostRecentScheduledTime(j, from, now)
		if scheduledAt.IsZero() {
			continue // no trigger in window
		}

		// Check cron_execution_log first (authoritative from this version onwards).
		if cronExecLogExists(ce.cfg.HistoryDB, j.ID, scheduledAt) {
			continue // job ran or was a zombie — skip
		}
		// Fallback: check job_runs for DBs upgraded before cron_execution_log existed.
		if jobRunExistsNear(ce.cfg.HistoryDB, j.ID, scheduledAt) {
			continue
		}

		logInfo("cron startup replay: missed run detected",
			"jobId", j.ID, "name", j.Name, "scheduledAt", scheduledAt.Format(time.RFC3339))
		missed = append(missed, j)
	}

	if len(missed) == 0 {
		return
	}

	logInfo("cron startup replay: scheduling missed jobs", "count", len(missed))

	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
		}

		for _, j := range missed {
			select {
			case <-ctx.Done():
				return
			default:
			}

			ce.mu.Lock()
			maxRuns := j.effectiveMaxConcurrentRuns()
			if j.runCount >= maxRuns {
				ce.mu.Unlock()
				logInfo("cron startup replay: job already running max instances, skipping",
					"jobId", j.ID, "running", j.runCount, "maxConcurrentRuns", maxRuns)
				continue
			}
			j.runCount++
			j.running = true
			j.replayed = true
			jobCtx, jobCancel := context.WithCancel(ctx)
			j.cancelFn = jobCancel
			ce.mu.Unlock()

			logInfo("cron startup replay: launching missed job", "jobId", j.ID, "name", j.Name)
			ce.jobWg.Add(1)
			go func(jj *cronJob) {
				defer ce.jobWg.Done()
				ce.runJob(jobCtx, jj)
				ce.mu.Lock()
				jj.replayed = false
				ce.mu.Unlock()
			}(j)
		}
	}()
}

// mostRecentScheduledTime returns the most recent time in [from, to] that the
// job's cron expression would have triggered. Returns zero time if none.
func (ce *CronEngine) mostRecentScheduledTime(j *cronJob, from, to time.Time) time.Time {
	var last time.Time
	// nextRunAfter starts from t+1min, so subtract 1min to include 'from' itself.
	t := from.In(j.loc).Add(-time.Minute)
	for {
		next := nextRunAfter(j.expr, j.loc, t)
		if next.IsZero() || next.After(to) {
			break
		}
		last = next
		t = next // nextRunAfter will advance by 1min internally on next call
	}
	return last
}

// cronDiscordSendWebhook sends a plain message to a Discord webhook URL.
func cronDiscordSendWebhook(webhookURL, msg string) error {
	payload, err := json.Marshal(map[string]string{"content": msg})
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(webhookURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("Discord returned HTTP %d", resp.StatusCode)
	}
	return nil
}
