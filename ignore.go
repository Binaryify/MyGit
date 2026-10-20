package main

import (
	"context"
	"errors"
	"io/fs"
	"path"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mygit/internal/git"
	loc "mygit/locales"
)

// ignoreItems add an untracked file, or the files of its type or folder,
// to the .gitignore at the top of the working tree.
func (a *app) ignoreItems(m *ui.Menu, file string) {
	m.Submenu(loc.AddToGitignore(), func(m *ui.Menu) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := appendIgnore(ctx, a.repo, pattern); err != nil {
		a.errMsg = loc.ErrGitignore(err.Error())
		return
	}
	a.refresh()
}

func appendIgnore(ctx context.Context, repo *git.Repo, pattern string) error {
	old, err := repo.ReadFile(ctx, ".gitignore")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, l := range strings.Split(old, "\n") {
		if strings.TrimSpace(l) == pattern {
			return nil
		}
	}
	text := old
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return repo.WriteFile(ctx, ".gitignore", text+pattern+"\n")
}
