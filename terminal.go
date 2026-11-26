package main

import (
	"os/exec"
	"runtime"
	"strings"

	loc "mygit/locales"
)

// openTerminal opens a terminal in the repository's folder, for what the
// app doesn't do (#56): the one set, or the system's. Only for a
// repository on this computer.
func (a *app) openTerminal() {
	args, dir := terminalCommand(runtime.GOOS, strings.TrimSpace(prefs.Terminal), a.repo.Dir)
	go func() {
		c := exec.Command(args[0], args[1:]...)
		c.Dir = dir
		if err := c.Start(); err != nil {
			a.update(func() { a.errMsg = loc.ErrTerminal(err.Error()) })
			return
		}
		c.Wait()
	}()
}

// terminalCommand is what opens a terminal in folder on a system, and the
// folder to run it in: on macOS the app named, Terminal unless set;
// elsewhere the command set, or the system's terminal.
func terminalCommand(goos, set, folder string) (args []string, dir string) {
	switch {
	case goos == "darwin":
		if set == "" {
			set = "Terminal"
		}
		return []string{"open", "-a", set, folder}, ""
	case set != "":
		return strings.Fields(set), folder
	case goos == "windows":
		if _, err := exec.LookPath("wt"); err == nil {
			return []string{"wt", "-d", folder}, ""
		}
		return []string{"powershell", "-NoExit"}, folder
	default:
		return []string{"x-terminal-emulator"}, folder
	}
}
