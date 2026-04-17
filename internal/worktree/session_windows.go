//go:build windows

package worktree

// isSessionActive is not supported on Windows; always returns false so
// worktrees are never considered locked by a live session on this platform.
func isSessionActive(_ string) bool {
	return false
}
