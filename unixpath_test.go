package main

import "testing"

// TestUnixPath checks a Windows path is copied as Git Bash writes it (#58).
func TestUnixPath(t *testing.T) {
	for in, want := range map[string]string{
		`C:\Users\foo`:     "/c/Users/foo",
		`D:/repo/src/a.go`: "/d/repo/src/a.go",
		`C:\`:              "/c/",
		`\\server\share\x`: "//server/share/x",
		"/home/foo":        "/home/foo",
	} {
		if got := unixPath(in); got != want {
			t.Errorf("unixPath(%q) = %q, want %q", in, got, want)
		}
	}
}
