package main

import (
	"slices"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mygit/internal/git"
	loc "mygit/locales"
)

// projectButton is the repository's name in the toolbar, which opens the
// project's menu as it is clicked, with either button.
func (a *app) projectButton(c *ui.Context) {
	t := c.Theme()
	name := a.repo.Name()
	b := ui.ButtonBase(c.Key("project")).Padding(2, 6).Gap(4).Radius(5).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Tooltip(a.repo.Location()).Label(name)
	if b.Hovered() {
		b.Background(t.Text.Alpha(0.12))
	}
	b.Children(func() {
		ui.Text(c, name).FontSize(13).Bold().SingleLine()
		ui.Icon(c, icons["chevron-down"]).Size(12, 12)
	})
	b.Menu(a.projectMenu).ContextMenu(a.projectMenu)
}

// projectMenu says where the repository is and where its remotes are, to
// copy, among many projects (#32), and switches the window to a
// repository opened lately, or closes it, as an IDE's project menu does.
func (a *app) projectMenu(m *ui.Menu) {
	if m.Item(loc.CopyLocalPath(a.repo.Dir)).Chosen() {
		a.copyText(a.repo.Dir)
	}
	for _, r := range a.remoteURLs {
		if m.Item(loc.CopyRemoteURL(r.Name, r.URL)).Chosen() {
			a.copyText(r.URL)
		}
	}
	m.Separator()
	if ws := a.ws; ws != nil && len(ws.apps) > 1 {
		// The repository the history shows, as choosing its changes
		// makes it, chosen here too (#54).
		m.Submenu(loc.RepoShown(), func(m *ui.Menu) {
			for i, b := range ws.apps {
				if !ws.hidden(i) && m.Item(b.repo.Name()).Checked(i == ws.active).Chosen() {
					ws.activate(i)
				}
			}
		})
		m.Separator()
	}
	here := a.locations()
	if slices.ContainsFunc(recentLocations(), func(l string) bool { return !slices.Contains(here, l) }) {
		m.Submenu(loc.SwitchProject(), func(m *ui.Menu) {
			recentItems(m, here, a.switchProject)
		})
		m.Separator()
	}
	if m.Item(loc.CloseProject()).Chosen() {
		a.closeProject()
	}
}

// copyText puts text on the clipboard, saying so.
func (a *app) copyText(text string) {
	mygo.Clipboard.WriteText(text)
	a.notice = loc.CopiedText(text)
}

// locations are where the window's repository is: its own, and its
// workspace's.
func (a *app) locations() []string {
	l := []string{a.repo.Location()}
	if a.ws != nil {
		l = append(l, git.Location(a.ws.host, a.ws.dir))
	}
	return l
}

// switchProject opens a repository opened lately in place of the window's.
func (a *app) switchProject(location string) {
	openRecent(location)
	a.closeProject()
}

// closeProject closes the window, its workspace's too, asking first about
// a file edited and not saved, and goes back to the welcome window when
// no repository's is left open, instead of quitting.
func (a *app) closeProject() {
	a.win.OnClosed(func() {
		if len(apps)+len(workspaces) == 0 {
			showWelcome()
		}
	})
	a.win.Close()
}

// recentItems are a menu's items of the repositories opened lately but
// those except, and choose opens the one chosen.
func recentItems(m *ui.Menu, except []string, choose func(location string)) {
	for _, l := range recentLocations() {
		if !slices.Contains(except, l) && m.Item(homeRelative(l)).Chosen() {
			choose(l)
		}
	}
}
