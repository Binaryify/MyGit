package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/egoist/mygo"
)

// openInEditor opens a file, at a line from 1 when given, or a folder in
// the editor set, or the system's app for it.
func (a *app) openInEditor(path string, line int) {
	cmd := strings.TrimSpace(prefs.Editor)
	if cmd == "" {
		if err := mygo.Shell.OpenPath(path); err != nil {
			a.errMsg = "打不开：" + err.Error()
		}
		return
	}
	go func() {
		err := runEditor(cmd, path, line)
		if err != nil {
			a.update(func() { a.errMsg = "打开编辑器失败：" + err.Error() })
		}
	}()
}

// runEditor starts an editor's command with a file, saying the line the
// way the editors that take one want it.
func runEditor(cmd, path string, line int) error {
	exe, err := lookEditor(cmd)
	if err != nil {
		return err
	}
	args := []string{path}
	if line > 0 {
		at := path + ":" + strconv.Itoa(line)
		switch strings.TrimSuffix(filepath.Base(cmd), ".exe") {
		case "code", "cursor", "codium", "windsurf":
			args = []string{"-g", at}
		case "zed", "subl":
			args = []string{at}
		case "idea":
			args = []string{"--line", strconv.Itoa(line), path}
		}
	}
	c := exec.Command(exe, args...)
	if err := c.Start(); err != nil {
		return err
	}
	go c.Wait()
	return nil
}

var (
	loginPath     string
	loginPathOnce sync.Once
)

// lookEditor finds an editor's command: on the PATH, or on the login
// shell's, which an app opened from Finder doesn't have.
func lookEditor(cmd string) (string, error) {
	if exe, err := exec.LookPath(cmd); err == nil {
		return exe, nil
	}
	if runtime.GOOS == "windows" || filepath.IsAbs(cmd) {
		return "", errors.New("找不到 " + cmd)
	}
	loginPathOnce.Do(func() {
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/zsh"
		}
		out, _ := exec.Command(shell, "-lc", "echo $PATH").Output()
		loginPath = strings.TrimSpace(string(out))
	})
	for _, dir := range append(filepath.SplitList(loginPath), "/opt/homebrew/bin", "/usr/local/bin") {
		exe := filepath.Join(dir, cmd)
		if fi, err := os.Stat(exe); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
			return exe, nil
		}
	}
	return "", errors.New("找不到 " + cmd + "，请在设置里填写它的完整路径")
}

// openChangeInEditor opens the file of the change shown, at the line of
// the caret or the change under it.
func (a *app) openChangeInEditor() {
	d := a.detail
	if d.change == nil {
		return
	}
	line := 0
	if d.whole && d.diff != nil {
		line = d.diff.New.Pos(d.diff.Cursor()).Line + 1
	} else if d.patch != nil && len(d.patch.Hunks) > 0 {
		line = d.patch.Hunks[0].NewStart
	}
	a.openInEditor(filepath.Join(a.repo.Dir, filepath.FromSlash(d.change.Path)), line)
}
