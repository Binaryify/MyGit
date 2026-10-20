package main

import (
	_ "embed"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mygit/internal/git"
	loc "mygit/locales"
)

//go:embed resources/icon.png
var iconPNG []byte

var icon, _ = ui.DecodeBitmap(iconPNG)

// welcome is the window that opens with no repository to show: a folder
// to drop on it or to choose, and the repositories opened lately.
type welcome struct {
	win    *mygo.Window
	recent []string
	errMsg string
	unsafe string // a repository another user owns, to trust
	cred   credPrompt
	clone  clonePlan
	ssh    connectPlan
	updFn  func(func()) // runs on the main thread: the window's Update, or the tests'
}

// update runs fn on the main thread.
func (w *welcome) update(fn func()) {
	if w.updFn != nil {
		w.updFn(fn)
		return
	}
	w.win.Update(fn)
}

var welcomeWin *welcome

// showWelcome opens the welcome window, or brings it to the front.
func showWelcome() {
	if welcomeWin != nil {
		welcomeWin.win.Show()
		welcomeWin.win.Focus()
		return
	}
	w := &welcome{recent: recentRepos()}
	w.win = mygo.NewWindow(customTitleBar(mygo.WindowOptions{
		Title:     "MyGit",
		Width:     520,
		Height:    640,
		MinWidth:  420,
		MinHeight: 520,
		Content:   ui.View(w.view),
	}))
	dropMenuBar(w.win)
	w.win.OnClosed(func() { welcomeWin = nil })
	welcomeWin = w
}

// closeWelcome closes the welcome window once a repository opened.
func closeWelcome() {
	if welcomeWin != nil {
		welcomeWin.win.Close()
	}
}

// recentRepos are the repositories opened lately that are still there.
func recentRepos() []string {
	var dirs []string
	for _, d := range mygo.App.RecentDocuments() {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

func (w *welcome) view(c *ui.Context) {
	defer w.cloneView(c)
	defer w.connectView(c)
	defer w.cred.view(c)
	root := ui.Column(c).Fill()
	root.Children(func() {
		titleBar(c) // nothing in it but room to drag the window by
		w.body(c)
		if root.FileDragOver() {
			dropHint(c)
		}
	})
	if paths := root.DroppedFiles(); len(paths) > 0 {
		w.open(dropTarget(paths[0]))
	}
}

func (w *welcome) body(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).Basis(0).Padding(0, 28, 28, 28).Gap(18).Children(func() {
		ui.Column(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Image(c, icon).Size(96, 96)
			ui.Text(c, "MyGit").FontSize(24).Bold()
			ui.Text(c, loc.Tagline()).TextColor(t.TextMuted)
		})
		w.dropZone(c)
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, loc.CloneMenu()).Grow(1).Clicked() {
				w.clone = clonePlan{open: true, parent: defaultParent()}
			}
			if ui.Button(c, loc.InitMenu()).Grow(1).Clicked() {
				go w.initRepo()
			}
			if ui.Button(c, loc.SshMenu()).Grow(1).Clicked() {
				w.ssh = newConnectPlan()
			}
		})
		if w.errMsg != "" {
			ui.Text(c, w.errMsg).TextColor(t.Danger).Wrap()
		}
		if w.unsafe != "" {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, loc.TrustHint()).
					FontSize(12).TextColor(t.TextMuted).Wrap().Grow(1).Basis(0)
				if ui.PrimaryButton(c, loc.TrustOpen()).Clicked() {
					dir := w.unsafe
					if err := git.Trust(dir); err != nil {
						w.errMsg = err.Error()
						return
					}
					w.open(dir)
				}
			})
		}
		if len(w.recent) > 0 || len(prefs.Remotes) > 0 {
			ui.Text(c, loc.Recent()).Bold().TextColor(t.TextMuted)
			ui.Scroll(c).Grow(1).Basis(0).Gap(2).Children(func() {
				for _, l := range prefs.Remotes {
					w.recentRow(c, l)
				}
				for _, dir := range w.recent {
					w.recentRow(c, dir)
				}
			})
		}
	})
}

