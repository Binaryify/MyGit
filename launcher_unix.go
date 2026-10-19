//go:build !windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	loc "mygit/locales"
)

// launcherScript is the mygit command: it makes the folder absolute, as
// the running app has another working folder, and opens it in the app
// without holding the terminal.
func launcherScript(exe string) string {
	return fmt.Sprintf("#!/bin/sh\n# %s: `mygit [folder]` opens the folder's repository.\ndir=$(cd \"${1:-.}\" && pwd) || exit 1\n%s \"$dir\" >/dev/null 2>&1 &\n",
		launcherMark, shellWord(exe))
}

func shellWord(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// shellPATH is the PATH of the user's terminals: an app started from the
// Finder or the Dock has the system's alone, so the login shell is asked,
// as VS Code does. The app's own comes after it.
var shellPATH = func() []string {
	dirs := []string{}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
		if runtime.GOOS == "darwin" {
			shell = "/bin/zsh"
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Between markers, as a shell's startup files may print too.
	out, err := exec.CommandContext(ctx, shell, "-ilc", `printf '\n@@%s@@\n' "$PATH"`).Output()
	if err == nil {
		if _, rest, ok := strings.Cut(string(out), "\n@@"); ok {
			if p, _, ok := strings.Cut(rest, "@@\n"); ok {
				dirs = filepath.SplitList(p)
			}
		}
	}
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		if !contains(dirs, d) {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// launcherDir is the folder of the terminals' PATH the command goes in:
// the usual ones for one's commands first (~/.local/bin, ~/bin, then
// Homebrew's and /usr/local/bin), then another of the user's own, then
// any, of those the user can write to; never a tool's folder of the
// moment, as fnm's of each shell.
func launcherDir(dirs []string) (string, bool) {
	home, _ := os.UserHomeDir()
	usual := []string{filepath.Join(home, ".local", "bin"), filepath.Join(home, "bin"), "/opt/homebrew/bin", "/usr/local/bin"}
	rank := func(d string) int {
		if i := slices.Index(usual, filepath.Clean(d)); i >= 0 {
			return i
		}
		if strings.HasPrefix(d, home+string(filepath.Separator)) {
			return len(usual)
		}
		return len(usual) + 1
	}
	var ok []string
	for _, d := range dirs {
		if d == "" || strings.Contains(d, "multishells") || slices.Contains(ok, d) || !writable(d) {
			continue
		}
		ok = append(ok, d)
	}
	slices.SortStableFunc(ok, func(a, b string) int { return rank(a) - rank(b) })
	if len(ok) == 0 {
		return "", false
	}
	return ok[0], true
}

func writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".mygit-*")
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(f.Name())
	return true
}

// writeLauncher writes the command into a folder of the PATH. With none
// to write to, it goes to /usr/local/bin with an administrator's password
// on macOS, as VS Code's does, and to ~/.local/bin elsewhere, saying to add
// it to the PATH.
func writeLauncher(exe string) (path, msg string, err error) {
	script := launcherScript(exe)
	dirs := shellPATH()
	if dir, ok := launcherDir(dirs); ok {
		path = filepath.Join(dir, "mygit")
		if _, err := os.Stat(path); err == nil && !ours(path) {
			return "", "", errors.New(loc.LauncherTaken(path))
		}
		return path, "", os.WriteFile(path, []byte(script), 0o755)
	}
	if runtime.GOOS == "darwin" {
		path = "/usr/local/bin/mygit"
		if _, err := os.Stat(path); err == nil && !ours(path) {
			return "", "", errors.New(loc.LauncherTaken(path))
		}
		tmp, err := os.CreateTemp("", "mygit-*")
		if err != nil {
			return "", "", err
		}
		defer os.Remove(tmp.Name())
		tmp.WriteString(script)
		tmp.Close()
		return path, "", asAdmin(fmt.Sprintf("mkdir -p /usr/local/bin && cp %s %s && chmod 755 %s", shellWord(tmp.Name()), path, path))
	}
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	path = filepath.Join(dir, "mygit")
	return path, loc.LauncherAddToPath(path, dir), os.WriteFile(path, []byte(script), 0o755)
}

// removeLaunchers removes the mygit commands MyGit wrote in the folders
// of the PATH, as an administrator on macOS where the user can't.
func removeLaunchers() ([]string, error) {
	var removed []string
	for _, d := range shellPATH() {
		p := filepath.Join(d, "mygit")
		if d == "" || contains(removed, p) || !ours(p) {
			continue
		}
		err := os.Remove(p)
		if errors.Is(err, os.ErrPermission) && runtime.GOOS == "darwin" {
			err = asAdmin("rm -f " + shellWord(p))
		}
		if err != nil {
			return removed, err
		}
		removed = append(removed, p)
	}
	return removed, nil
}

// asAdmin runs a shell command as an administrator, macOS asking for the
// password.
func asAdmin(cmd string) error {
	script := fmt.Sprintf("do shell script %q with administrator privileges", cmd)
	if out, err := exec.Command("osascript", "-e", script).CombinedOutput(); err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return errors.New(msg)
		}
		return err
	}
	return nil
}
