package main

import (
	_ "embed"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mygit/internal/git"
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
	w.win = mygo.NewWindow(mygo.WindowOptions{
		Title:     "MyGit",
		Width:     520,
		Height:    600,
		MinWidth:  420,
		MinHeight: 480,
		Content:   ui.View(w.view),
	})
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
	t := c.Theme()
	defer w.cloneView(c)
	defer w.cred.view(c)
	ui.Column(c).Fill().Padding(28).Gap(18).Children(func() {
		ui.Column(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Image(c, icon).Size(96, 96)
			ui.Text(c, "MyGit").FontSize(24).Bold()
			ui.Text(c, "轻量的 git 客户端").TextColor(t.TextMuted)
		})
		w.dropZone(c)
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, "克隆仓库…").Grow(1).Clicked() {
				w.clone = clonePlan{open: true, parent: defaultParent()}
			}
			if ui.Button(c, "新建仓库…").Grow(1).Clicked() {
				go w.initRepo()
			}
		})
		if w.errMsg != "" {
			ui.Text(c, w.errMsg).TextColor(t.Danger).Wrap()
		}
		if w.unsafe != "" {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "确认这是你自己的仓库，就可以让 git 信任它（写入 git 的 safe.directory 设置）。").
					FontSize(12).TextColor(t.TextMuted).Wrap().Grow(1).Basis(0)
				if ui.PrimaryButton(c, "信任并打开").Clicked() {
					dir := w.unsafe
					if err := git.Trust(dir); err != nil {
						w.errMsg = err.Error()
						return
					}
					w.open(dir)
				}
			})
		}
		if len(w.recent) > 0 {
			ui.Text(c, "最近打开").Bold().TextColor(t.TextMuted)
			ui.Scroll(c).Grow(1).Basis(0).Gap(2).Children(func() {
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
		BorderStyle(ui.BorderDashed).Cursor(ui.CursorPointer).Label("拖入或选择仓库文件夹")
	if zone.FileDragOver() {
		zone.Border(2, t.Accent).Background(t.Selection)
	} else if zone.Hovered() {
		zone.Background(t.SurfaceHover)
	}
	zone.Children(func() {
		ui.Text(c, "把仓库文件夹拖到这里").FontSize(15).Bold()
		ui.Text(c, "或点击选择文件夹").TextColor(t.TextMuted)
	})
	if files := zone.DroppedFiles(); len(files) > 0 {
		w.open(files[0])
	}
	if zone.Clicked() {
		go w.pick()
	}
}

func (w *welcome) recentRow(c *ui.Context, dir string) {
	t := c.Theme()
	row := ui.ButtonBase(c.Key(dir)).Padding(8, 10).Gap(10).Radius(8).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Tooltip(dir).Label("打开 " + filepath.Base(dir))
	if row.Hovered() {
		row.Background(t.SurfaceHover)
	}
	row.Children(func() {
		ui.Column(c).Gap(2).Grow(1).Basis(0).Children(func() {
			ui.Text(c, filepath.Base(dir)).Bold().SingleLine()
			ui.Text(c, homeRelative(dir)).FontSize(11).TextColor(t.TextMuted).SingleLine()
		})
	})
	if row.Clicked() {
		w.open(dir)
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
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: w.win, Title: "打开 git 仓库", Directory: true})
	if err == nil && len(paths) > 0 {
		w.win.Update(func() { w.open(paths[0]) })
	}
}

// open opens a repository's window, and says why when the folder isn't
// in one.
func (w *welcome) open(dir string) {
	repo, err := git.Open(dir)
	w.failed(dir, err)
	if err == nil {
		open(repo)
	}
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
