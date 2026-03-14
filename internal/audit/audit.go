// Package audit provides batched, non-blocking audit logging backed by SQLite.
package audit

import (
	"fmt"
	"strings"
	"time"

	"tetora/internal/db"
	tlog "tetora/internal/log"
)

// Entry represents a row in the audit_log table.
type Entry struct {
	ID        int    `json:"id"`
	Timestamp string `json:"timestamp"`
	Action    string `json:"action"`
	Source    string `json:"source"`
	Detail    string `json:"detail"`
	IP        string `json:"ip"`
}

// RoutingHistoryEntry represents a parsed route.dispatch audit log entry.
type RoutingHistoryEntry struct {
	ID         int    `json:"id"`
	Timestamp  string `json:"timestamp"`
	Source     string `json:"source"`
	Agent      string `json:"agent"`
	Method     string `json:"method"`
	Confidence string `json:"confidence"`
	Prompt     string `json:"prompt"`
}

// AgentRoutingStats aggregates routing stats for a single agent.
type AgentRoutingStats struct {
	Total int `json:"total"`
}

// entry holds a single audit log item for the batched writer.
type entry struct {
	dbPath string
	ts     string
	action string
	source string
	detail string
	ip     string
}

// Chan is a buffered channel for non-blocking audit log writes.
// The single writer goroutine drains this channel and batches inserts into
// one sqlite3 call, eliminating "database is locked" errors from concurrent
// fire-and-forget goroutines.
var Chan = make(chan entry, 256)

// StartWriter starts the background goroutine that drains Chan and writes
// entries in batches. Call once at startup.
func StartWriter() {
	go writer()
}

func writer() {
	// Batch window: collect entries for up to 500ms or 50 entries, whichever comes first.
	const maxBatch = 50
	const flushInterval = 500 * time.Millisecond

	buf := make([]entry, 0, maxBatch)
	timer := time.NewTimer(flushInterval)
	defer timer.Stop()

	for {
		select {
		case e, ok := <-Chan:
			if !ok {
				// Channel closed — flush remaining and exit.
				if len(buf) > 0 {
					flush(buf)
				}
				return
			}
			buf = append(buf, e)
			if len(buf) >= maxBatch {
				flush(buf)
				buf = buf[:0]
				timer.Reset(flushInterval)
			}
		case <-timer.C:
			if len(buf) > 0 {
				flush(buf)
				buf = buf[:0]
			}
			timer.Reset(flushInterval)
		}
	}
}

// flush writes a batch of audit entries in a single sqlite3 call.
func flush(entries []entry) {
	if len(entries) == 0 {
		return
	}
	// Group by dbPath (almost always the same, but be safe).
	byDB := make(map[string][]entry)
	for _, e := range entries {
		byDB[e.dbPath] = append(byDB[e.dbPath], e)
	}
	for dbPath, batch := range byDB {
		var stmts []string
		for _, e := range batch {
			stmts = append(stmts, fmt.Sprintf(
				`INSERT INTO audit_log (timestamp, action, source, detail, ip) VALUES ('%s','%s','%s','%s','%s')`,
				e.ts, e.action, e.source, e.detail, e.ip,
			))
		}
		sql := strings.Join(stmts, ";\n")
		if err := db.Exec(dbPath, sql); err != nil {
			tlog.Error("audit log batch insert failed", "count", len(batch), "error", err)
		}
	}
}

// Init creates the audit_log table and indexes if they do not exist.
func Init(dbPath string) error {
	db.Pragma(dbPath)
	sql := `CREATE TABLE IF NOT EXISTS audit_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  timestamp TEXT NOT NULL,
  action TEXT NOT NULL,
  source TEXT NOT NULL DEFAULT '',
  detail TEXT DEFAULT '',
  ip TEXT DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_audit_log_ts ON audit_log(timestamp);
CREATE INDEX IF NOT EXISTS idx_audit_log_action ON audit_log(action);`

	return db.Exec(dbPath, sql)
}

