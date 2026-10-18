package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mygit/internal/git"
)

// clonePlan is the clone dialog: a URL, the folder to clone into, and the
// clone's progress once it runs.
type clonePlan struct {
	open           bool
	url, parent    string
	name           string // the folder's name, from the URL until edited
	named          bool
	running        bool
	progress, fail string
	cancel         context.CancelFunc
}

// defaultParent is where clones go: ~/Projects when there is one.
func defaultParent() string {
	home, _ := os.UserHomeDir()
	for _, d := range []string{"Projects", "projects", "Code", "code", "src"} {
		if fi, err := os.Stat(filepath.Join(home, d)); err == nil && fi.IsDir() {
			return filepath.Join(home, d)
		}
	}
	return home
}

func (w *welcome) cloneView(c *ui.Context) {
	p := &w.clone
	if !p.open {
		return
	}
	t := c.Theme()
	if ui.Modal(c, &p.open, func() {
		ui.Column(c).Gap(10).Width(460).Children(func() {
			ui.Text(c, "克隆仓库").Bold().FontSize(15)
			if ui.TextInput(c, &p.url).Placeholder("https://github.com/user/repo.git 或 git@…").Label("仓库地址").AutoFocus().Changed() && !p.named {
				p.name = git.RepoName(strings.TrimSpace(p.url))
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, homeRelative(p.parent)).TextColor(t.TextMuted).SingleLine().Grow(1).Basis(0).Tooltip(p.parent)
				if ui.Button(c, "更改…").Disabled(p.running).Clicked() {
					go w.pickParent()
				}
			})
			if ui.TextInput(c, &p.name).Label("文件夹名").Placeholder("文件夹名").Changed() {
				p.named = true
			}
			switch {
			case p.running:
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Spinner(c).Size(14, 14)
					ui.Text(c, p.progress).TextColor(t.TextMuted).SingleLine().Grow(1).Basis(0)
				})
			case p.fail != "":
				ui.Text(c, p.fail).TextColor(t.Danger).Wrap()
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					w.stopClone()
				}
				ok := strings.TrimSpace(p.url) != "" && strings.TrimSpace(p.name) != "" && !p.running
				if ui.PrimaryButton(c, "克隆").Disabled(!ok).Clicked() {
					w.startClone()
				}
			})
		})
	}).Dismissed() {
		w.stopClone()
	}
}

func (w *welcome) stopClone() {
	if w.clone.cancel != nil {
		w.clone.cancel()
	}
	w.clone = clonePlan{}
}

func (w *welcome) pickParent() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: w.win, Title: "克隆到", Directory: true, CreateDirectories: true})
	if err == nil && len(paths) > 0 {
		w.win.Update(func() { w.clone.parent = paths[0] })
	}
}

// startClone clones on a goroutine, showing git's progress, and opens the
// clone once done.
func (w *welcome) startClone() {
	p := &w.clone
	url, parent, name := strings.TrimSpace(p.url), p.parent, strings.TrimSpace(p.name)
	ctx, cancel := context.WithCancel(context.Background())
	p.running, p.fail, p.progress, p.cancel = true, "", "正在连接…", cancel
	go func() {
		err := git.Clone(ctx, parent, url, name, func(line string) {
			w.win.Update(func() { w.clone.progress = line })
		})
		w.win.Update(func() {
			if ctx.Err() != nil {
				return // canceled
			}
			w.clone.running = false
			if err != nil {
				w.clone.fail = err.Error()
				return
			}
			w.clone.open = false
			w.open(filepath.Join(parent, name))
		})
	}()
}

// initRepo asks for a folder, makes a repository in it and opens it. The
// dialog waits: call it on a goroutine.
func (w *welcome) initRepo() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: w.win, Title: "在文件夹里新建仓库", Directory: true, CreateDirectories: true})
	if err != nil || len(paths) == 0 {
		return
	}
	dir := paths[0]
	err = git.Init(context.Background(), dir)
	w.win.Update(func() {
		if err != nil {
			w.errMsg = err.Error()
			return
		}
		w.open(dir)
	})
}