// dropZone takes a folder dropped on it, or clicked, asks for one.
func (w *welcome) dropZone(c *ui.Context) {
	t := c.Theme()
	zone := ui.ButtonBase(c).Column().Height(150).Center().Gap(6).Radius(14).Border(1.5, t.Border).
		BorderStyle(ui.BorderDashed).Cursor(ui.CursorPointer).Label(loc.DropZoneLabel())
	if zone.FileDragOver() {
		zone.Border(2, t.Accent).Background(t.Selection)
	} else if zone.Hovered() {
		zone.Background(t.SurfaceHover)
	}
	zone.Children(func() {
		ui.Text(c, loc.DropZoneTitle()).FontSize(15).Bold()
		ui.Text(c, loc.DropZoneSubtitle()).TextColor(t.TextMuted)
	})
	if zone.Clicked() {
		go w.pick()
	}
}

// recentRow is a repository opened lately: a folder, or host:dir on
// another computer.
func (w *welcome) recentRow(c *ui.Context, location string) {
	t := c.Theme()
	name, where := filepath.Base(location), homeRelative(location)
	host, dir, remote := "", "", false
	if !exists(location) {
		if host, dir, remote = parseLocation(location); remote {
			name = path.Base(dir)
		}
	}
	row := ui.ButtonBase(c.Key(location)).Padding(8, 10).Gap(10).Radius(8).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Tooltip(location).Label(loc.OpenRepoNamed(name))
	if row.Hovered() {
		row.Background(t.SurfaceHover)
	}
	row.Children(func() {
		ui.Column(c).Gap(2).Grow(1).Basis(0).Children(func() {
			ui.Text(c, name).Bold().SingleLine()
			ui.Text(c, where).FontSize(11).TextColor(t.TextMuted).SingleLine()
		})
	})
	if row.Clicked() {
		if remote {
			w.connect(host, dir)
		} else {
			w.open(location)
		}
	}
}

// homeRelative writes a path under the home folder with ~.
func homeRelative(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + p[len(home):]
	}
	return p
}

// pick asks for a folder. The dialog waits: call it on a goroutine.
func (w *welcome) pick() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: w.win, Title: loc.OpenRepoDialog(), Directory: true})
	if err == nil && len(paths) > 0 {
		w.win.Update(func() { w.open(paths[0]) })
	}
}

// open opens a repository's window, and says why when the folder isn't
// in one.
func (w *welcome) open(dir string) {
	w.failed(dir, openLocal(dir))
}

// failed says why a folder didn't open, and offers to trust a repository
// another user owns.
func (w *welcome) failed(dir string, err error) {
	w.errMsg, w.unsafe = "", ""
	if err == nil {
		return
	}
	w.errMsg = err.Error()
	var unsafe *git.UnsafeError
	if errors.As(err, &unsafe) {
		w.unsafe = dir
	}
}

// dropHint covers a window while folders are dragged over it, saying
// they open as repositories.
func dropHint(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Absolute().Top(8).Right(8).Bottom(8).Left(8).Center().Gap(6).Radius(14).
		Background(t.Background.Alpha(0.88)).Border(2, t.Accent).BorderStyle(ui.BorderDashed).Children(func() {
		ui.Text(c, loc.DropHintTitle()).FontSize(16).Bold()
		ui.Text(c, loc.DropHintSubtitle()).TextColor(t.TextMuted)
	})
}

// openDropped opens the repositories of folders dropped on a repository's
// window, or of the folders of files dropped, and says in that window why
// one didn't open.
func (a *app) openDropped(paths []string) {
	a.errMsg, a.trustDir = "", ""
	for _, p := range paths {
		dir := dropTarget(p)
		if err := openLocal(dir); err != nil {
			a.errMsg = err.Error()
			var unsafe *git.UnsafeError
			if errors.As(err, &unsafe) {
				a.trustDir = dir
			}
		}
	}
}

// dropTarget is the folder a dropped path opens: itself, or a file's.
func dropTarget(p string) string {
	if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
		return filepath.Dir(p)
	}
	return p
}
