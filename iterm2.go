package main

import (
	"context"
	"fmt"
)

// sendITerm2Keystroke sends a keystroke to the frontmost iTerm2 session via osascript.
// Supported keys: ArrowUp, ArrowDown, Return, Escape.
func sendITerm2Keystroke(key string) error {
	// Map key names to AppleScript key code expressions.
	// iTerm2's "write text" sends escape sequences for arrow keys;
	// key codes use System Events for non-printable keys.
	var script string
	switch key {
	case "ArrowUp":
		// Send escape sequence for up arrow: ESC [ A
		script = `tell application "iTerm2" to tell current session of current window to write text (character id 27) & "[A" without newline`
	case "ArrowDown":
		// Send escape sequence for down arrow: ESC [ B
		script = `tell application "iTerm2" to tell current session of current window to write text (character id 27) & "[B" without newline`
	case "Return":
		// Send return/enter key
		script = `tell application "iTerm2" to tell current session of current window to write text ""` // write text with newline (default)
	case "Escape":
		// Send ESC character
		script = `tell application "iTerm2" to tell current session of current window to write text (character id 27) without newline`
	default:
		return fmt.Errorf("unsupported iTerm2 key: %q", key)
	}

	_, err := runDeviceCommand(context.Background(), "osascript", "-e", script)
	if err != nil {
		return fmt.Errorf("iTerm2 keystroke %q failed: %w", key, err)
	}
	return nil
}
