package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func cmdHealth(args []string) {
	jsonOutput := false
	for _, a := range args {
		if a == "--json" {
			jsonOutput = true
		}
	}

	cfg := loadConfig(findConfigPath())
	api := newAPIClient(cfg)
	api.client.Timeout = 3 * time.Second

	if jsonOutput {
		cmdHealthJSON(cfg, api)
		return
	}

	fmt.Println("=== Tetora Health ===")
	fmt.Println()

	// 1. Daemon.
	daemonOK := false
	var healthData map[string]any
	resp, err := api.get("/healthz")
	if err != nil {
		printHealth("❌", "Daemon", fmt.Sprintf("offline (%s)", cfg.ListenAddr))
	} else {
		defer resp.Body.Close()
		json.NewDecoder(resp.Body).Decode(&healthData)
		daemonOK = true

		status := "healthy"
		if s, ok := healthData["status"].(string); ok {
			status = s
		}
		uptime := ""
		if ut, ok := healthData["uptime"].(map[string]any); ok {
			if d, ok := ut["duration"].(string); ok {
				uptime = d
			}
		}
		icon := "✅"
		if status == "degraded" {
			icon = "⚠️"
		} else if status == "unhealthy" {
			icon = "❌"
		}
		printHealth(icon, "Daemon", fmt.Sprintf("%s, up %s (%s)", status, uptime, cfg.ListenAddr))
	}

	// 2. Workers.
	if daemonOK {
		resp2, err := api.get("/api/workers")
		if err == nil {
			defer resp2.Body.Close()
			var wData map[string]any
			json.NewDecoder(resp2.Body).Decode(&wData)
			count := int(jsonFloatSafe(wData["count"]))
			if count == 0 {
				printHealth("✅", "Workers", "0 active")
			} else {
				// Show active workers.
				detail := fmt.Sprintf("%d active", count)
				if workers, ok := wData["workers"].([]any); ok {
					var names []string
					for _, w := range workers {
						if wm, ok := w.(map[string]any); ok {
							name := jsonStrSafe(wm["name"])
							agent := jsonStrSafe(wm["agent"])
							if agent != "" {
								name = agent + ":" + name
							}
							names = append(names, name)
						}
					}
					if len(names) > 0 {
						detail += " (" + joinStrings(names, ", ") + ")"
					}
				}
				printHealth("🔵", "Workers", detail)
			}
		}
	}

	// 3. Cron jobs — check for staleness.
	if daemonOK {
		resp3, err := api.get("/cron")
		if err == nil {
			defer resp3.Body.Close()
			var jobs []CronJobInfo
			if json.NewDecoder(resp3.Body).Decode(&jobs) == nil {
				enabled := 0
				failing := 0
				for _, j := range jobs {
					if j.Enabled {
						enabled++
					}
					if j.Errors > 0 {
						failing++
					}
				}
				icon := "✅"
				detail := fmt.Sprintf("%d enabled / %d total", enabled, len(jobs))
				if failing > 0 {
					icon = "⚠️"
					detail += fmt.Sprintf(", %d failing", failing)
				}
				printHealth(icon, "Cron", detail)
			}
		}
	}

	// 4. Taskboard — stale/stuck tasks.
	if cfg.HistoryDB != "" {
		stats, err := getTaskStats(cfg.HistoryDB)
		if err == nil {
			icon := "✅"
			detail := fmt.Sprintf("%d todo, %d doing, %d review, %d done, %d failed",
				stats.Todo, stats.Running, stats.Review, stats.Done, stats.Failed)
			if stats.Running > 0 {
				// Check for stuck doing tasks.
				stuckCount := countStuckDoing(cfg.HistoryDB, 2*time.Hour)
				if stuckCount > 0 {
					icon = "⚠️"
					detail += fmt.Sprintf(" (%d stuck >2h)", stuckCount)
				}
			}
			printHealth(icon, "Taskboard", detail)
		}
	}

	// 5. Disk.
	if daemonOK && healthData != nil {
		if disk, ok := healthData["disk"].(map[string]any); ok {
			freeGB := jsonFloatSafe(disk["freeGB"])
			icon := "✅"
			if freeGB < 5 {
				icon = "⚠️"
			}
			if freeGB < 1 {
				icon = "❌"
			}
			printHealth(icon, "Disk", fmt.Sprintf("%.1f GB free", freeGB))
		}
	}

	// 6. Providers (circuit breakers).
	if daemonOK && healthData != nil {
		if providers, ok := healthData["providers"].(map[string]any); ok {
			for name, pv := range providers {
				pm, ok := pv.(map[string]any)
				if !ok {
					continue
				}
				status := jsonStrSafe(pm["status"])
				circuit := jsonStrSafe(pm["circuit"])
				icon := "✅"
				detail := status
				if circuit == "open" {
					icon = "❌"
					detail += " (circuit OPEN)"
				} else if circuit == "half-open" {
					icon = "⚠️"
					detail += " (circuit half-open)"
				}
				printHealth(icon, "Provider/"+name, detail)
			}
		}
	}

	// 7. Heartbeat.
	if daemonOK && healthData != nil {
		if hb, ok := healthData["heartbeat"].(map[string]any); ok {
			if enabled, ok := hb["enabled"].(bool); ok && enabled {
				stalled := int(jsonFloatSafe(hb["stalledNow"]))
				icon := "✅"
				detail := "monitoring"
				if stalled > 0 {
					icon = "⚠️"
					detail = fmt.Sprintf("%d stalled sessions", stalled)
				}
				printHealth(icon, "Heartbeat", detail)
			}
		}
	}

	// 8. Git worktrees (local check, no daemon needed).
	if wtOut, err := exec.Command("git", "worktree", "list", "--porcelain").CombinedOutput(); err == nil {
		count := 0
		for _, line := range strings.Split(string(wtOut), "\n") {
			if strings.HasPrefix(line, "worktree ") {
				count++
			}
		}
		icon := "✅"
		detail := fmt.Sprintf("%d worktree(s)", count)
		if count > 3 {
			icon = "⚠️"
			detail += " — consider cleanup"
		}
		printHealth(icon, "Worktrees", detail)
	}

	fmt.Println()
}

