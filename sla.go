package main

// sla.go is a thin facade wrapping internal/sla.
// Business logic lives in internal/sla/; this file bridges globals and *Config.

import (
	"context"
	"time"

	"tetora/internal/sla"
)

// --- Type aliases ---

type SLAConfig = sla.SLAConfig
type AgentSLACfg = sla.AgentSLACfg
type SLAMetrics = sla.SLAMetrics
type SLAStatus = sla.SLAStatus
type SLACheckResult = sla.SLACheckResult

// --- slaChecker facade ---

// slaChecker wraps sla.Checker, bridging *Config.
type slaChecker struct {
	cfg      *Config
	inner    *sla.Checker
	lastRun  time.Time
}

func newSLAChecker(cfg *Config, notifyFn func(string)) *slaChecker {
	return &slaChecker{
		cfg:   cfg,
		inner: sla.NewChecker(cfg.HistoryDB, cfg.SLA, notifyFn),
	}
}

func (s *slaChecker) tick(ctx context.Context) {
	if !s.cfg.SLA.Enabled {
		return
	}
	s.inner.Tick(ctx)
	s.lastRun = s.inner.LastRun()
}

// --- Forwarding functions ---

func initSLADB(dbPath string) {
	sla.InitSLADB(dbPath)
}

func querySLAMetrics(dbPath, agent string, windowHours int) (*SLAMetrics, error) {
	return sla.QuerySLAMetrics(dbPath, agent, windowHours)
}

func queryP95Latency(dbPath, agent string, windowHours int) int64 {
	return sla.QueryP95Latency(dbPath, agent, windowHours)
}

func querySLAAll(dbPath string, agents []string, windowHours int) ([]SLAMetrics, error) {
	return sla.QuerySLAAll(dbPath, agents, windowHours)
}

func querySLAStatusAll(c *Config) ([]SLAStatus, error) {
	window := c.SLA.WindowOrDefault()
	windowHours := int(window.Hours())
	if windowHours <= 0 {
		windowHours = 24
	}

	// Collect all agent names.
	names := make([]string, 0, len(c.Agents))
	for name := range c.Agents {
		names = append(names, name)
	}

	return sla.QuerySLAStatusAll(c.HistoryDB, c.SLA.Agents, names, windowHours)
}

func checkSLAViolations(c *Config, notifyFn func(string)) {
	if !c.SLA.Enabled || c.HistoryDB == "" {
		return
	}
	window := c.SLA.WindowOrDefault()
	windowHours := int(window.Hours())
	if windowHours <= 0 {
		windowHours = 24
	}
	sla.CheckSLAViolations(c.HistoryDB, c.SLA.Agents, windowHours, notifyFn)
}

func recordSLACheck(dbPath string, r SLACheckResult) {
	sla.RecordSLACheck(dbPath, r)
}

func querySLAHistory(dbPath, agent string, limit int) ([]SLACheckResult, error) {
	return sla.QuerySLAHistory(dbPath, agent, limit)
}
