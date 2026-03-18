package main

import dtypes "tetora/internal/dispatch"

// Type aliases — callers in package main use these names directly.
type SlotPressureGuard = dtypes.SlotPressureGuard
type AcquireResult = dtypes.AcquireResult

// isInteractiveSource delegates to the canonical implementation in internal/dispatch.
func isInteractiveSource(source string) bool {
	return dtypes.IsInteractiveSource(source)
}
