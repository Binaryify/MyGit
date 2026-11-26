package main

import (
	"slices"
	"testing"
)

// TestTerminalCommand checks what opens a terminal in a repository's
// folder on each system (#56).
func TestTerminalCommand(t *testing.T) {
	for _, tc := range []struct {
		goos, set string
		args      []string
		dir       string
	}{
		{"darwin", "", []string{"open", "-a", "Terminal", "/r"}, ""},
		{"darwin", "Ghostty", []string{"open", "-a", "Ghostty", "/r"}, ""},
		{"linux", "", []string{"x-terminal-emulator"}, "/r"},
		{"linux", "kitty --single-instance", []string{"kitty", "--single-instance"}, "/r"},
		{"windows", "alacritty", []string{"alacritty"}, "/r"},
	} {
		args, dir := terminalCommand(tc.goos, tc.set, "/r")
		if !slices.Equal(args, tc.args) || dir != tc.dir {
			t.Errorf("%s %q: %q in %q, want %q in %q", tc.goos, tc.set, args, dir, tc.args, tc.dir)
		}
	}
}
