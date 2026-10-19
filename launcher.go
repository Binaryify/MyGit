package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"

	loc "mygit/locales"
)

// launcherMark is in every mygit command MyGit writes, so that it removes
// its own and leaves another program's of the name alone.
const launcherMark = "Installed by MyGit"

// installLauncher puts a mygit command on the PATH, as VS Code's "Install
// 'code' command in PATH": `mygit .` in a terminal then opens the
// repository there in the running app. It shows dialogs, which wait:
// call it on a goroutine.
func installLauncher() {
	exe, err := appExecutable()
	if err != nil {
		launcherMessage(loc.LauncherNoExe(), err.Error())
		return
	}
	path, msg, err := writeLauncher(exe)
	if err != nil {
		launcherMessage(loc.LauncherFailed(), err.Error())
		return
	}
	if msg == "" {
		msg = loc.LauncherDoneMessage(path)
	}
	launcherMessage(loc.LauncherDone(), msg)
}

// uninstallLauncher removes the mygit commands MyGit put on the PATH.
func uninstallLauncher() {
	removed, err := removeLaunchers()
	switch {
	case err != nil:
		launcherMessage(loc.LauncherUninstallFailed(), err.Error())
	case len(removed) == 0:
		launcherMessage(loc.LauncherUninstalled(), loc.LauncherNotFound())
	default:
		launcherMessage(loc.LauncherUninstalled(), loc.LauncherRemoved(strings.Join(removed, "\n")))
	}
}

// appExecutable is the app's executable, its links followed: what the
// command runs.
func appExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// ours tells whether the file at path is a mygit command MyGit wrote:
// marked, or one of before the mark, which ran MyGit's executable.
func ours(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil || len(b) > 4096 {
		return false
	}
	s := string(b)
	return strings.Contains(s, launcherMark) ||
		strings.HasPrefix(s, "#!/bin/sh\n") && strings.Contains(s, "MyGit") && strings.Contains(s, `"$dir"`)
}

func launcherMessage(title, msg string) {
	_, _ = mygo.Dialog.Message(mygo.MessageOptions{Title: title, Message: msg})
}
