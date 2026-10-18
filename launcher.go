package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
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
		launcherMessage("找不到 mygit 的可执行文件", err.Error())
		return
	}
	if runtime.GOOS == "windows" {
		launcherMessage("命令行工具", "把这个文件夹加入 PATH 后，就能在终端里用 mygit 打开仓库：\n"+filepath.Dir(exe))
		return
	}
	dir, err := launcherDir()
	if err != nil {
		launcherMessage("没有可写的 PATH 目录", "把这个可执行文件链接到 PATH 里的目录即可：\n"+exe)
		return
	}
	path := filepath.Join(dir, "mygit")
	// The repository's path is made absolute here, as the running app has
	// another working folder.
	script := fmt.Sprintf("#!/bin/sh\ndir=$(cd \"${1:-.}\" && pwd) || exit 1\n%s \"$dir\" >/dev/null 2>&1 &\n", shellWord(exe))
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		launcherMessage("安装失败", err.Error())
		return
	}
	launcherMessage("已安装命令行工具", "现在可以在终端里运行：\n  mygit .\n\n命令写在 "+path)
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
