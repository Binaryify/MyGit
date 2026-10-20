package main

import (
	"context"
	"errors"
	"os"
	"path"
	"strings"

	"github.com/egoist/mygo/ui"

	"mygit/internal/git"
	"mygit/internal/helper"
	"mygit/internal/remote"
	loc "mygit/locales"
)

// connectPlan is the dialog opening a repository on another computer: a
// host to connect to, then its folders to choose the repository from.
type connectPlan struct {
	open    bool
	hosts   []string // those ~/.ssh/config names, to choose from
	host    string   // as written: user@host, an alias, or host:/path
	running bool     // connecting, listing or opening
	fail    string
	cancel  context.CancelFunc

	unsafe *git.UnsafeError // the repository that failed, owned by another user there, to trust

	// Once connected: the host and the folder shown.
	connected string
	listing   helper.Listing
	path      string // the folder's path as edited
}

func (w *welcome) connectView(c *ui.Context) {
	p := &w.ssh
	if !p.open {
		return
	}
	if ui.Modal(c, &p.open, func() {
		width := float32(460)
		if p.connected != "" {
			width = 600 // room for the folders' names
		}
		ui.Column(c).Gap(10).Width(dialogWidth(c, width)).Children(func() {
			if p.connected == "" {
				w.hostStep(c)
			} else {
				w.folderStep(c)
			}
		})
	}).Dismissed() {
		w.stopConnect()
	}
}

// hostStep asks for the computer, written or one of ~/.ssh/config.
func (w *welcome) hostStep(c *ui.Context) {
	p := &w.ssh
	t := c.Theme()
	ui.Text(c, loc.SshTitle()).Bold().FontSize(15)
	submit := ui.TextInput(c, &p.host).Placeholder(loc.SshHostPlaceholder()).Label(loc.SshHost()).AutoFocus().Submitted()
	ui.Text(c, loc.SshHint()).FontSize(12).TextColor(t.TextMuted).Wrap()
	w.connectStatus(c, loc.ConnectingTo(p.host))
	ui.Row(c).Gap(8).Children(func() {
		if len(p.hosts) > 0 {
			ui.MenuButton(c, loc.SshHosts(), func(m *ui.Menu) {
				for _, h := range p.hosts {
					if m.Item(h).Chosen() {
						p.host = h
						w.connectHost(h, "")
					}
				}
			}).Disabled(p.running)
		}
		ui.Box(c).Grow(1)
		if ui.Button(c, loc.Cancel()).Clicked() {
			w.stopConnect()
		}
		ok := strings.TrimSpace(p.host) != "" && !p.running
		if ui.PrimaryButton(c, loc.Connect()).Disabled(!ok).Clicked() || submit && ok {
			host, dir, valid := splitHost(p.host)
			if !valid {
				p.fail = loc.SshBadLocation()
				return
			}
			w.connectHost(host, dir)
		}
	})
}

// folderStep shows a folder of the computer connected to, its folders to
// go into and the repositories among them to open.
func (w *welcome) folderStep(c *ui.Context) {
	p := &w.ssh
	t := c.Theme()
	l := p.listing
	ui.Text(c, loc.SshChoose(p.connected)).Bold().FontSize(15)
	if ui.TextInput(c, &p.path).Label(loc.SshFolder()).Submitted() && !p.running {
		w.browse(strings.TrimSpace(p.path))
	}
	ui.Scroll(c).Height(360).Gap(2).Radius(8).Border(1, t.Border).Padding(4).Children(func() {
		if l.Parent != "" {
			w.folderRow(c, "up", "..", l.Parent, false)
		}
		for _, e := range l.Entries {
			icon := "folder"
			if e.Repo {
				icon = "repo"
			}
			w.folderRow(c, icon, e.Name, path.Join(l.Dir, e.Name), e.Repo)
		}
		if len(l.Entries) == 0 {
			ui.Text(c, loc.SshNoFolders()).TextColor(t.TextMuted).Padding(8, 10)
		}
	})
	w.connectStatus(c, "")
	ui.Row(c).Gap(8).Children(func() {
		if ui.Button(c, loc.SshChangeHost()).Disabled(p.running).Clicked() {
			w.ssh = connectPlan{open: true, hosts: p.hosts, host: p.connected}
			return
		}
		ui.Box(c).Grow(1)
		if ui.Button(c, loc.Cancel()).Clicked() {
			w.stopConnect()
		}
		if ui.PrimaryButton(c, loc.SshOpenFolder()).Disabled(p.running).Clicked() {
			w.openFolder(l.Dir)
		}
	})
}

