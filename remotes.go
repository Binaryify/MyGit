package main

import (
	"context"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mygit/internal/git"
	loc "mygit/locales"
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
	title, button, op := loc.RemoteAddTitle(), loc.Add(), loc.OpAddRemote()
	if r.Name != "" {
		title, button, op = loc.RemoteEditTitle(r.Name), loc.Save(), loc.OpSaveRemote()
	} else {
		r.Name = "origin"
		if len(a.remotes2.list) > 0 {
			r.Name = "upstream"
		}
	}
	old := r
	a.prompt = prompt{open: true, title: title, button: button, value: r.Name, message: loc.RemoteURL(), msgVal: r.URL,
		ok: func(name, url string) {
			a.run(op, func(ctx context.Context) error {
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
			ui.Text(c, loc.Remotes()).Bold().FontSize(15)
			if len(p.list) == 0 {
				ui.Text(c, loc.RemotesNone()).TextColor(t.TextMuted)
			}
			for _, r := range p.list {
				ui.Row(c.Key(r.Name)).Gap(8).AlignItems(ui.Center).Padding(4, 0).Children(func() {
					ui.Column(c).Gap(2).Grow(1).Basis(0).Children(func() {
						ui.Text(c, r.Name).Bold()
						ui.Text(c, r.URL).FontSize(11).TextColor(t.TextMuted).SingleLine().Tooltip(r.URL)
					})
					if ui.Button(c, loc.Fetch()).Disabled(a.busy != "").Clicked() {
						a.run(loc.OpFetchRemote(r.Name), func(ctx context.Context) error { return a.repo.FetchRemote(ctx, r.Name) }, nil)
					}
					if ui.Button(c, loc.Edit()).Clicked() {
						a.editRemote(r)
					}
					if ui.Button(c, loc.DeleteMenu()).Clicked() {
						a.ask(loc.RemoteDeleteTitle(r.Name), loc.RemoteDeleteMessage(), loc.Delete(), func() {
							a.run(loc.OpDeleteRemote(), func(ctx context.Context) error { return a.repo.RemoveRemote(ctx, r.Name) }, a.loadRemotes)
						})
					}
				})
			}
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c, loc.RemoteAddMenu()).Clicked() {
					a.editRemote(git.Remote{})
				}
				ui.Box(c).Grow(1)
				if ui.PrimaryButton(c, loc.Done()).Clicked() {
					p.open = false
				}
			})
		})
	})
}

// repoMenu is the toolbar's menu of the repository's own things.
func (a *app) repoMenu(m *ui.Menu) {
	if m.Item(loc.RemotesMenu()).Chosen() {
		a.openRemotes()
	}
	if m.Item(loc.PullRequestsMenu()).Chosen() {
		a.openPulls()
	}
	if m.Item(loc.ShowInFolder()).Chosen() {
		mygo.Shell.ShowItemInFolder(a.repo.Dir)
	}
	if m.Item(loc.OpenRepoInEditor()).Chosen() {
		a.openInEditor(a.repo.Dir, 0)
	}
	m.Separator()
	if m.Item(loc.MenuOpenRepo()).Chosen() {
		showWelcome()
	}
	if recent := recentRepos(); len(recent) > 0 {
		m.Submenu(loc.MenuRecent(), func(m *ui.Menu) {
			for _, dir := range recent {
				if m.Item(homeRelative(dir)).Chosen() {
					openDir(dir)
				}
			}
		})
	}
	if m.Item(loc.MenuInstallCLI()).Chosen() {
		go installLauncher()
	}
	m.Separator()
	if m.Item(loc.SettingsMenu()).Chosen() {
		a.settingsOpen = true
	}
	if mygo.Updater.Enabled() && m.Item(loc.UpdateCheck()).Chosen() {
		checkUpdate(func(msg string) { a.notice = msg })
	}
	if m.Item(loc.About()).Chosen() {
		go about()
	}
}