// Log records an action to the audit_log table.
// Non-blocking: entries are queued to the batched writer.
func Log(dbPath, action, source, detail, ip string) {
	if dbPath == "" {
		return
	}
	select {
	case Chan <- entry{
		dbPath: dbPath,
		ts:     db.Escape(time.Now().UTC().Format(time.RFC3339)),
		action: db.Escape(action),
		source: db.Escape(source),
		detail: db.Escape(db.Truncate(detail, 500)),
		ip:     db.Escape(ip),
	}:
	default:
		// Channel full — drop entry rather than block the caller.
		tlog.Warn("audit log queue full, dropping entry", "action", action)
	}
}

// Query returns recent audit log entries with a total count.
func Query(dbPath string, limit, offset int) ([]Entry, int, error) {
	if limit <= 0 {
		limit = 50
	}

	// Count total.
	countSQL := "SELECT COUNT(*) as cnt FROM audit_log"
	countRows, err := db.Query(dbPath, countSQL)
	if err != nil {
		return nil, 0, err
	}
	total := 0
	if len(countRows) > 0 {
		total = db.Int(countRows[0]["cnt"])
	}

	// Query entries.
	sql := fmt.Sprintf(
		`SELECT id, timestamp, action, source, detail, ip
		 FROM audit_log ORDER BY id DESC LIMIT %d OFFSET %d`,
		limit, offset)
	rows, err := db.Query(dbPath, sql)
	if err != nil {
		return nil, 0, err
	}

	var entries []Entry
	for _, row := range rows {
		entries = append(entries, Entry{
			ID:        db.Int(row["id"]),
			Timestamp: db.Str(row["timestamp"]),
			Action:    db.Str(row["action"]),
			Source:    db.Str(row["source"]),
			Detail:    db.Str(row["detail"]),
			IP:        db.Str(row["ip"]),
		})
	}
	return entries, total, nil
}

// ParseRouteDetail extracts agent, method, confidence, and prompt from the detail field.
// Format: "role=X method=Y confidence=Z prompt=..."
func ParseRouteDetail(detail string) (role, method, confidence, prompt string) {
	// The prompt field may contain spaces, so split on it first.
	parts := strings.SplitN(detail, " prompt=", 2)
	if len(parts) == 2 {
		prompt = parts[1]
	}

	kvPart := parts[0]
	for _, token := range strings.Fields(kvPart) {
		if strings.HasPrefix(token, "role=") {
			role = strings.TrimPrefix(token, "role=")
		} else if strings.HasPrefix(token, "method=") {
			method = strings.TrimPrefix(token, "method=")
		} else if strings.HasPrefix(token, "confidence=") {
			confidence = strings.TrimPrefix(token, "confidence=")
		}
	}
	return
}

// QueryRoutingStats queries audit_log for route.dispatch events and returns
// a list of routing history entries and per-agent stats.
func QueryRoutingStats(dbPath string, limit int) ([]RoutingHistoryEntry, map[string]*AgentRoutingStats, error) {
	if limit <= 0 {
		limit = 50
	}

	sql := fmt.Sprintf(
		`SELECT id, timestamp, source, detail
		 FROM audit_log WHERE action='route.dispatch'
		 ORDER BY id DESC LIMIT %d`,
		limit)
	rows, err := db.Query(dbPath, sql)
	if err != nil {
		return nil, nil, err
	}

	var history []RoutingHistoryEntry
	byRole := make(map[string]*AgentRoutingStats)

	for _, row := range rows {
		detail := db.Str(row["detail"])
		role, method, confidence, prompt := ParseRouteDetail(detail)

		history = append(history, RoutingHistoryEntry{
			ID:         db.Int(row["id"]),
			Timestamp:  db.Str(row["timestamp"]),
			Source:     db.Str(row["source"]),
			Agent:      role,
			Method:     method,
			Confidence: confidence,
			Prompt:     prompt,
		})

		if role != "" {
			stats, ok := byRole[role]
			if !ok {
				stats = &AgentRoutingStats{}
				byRole[role] = stats
			}
			stats.Total++
		}
	}

	return history, byRole, nil
}

// Cleanup removes audit log entries older than the given number of days.
func Cleanup(dbPath string, days int) error {
	sql := fmt.Sprintf(
		`DELETE FROM audit_log WHERE datetime(timestamp) < datetime('now','-%d days')`, days)
	return db.Exec(dbPath, sql)
}
