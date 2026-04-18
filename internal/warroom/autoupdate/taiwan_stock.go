package autoupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"tetora/internal/config"
	"tetora/internal/log"
	"tetora/internal/warroom"
)

func updateTaiwanStockAuto(ctx context.Context, cfg *config.Config, front json.RawMessage) (map[string]any, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}

	dbPath := filepath.Join(home, "Workspace/Projects/01-Personal/stock-trading/data/trading.db")

	// Stat DB file.
	fi, err := os.Stat(dbPath)
	if err != nil || fi.Size() == 0 {
		log.Warn("taiwan-stock autoupdate: trading.db missing or empty", "path", dbPath)
		return nil, nil
	}

	// Query last trade timestamp.
	var lastTrade time.Time
	var paperDays int
	var lastTradeStr string

	out, execErr := exec.CommandContext(ctx, "sqlite3", dbPath, "SELECT MAX(timestamp) FROM trades;").Output()
	if execErr != nil {
		log.Warn("taiwan-stock autoupdate: sqlite3 query failed", "err", execErr)
	} else {
		s := strings.TrimSpace(string(out))
		if s != "" {
			t, err := time.Parse(time.RFC3339, s)
			if err != nil {
				t, err = time.Parse("2006-01-02 15:04:05", s)
			}
			if err == nil {
				lastTrade = t
				paperDays = int(time.Since(lastTrade).Hours() / 24)
				lastTradeStr = lastTrade.Format("2006-01-02")
			}
		}
	}

	// Shioaji log connection status.
	shioajiLog := filepath.Join(cfg.BaseDir, "workspace/shioaji.log")
	connectionStatus := "unknown"
	if lfi, err := os.Stat(shioajiLog); err == nil {
		if time.Since(lfi.ModTime()) < 24*time.Hour {
			connectionStatus = "connected"
		} else {
			connectionStatus = "down"
		}
	}

	// Read existing metrics and merge.
	var existingMetrics map[string]any
	if err := warroom.FrontField(front, "metrics", &existingMetrics); err != nil {
		existingMetrics = map[string]any{}
	}
	if existingMetrics == nil {
		existingMetrics = map[string]any{}
	}

	if !lastTrade.IsZero() {
		existingMetrics["paper_days"] = paperDays
	}
	existingMetrics["connection_status"] = connectionStatus

	// Compute status.
	var statusStr string
	switch {
	case connectionStatus == "down" || paperDays > 7:
		statusStr = "red"
	case paperDays > 1:
		statusStr = "yellow"
	default:
		statusStr = "green"
	}

	// Build summary.
	var summaryParts []string
	if !lastTrade.IsZero() {
		summaryParts = append(summaryParts, fmt.Sprintf("紙上天數 %d", paperDays))
	}
	summaryParts = append(summaryParts, fmt.Sprintf("連線 %s", connectionStatus))
	if lastTradeStr != "" {
		summaryParts = append(summaryParts, fmt.Sprintf("最後交易 %s", lastTradeStr))
	}
	summary := "[auto] " + strings.Join(summaryParts, "；")

	updates := map[string]any{
		"metrics":      existingMetrics,
		"summary":      summary,
		"status":       statusStr,
		"last_updated": time.Now().UTC().Format(time.RFC3339),
	}
	return updates, nil
}