// folderRow is a folder to go into, and a repository to open as well.
func (w *welcome) folderRow(c *ui.Context, icon, name, dir string, repo bool) {
	t := c.Theme()
	row := ui.ButtonBase(c.Key(dir)).Height(30).Padding(0, 8).Gap(8).Radius(6).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Label(name)
	if row.Hovered() {
		row.Background(t.SurfaceHover)
	}
	row.Children(func() {
		ui.Icon(c, icons[icon]).Size(15, 15)
		ui.Text(c, name).SingleLine().Grow(1).Basis(0)
		if repo && ui.Button(c.Key("open"), loc.SshOpen()).Height(22).Padding(0, 10).Disabled(w.ssh.running).Clicked() {
			w.openFolder(dir)
		}
	})
	if row.Clicked() && !w.ssh.running {
		w.browse(dir)
	}
}

// connectStatus shows what runs, with what it says, or why it failed.
func (w *welcome) connectStatus(c *ui.Context, running string) {
	p := &w.ssh
	t := c.Theme()
	switch {
	case p.running:
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c).Size(14, 14)
			if running != "" {
				ui.Text(c, running).TextColor(t.TextMuted).SingleLine().Grow(1).Basis(0)
			}
		})
	case p.fail != "":
		ui.Text(c, p.fail).TextColor(t.Danger).Wrap()
		if p.unsafe != nil {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, loc.TrustHint()).FontSize(12).TextColor(t.TextMuted).Wrap().Grow(1).Basis(0)
				if ui.PrimaryButton(c, loc.TrustOpen()).Tooltip(loc.TrustTooltip()).Clicked() {
					w.trustAndOpen(*p.unsafe)
				}
			})
		}
	}
}

// dialogWidth is width, or less in a window too narrow for it: as much
// as a modal's panel has room for in it.
func dialogWidth(c *ui.Context, width float32) float32 {
	w, _ := c.Size()
	return min(width, w-c.Theme().Space(20))
}

// openSSH shows the welcome window with its dialog to open a repository
// over SSH, from a repository's window or the menu bar.
func openSSH() {
	showWelcome()
	w := welcomeWin
	w.win.Update(func() {
		if !w.ssh.open {
			w.ssh = newConnectPlan()
		}
	})
}

// newConnectPlan opens the dialog with the hosts of ~/.ssh/config.
func newConnectPlan() connectPlan {
	home, _ := os.UserHomeDir()
	return connectPlan{open: true, hosts: remote.ConfigHosts(home)}
}

// splitHost reads the host written, and the folder to start in when it is
// host:/path; "" for the home folder.
func splitHost(s string) (host, dir string, ok bool) {
	if host, dir, ok := parseLocation(s); ok {
		return host, dir, true
	}
	s = strings.TrimSpace(s)
	return s, "", s != "" && !strings.ContainsAny(s, `/\ `)
}

// makeRoom grows the window to width and height when it is smaller,
// about its center, for the folders to choose from.
func (w *welcome) makeRoom(width, height int) {
	if w.win == nil {
		return // tests
	}
	b := w.win.Bounds()
	dw, dh := max(width-b.Width, 0), max(height-b.Height, 0)
	if dw == 0 && dh == 0 {
		return
	}
	b.X, b.Y, b.Width, b.Height = b.X-dw/2, max(b.Y-dh/2, 0), b.Width+dw, b.Height+dh
	w.win.SetBounds(b)
}

