package messaging

import "tetora/internal/text"

// TruncateStr truncates a string to maxLen runes, appending "..." if truncated.
// Delegates to the canonical rune-aware implementation in internal/text.
func TruncateStr(s string, maxLen int) string {
	return text.TruncateStr(s, maxLen)
}
