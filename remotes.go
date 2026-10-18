package main

import (
	"context"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mygit/internal/git"
)

// remotesPlan is the dialog listing the remotes, to add, edit or remove.
type remotesPlan struct {
	open bool
	list []git.Remote
}

func (a *app) openRemotes() {
	a.remotes2.open = true
	a.loadRemotes()
}

func (a *app) loadRemotes() {
	work(a, a.repo.Remotes, func(list []git.Remote, err error) {
		a.remotes2.list = list
		if err != nil {
			a.errMsg = err.Error()
		}
	})
}

// editRemote asks for a remote's name and URL, a new one's when r is
// empty.
func (a *app) editRemote(r git.Remote) {
	title, button := "添加远程仓库", "添加"
	if r.Name != "" {
		title, button = "编辑远程仓库 "+r.Name, "保存"
	} else {
		r.Name = "origin"
		if len(a.remotes2.list) > 0 {
			r.Name = "upstream"
		}
	}
	old := r
	a.prompt = prompt{open: true, title: title, button: button, value: r.Name, message: "地址", msgVal: r.URL,
		ok: func(name, url string) {
			a.run(button+"远程仓库", func(ctx context.Context) error {
				if old.URL == "" {
					return a.repo.AddRemote(ctx, name, url)
				}
				if name != old.Name {
					if err := a.repo.RenameRemote(ctx, old.Name, name); err != nil {
						return err
					}
				}
				if url != old.URL {
					return a.repo.SetRemoteURL(ctx, name, url)
				}
				return nil
			}, a.loadRemotes)
		}}
}

func (a *app) remotesView(c *ui.Context) {
	p := &a.remotes2
	if !p.open || a.prompt.open || a.confirm.open {
		return // its own dialogs show over the window in its place
	}
	t := c.Theme()
	ui.Modal(c, &p.open, func() {
		ui.Column(c).Gap(10).Width(560).Children(func() {
			ui.Text(c, "远程仓库").Bold().FontSize(15)
			if len(p.list) == 0 {
				ui.Text(c, "还没有远程仓库").TextColor(t.TextMuted)
			}
			for _, r := range p.list {
				ui.Row(c.Key(r.Name)).Gap(8).AlignItems(ui.Center).Padding(4, 0).Children(func() {
					ui.Column(c).Gap(2).Grow(1).Basis(0).Children(func() {
						ui.Text(c, r.Name).Bold()
						ui.Text(c, r.URL).FontSize(11).TextColor(t.TextMuted).SingleLine().Tooltip(r.URL)
					})
					if ui.Button(c, "获取").Disabled(a.busy != "").Clicked() {
						a.run("获取 "+r.Name, func(ctx context.Context) error { return a.repo.FetchRemote(ctx, r.Name) }, nil)
					}
					if ui.Button(c, "编辑…").Clicked() {
						a.editRemote(r)
					}
					if ui.Button(c, "删除…").Clicked() {
						a.ask("删除远程仓库 "+r.Name+"？", "只删除本地对它的记录和远程分支的引用，不会动远程仓库本身。", "删除", func() {
							a.run("删除远程仓库", func(ctx context.Context) error { return a.repo.RemoveRemote(ctx, r.Name) }, a.loadRemotes)
						})
					}
				})
			}
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c, "添加远程仓库…").Clicked() {
					a.editRemote(git.Remote{})
				}
				ui.Box(c).Grow(1)
				if ui.PrimaryButton(c, "完成").Clicked() {
					p.open = false
				}
			})
		})
	})
}

// repoMenu is the toolbar's menu of the repository's own things.
func (a *app) repoMenu(m *ui.Menu) {
	if m.Item("远程仓库…").Chosen() {
		a.openRemotes()
	}
	if m.Item("在文件夹中显示").Chosen() {
		mygo.Shell.ShowItemInFolder(a.repo.Dir)
	}
	if m.Item("在编辑器中打开仓库").Chosen() {
		a.openInEditor(a.repo.Dir, 0)
	}
	m.Separator()
	if m.Item("打开仓库…").Chosen() {
		showWelcome()
	}
	if recent := recentRepos(); len(recent) > 0 {
		m.Submenu("最近打开", func(m *ui.Menu) {
			for _, dir := range recent {
				if m.Item(homeRelative(dir)).Chosen() {
					openDir(dir)
				}
			}
		})
	}
	if m.Item("安装命令行工具…").Chosen() {
		go installLauncher()
	}
	m.Separator()
	if m.Item("设置…").Chosen() {
		a.settingsOpen = true
	}
}
