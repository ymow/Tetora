//go:build unix

package provider

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"tetora/internal/log"
)

// StartRSSGuard polls the RSS of the given pid every interval.
// When RSS exceeds maxMB, it calls onBreach(rssMB) then cancel().
// Returns a stop func that is safe to call multiple times.
// pid=0 or maxMB<=0 → returns a no-op (guard is disabled).
func StartRSSGuard(pid int, maxMB int, interval time.Duration, cancel context.CancelFunc, onBreach func(rssMB int)) (stop func()) {
	noop := func() {}
	if pid == 0 || maxMB <= 0 {
		return noop
	}

	done := make(chan struct{})
	var once sync.Once
	stopFn := func() {
		once.Do(func() { close(done) })
	}

	// Compare in KB internally to avoid integer-division precision loss
	// (a 500KB process would round to 0MB and slip past tight limits).
	maxKB := maxMB * 1024

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				rssKB, ok := readRSSKB(pid)
				if !ok {
					// Process has already exited — stop guard silently.
					return
				}
				if rssKB > maxKB {
					rssMB := (rssKB + 1023) / 1024 // ceil for observability
					log.Warn("rss_guard: process exceeded memory limit, cancelling",
						"pid", pid, "limitMB", maxMB, "rssMB", rssMB)
					if onBreach != nil {
						onBreach(rssMB)
					}
					cancel()
					return
				}
			}
		}
	}()

	return stopFn
}

// readRSSKB returns (rssKB, true) on success, or (0, false) if the process
// no longer exists or the value cannot be parsed.
func readRSSKB(pid int) (int, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "ps", "-o", "rss=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		// Process gone or ps error — treat as clean exit.
		return 0, false
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return 0, false
	}
	kb, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return kb, true
}
