package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/egoist/mygo"

	loc "mygit/locales"
)

// installLauncher writes a mygit command to a folder on the PATH, so that
// `mygit .` in a terminal opens the repository there in the running app.
// It shows dialogs, which wait: call it on a goroutine.
func installLauncher() {
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		launcherMessage(loc.LauncherNoExe(), err.Error())
		return
	}
	if runtime.GOOS == "windows" {
		launcherMessage(loc.LauncherTitle(), loc.LauncherWindows(filepath.Dir(exe)))
		return
	}
	dir, err := launcherDir()
	if err != nil {
		launcherMessage(loc.LauncherNoDir(), loc.LauncherLink(exe))
		return
	}
	path := filepath.Join(dir, "mygit")
	// The repository's path is made absolute here, as the running app has
	// another working folder.
	script := fmt.Sprintf("#!/bin/sh\ndir=$(cd \"${1:-.}\" && pwd) || exit 1\n%s \"$dir\" >/dev/null 2>&1 &\n", shellWord(exe))
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		launcherMessage(loc.LauncherFailed(), err.Error())
		return
	}
	launcherMessage(loc.LauncherDone(), loc.LauncherDoneMessage(path))
}

// launcherDir is the first folder of the PATH the user can write to,
// preferring their own over the system's.
func launcherDir() (string, error) {
	home, _ := os.UserHomeDir()
	dirs := filepath.SplitList(os.Getenv("PATH"))
	for _, own := range []bool{true, false} {
		for _, d := range dirs {
			if strings.HasPrefix(d, home+string(filepath.Separator)) != own {
				continue
			}
			if f, err := os.CreateTemp(d, ".mygit-*"); err == nil {
				f.Close()
				os.Remove(f.Name())
				return d, nil
			}
		}
	}
	return "", os.ErrPermission
}

func shellWord(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func launcherMessage(title, msg string) {
	_, _ = mygo.Dialog.Message(mygo.MessageOptions{Title: title, Message: msg})
}
