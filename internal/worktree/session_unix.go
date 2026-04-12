//go:build !windows

package worktree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// isSessionActive returns true when the worktree at wtDir has an active
// session lock whose recorded PID is still running. A missing lock file, a
// zero/invalid PID, or a dead process all return false.
func isSessionActive(wtDir string) bool {
	data, err := os.ReadFile(filepath.Join(wtDir, sessionLockFile))
	if err != nil {
		return false
	}
	var pid int
	if n, _ := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid); n != 1 || pid <= 0 {
		return false
	}
	// syscall.Kill(pid, 0) returns nil only if the process exists and is accessible.
	return syscall.Kill(pid, 0) == nil
}