func printHealth(icon, label, detail string) {
	fmt.Printf("  %s %-16s %s\n", icon, label, detail)
}

// countStuckDoing returns the number of tasks in "doing" status older than threshold.
func countStuckDoing(dbPath string, threshold time.Duration) int {
	cutoff := time.Now().Add(-threshold).UTC().Format(time.RFC3339)
	sql := fmt.Sprintf(`SELECT COUNT(*) as cnt FROM tasks WHERE status = 'doing' AND updated_at < '%s'`,
		escapeSQLite(cutoff))
	rows, err := queryDB(dbPath, sql)
	if err != nil || len(rows) == 0 {
		return 0
	}
	return int(jsonFloatSafe(rows[0]["cnt"]))
}

func cmdHealthJSON(cfg *Config, api *apiClient) {
	result := map[string]any{
		"version": tetoraVersion,
	}

	// Daemon + deep health.
	resp, err := api.get("/healthz")
	if err != nil {
		result["daemon"] = "offline"
	} else {
		defer resp.Body.Close()
		var health map[string]any
		json.NewDecoder(resp.Body).Decode(&health)
		result["daemon"] = "running"
		result["health"] = health
	}

	// Workers.
	if result["daemon"] == "running" {
		resp2, err := api.get("/api/workers")
		if err == nil {
			defer resp2.Body.Close()
			var wData map[string]any
			json.NewDecoder(resp2.Body).Decode(&wData)
			result["workers"] = wData
		}
	}

	// Taskboard.
	if cfg.HistoryDB != "" {
		stats, err := getTaskStats(cfg.HistoryDB)
		if err == nil {
			stuckCount := countStuckDoing(cfg.HistoryDB, 2*time.Hour)
			result["taskboard"] = map[string]any{
				"stats": stats,
				"stuck": stuckCount,
			}
		}
	}

	// Worktrees.
	if wtOut, err := exec.Command("git", "worktree", "list", "--porcelain").CombinedOutput(); err == nil {
		count := 0
		for _, line := range strings.Split(string(wtOut), "\n") {
			if strings.HasPrefix(line, "worktree ") {
				count++
			}
		}
		result["worktrees"] = count
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(result)
}
