package main

import (
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	loc "mygit/locales"
)

// copyUnixPathItem offers, on Windows, to copy a local path the way Git
// Bash writes it (#58).
func (a *app) copyUnixPathItem(m *ui.Menu, p string) {
	if runtime.GOOS == "windows" && a.local() && m.Item(loc.CopyUnixPath()).Chosen() {
		mygo.Clipboard.WriteText(unixPath(p))
	}
}

// unixPath is a Windows path as cygpath -u writes it: C:\Users\foo is
// /c/Users/foo, \\server\share is //server/share.
func unixPath(p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	if len(p) >= 2 && p[1] == ':' {
		return "/" + strings.ToLower(p[:1]) + p[2:]
	}
	return p
}