func (w *welcome) stopConnect() {
	if w.ssh.cancel != nil {
		w.ssh.cancel()
	}
	w.ssh = connectPlan{}
}

// task runs fn on a goroutine while the dialog shows it running, then
// done on the main thread, unless the dialog was closed meanwhile.
func (w *welcome) task(fn func(ctx context.Context) error, done func()) {
	ctx, cancel := context.WithCancel(context.Background())
	w.ssh.running, w.ssh.fail, w.ssh.unsafe, w.ssh.cancel = true, "", nil, cancel
	go func() {
		err := fn(ctx)
		w.update(func() {
			if ctx.Err() != nil {
				return // canceled
			}
			w.ssh.running = false
			if err != nil {
				w.ssh.fail = err.Error()
				if u := (*git.UnsafeError)(nil); errors.As(err, &u) && u.Host != "" {
					w.ssh.unsafe = u
				}
				return
			}
			done()
		})
	}()
}

// listFolder lists dir on host, starting its helper if it isn't running.
var listFolder = func(ctx context.Context, host, dir string) (helper.Listing, error) {
	c, err := connect(ctx, host)
	if err != nil {
		return helper.Listing{}, err
	}
	return c.List(ctx, dir)
}

// connectHost starts the helper on host and lists dir there, the home
// folder for "" or when dir isn't there.
func (w *welcome) connectHost(host, dir string) {
	var l helper.Listing
	w.task(func(ctx context.Context) (err error) {
		if l, err = listFolder(ctx, host, dir); err != nil && dir != "" && !errors.Is(err, context.Canceled) {
			l, err = listFolder(ctx, host, "")
		}
		return err
	}, func() {
		w.ssh.connected, w.ssh.listing, w.ssh.path = host, l, l.Dir
		w.makeRoom(680, 720)
	})
}

// browse shows dir of the computer connected to.
func (w *welcome) browse(dir string) {
	host := w.ssh.connected
	var l helper.Listing
	w.task(func(ctx context.Context) (err error) {
		l, err = listFolder(ctx, host, dir)
		return err
	}, func() {
		w.ssh.listing, w.ssh.path = l, l.Dir
	})
}

// openFolder opens the repository dir is in, on the computer connected to.
func (w *welcome) openFolder(dir string) {
	host := w.ssh.connected
	var whole string
	var repos []*git.Repo
	w.task(func(ctx context.Context) (err error) {
		whole, repos, err = openRemote(ctx, host, dir)
		return err
	}, func() {
		w.ssh.open = false
		show(host, whole, repos)
	})
}

// trustAndOpen has git on the repository's computer trust it, as the
// user said to, and opens it. Its path is made whole first, as git's
// safe.directory takes it.
func (w *welcome) trustAndOpen(u git.UnsafeError) {
	var whole string
	var repos []*git.Repo
	w.task(func(ctx context.Context) error {
		l, err := listFolder(ctx, u.Host, u.Dir)
		if err != nil {
			return err
		}
		if err := trustRemote(ctx, u.Host, l.Dir); err != nil {
			return err
		}
		whole, repos, err = openRemote(ctx, u.Host, l.Dir)
		return err
	}, func() {
		w.ssh.open = false
		show(u.Host, whole, repos)
	})
}

// trustRemote lets git work in dir on host.
var trustRemote = func(ctx context.Context, host, dir string) error {
	return git.TrustOn(ctx, remoteHost{host}, dir)
}

// connect opens the repository dir is in on host, as one opened lately,
// starting the helper there.
func (w *welcome) connect(host, dir string) {
	w.ssh = connectPlan{open: true, host: git.Location(host, dir)}
	var whole string
	var repos []*git.Repo
	w.task(func(ctx context.Context) (err error) {
		whole, repos, err = openRemote(ctx, host, dir)
		return err
	}, func() {
		w.ssh.open = false
		show(host, whole, repos)
	})
}
