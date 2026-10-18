package main

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

// ignoreItems add an untracked file, or the files of its type or folder,
// to the .gitignore at the top of the working tree.
func (a *app) ignoreItems(m *ui.Menu, file string) {
	m.Submenu("添加到 .gitignore", func(m *ui.Menu) {
		if m.Item("/" + file).Chosen() {
			a.addIgnore("/" + file)
		}
		if ext := path.Ext(file); ext != "" && m.Item("*"+ext).Chosen() {
			a.addIgnore("*" + ext)
		}
		if dir := path.Dir(file); dir != "." && m.Item("/"+dir+"/").Chosen() {
			a.addIgnore("/" + dir + "/")
		}
	})
}

// addIgnore appends a pattern to .gitignore, on a line of its own.
func (a *app) addIgnore(pattern string) {
	if err := appendIgnore(a.repo.Dir, pattern); err != nil {
		a.errMsg = "写入 .gitignore 失败：" + err.Error()
		return
	}
	a.refresh()
}

func appendIgnore(dir, pattern string) error {
	file := filepath.Join(dir, ".gitignore")
	old, err := os.ReadFile(file)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, l := range strings.Split(string(old), "\n") {
		if strings.TrimSpace(l) == pattern {
			return nil
		}
	}
	text := string(old)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return os.WriteFile(file, []byte(text+pattern+"\n"), 0o644)
}
