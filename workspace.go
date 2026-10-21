package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mygit/internal/git"
	loc "mygit/locales"
)

// A workspace is a folder of repositories open in one window, as VS Code's
// source control has them: each repository's changes, its message and
// its sync in the sidebar, one under the other, and the history and the
// diffs of the one worked in last beside them.
type workspace struct {
	host, dir string // the folder, on host, "" for this computer
	apps      []*app // a repository each
	active    int
	closed    map[string]bool // the repositories folded away, by folder
	win       *mygo.Window
}

// workspaces are the workspaces' windows, by folder.
var workspaces = map[string]*workspace{}

// folder is a folder in a folder: its whole path, and whether it is a
// repository's working tree.
type folder struct {
	path string
	repo bool
}

// lister lists a folder's folders, giving its whole path too.
type lister func(ctx context.Context, dir string) (string, []folder, error)

// Folders that hold no repositories of their own, or too much to look in.
var skipFolders = map[string]bool{"node_modules": true, "vendor": true, "Pods": true, "target": true}

// findRepos are the repositories in root and the folders under it, down
// to depth folders, not looking inside a repository; at most max, and
// looking in at most 400 folders, as a home folder has thousands. Root
// comes back whole.
func findRepos(ctx context.Context, list lister, root string, depth, max int) (string, []string, error) {
	whole, top, err := list(ctx, root)
	if err != nil {
		return "", nil, err
	}
	budget := 400
	var repos []string
	level := top
	for d := 1; d <= depth && len(level) > 0 && len(repos) < max; d++ {
		var next []folder
		for _, f := range level {
			if len(repos) == max {
				break
			}
			if f.repo {
				repos = append(repos, f.path)
				continue
			}
			if d == depth || skipFolders[path.Base(strings.ReplaceAll(f.path, `\`, "/"))] {
				continue
			}
			if budget--; budget < 0 {
				break
			}
			if _, sub, err := list(ctx, f.path); err == nil {
				next = append(next, sub...)
			}
		}
		level = next
	}
	return whole, repos, nil
}

// ignoredRepos are the repositories in the folders repo ignores, down to
// depth folders from its top: working trees cloned there apart, not its
// own files or its submodules, which it tracks. Git names the folders it
// ignores, so only those are looked in, a folder or so for most
// repositories, which matters over SSH, a round trip each.
func ignoredRepos(ctx context.Context, list lister, repo *git.Repo, depth, max int) []string {
	out, err := repo.Git(ctx, "ls-files", "-z", "--others", "--ignored", "--exclude-standard", "--directory", "--no-empty-directory")
	if err != nil {
		return nil
	}
	// Folders listed once, as the ignored ones share their parents.
	listed := map[string][]folder{}
	siblingsOf := func(dir string) ([]folder, error) {
		if fs, ok := listed[dir]; ok {
			return fs, nil
		}
		_, fs, err := list(ctx, dir)
		if err == nil {
			listed[dir] = fs
		}
		return fs, err
	}
	var found []string
	for _, rel := range strings.Split(out, "\x00") {
		rel, isDir := strings.CutSuffix(rel, "/")
		levels := strings.Count(rel, "/") + 1
		name := path.Base(rel)
		if !isDir || rel == "" || levels > depth || strings.HasPrefix(name, ".") || skipFolders[name] {
			continue
		}
		// The folder itself a repository, as its parent's listing tells,
		// or repositories under it.
		p := repo.Path(rel)
		siblings, err := siblingsOf(repo.Path(path.Dir(rel)))
		if err != nil {
			continue
		}
		if i := slices.IndexFunc(siblings, func(f folder) bool { return f.path == p }); i >= 0 && siblings[i].repo {
			found = append(found, p)
		} else if levels < depth {
			if _, under, err := findRepos(ctx, list, p, depth-levels, max-len(found)); err == nil {
				found = append(found, under...)
			}
		}
		if len(found) >= max {
			return found[:max]
		}
	}
	return found
}

// listLocal lists a folder of this computer, leaving out hidden folders
// and links, which could lead back up.
func listLocal(_ context.Context, dir string) (string, []folder, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", nil, err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return "", nil, err
	}
	var fs []folder
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		p := filepath.Join(abs, e.Name())
		fs = append(fs, folder{p, exists(filepath.Join(p, ".git"))})
	}
	return abs, fs, nil
}

// listOn lists a folder of host, through its helper.
func listOn(host string) lister {
	return func(ctx context.Context, dir string) (string, []folder, error) {
		l, err := listFolder(ctx, host, dir)
		if err != nil {
			return "", nil, err
		}
		fs := make([]folder, len(l.Entries))
		for i, e := range l.Entries {
			fs[i] = folder{path.Join(l.Dir, e.Name), e.Repo}
		}
		return l.Dir, fs, nil
	}
}

// openFolders opens the repository dir is in, through run, or else the
// repositories in the folders under it: a workspace's. The folder comes
// back whole.
func openFolders(ctx context.Context, run git.Runner, list lister, dir string) (string, []*git.Repo, error) {
	repo, err := git.OpenOn(run, dir)
	if err == nil {
		// A workspace folder can itself be a repository, with separately
		// cloned repositories in folders it ignores. Keep it as one section
		// beside those, but not with the repositories it tracks (submodules).
		found := ignoredRepos(ctx, list, repo, 3, 64)
		if len(found) == 0 {
			return repo.Dir, []*git.Repo{repo}, nil
		}
		repos := []*git.Repo{repo}
		for _, p := range found {
			r, err := git.OpenOn(run, p)
			if err != nil {
				continue
			}
			repos = append(repos, r)
		}
		return repo.Dir, repos, nil
	}
	var unsafe *git.UnsafeError
	if errors.As(err, &unsafe) {
		return "", nil, err
	}
	whole, found, ferr := findRepos(ctx, list, dir, 3, 64)
	if ferr != nil || len(found) == 0 {
		return "", nil, err // not a repository, nor a folder of them
	}
	var repos []*git.Repo
	var first error
	for _, p := range found {
		r, err := git.OpenOn(run, p)
		if err != nil {
			first = firstErr(first, err)
			continue
		}
		repos = append(repos, r)
	}
	if len(repos) == 0 {
		return "", nil, first
	}
	return whole, repos, nil
}

func firstErr(a, b error) error {
	if a != nil {
		return a
	}
	return b
}

// openLocal opens the repository dir is in on this computer, or the
// workspace of those under it.
func openLocal(dir string) error {
	whole, repos, err := openFolders(context.Background(), git.Local, listLocal, dir)
	if err != nil {
		return err
	}
	show("", whole, repos)
	return nil
}

// show opens repos' window: the one's, or the workspace's of dir.
func show(host, dir string, repos []*git.Repo) {
	if len(repos) == 1 {
		open(repos[0])
		return
	}
	openWorkspace(host, dir, repos)
}

// openWorkspace shows a workspace's window, the one open already if
// there is one.
func openWorkspace(host, dir string, repos []*git.Repo) {
	key := git.Location(host, dir)
	if ws := workspaces[key]; ws != nil {
		ws.win.Show()
		ws.win.Focus()
		return
	}
	name := path.Base(strings.ReplaceAll(dir, `\`, "/"))
	title := name + " — MyGit"
	if host == "" {
		_ = mygo.App.AddRecentDocument(dir)
	} else {
		addRemoteRecent(key)
		title = name + " — " + host
	}
	ws := &workspace{host: host, dir: dir, closed: map[string]bool{}}
	for _, r := range repos {
		a := newApp(r)
		a.ws = ws
		ws.apps = append(ws.apps, a)
	}
	ws.win = mygo.NewWindow(customTitleBar(mygo.WindowOptions{
		Title:     title,
		Width:     1200,
		Height:    800,
		MinWidth:  720,
		MinHeight: 480,
		StateKey:  "main",
		Content:   ui.View(ws.view),
	}))
	dropMenuBar(ws.win)
	workspaces[key] = ws
	ws.win.OnClosed(func() { delete(workspaces, key) })
	// A window closed with a file edited and not saved asks first, in the
	// repository it is in.
	ws.win.OnClose(func(e *mygo.CloseEvent) {
		for i, a := range ws.apps {
			if a.unsaved() {
				ws.activate(i)
				if a.leaving(ws.win.Close) {
					e.PreventDefault()
				}
				return
			}
		}
	})
	for _, a := range ws.apps {
		a.win = ws.win
		a.start()
	}
	closeWelcome()
}

// name is the workspace's folder's name.
func (ws *workspace) name() string { return path.Base(strings.ReplaceAll(ws.dir, `\`, "/")) }

// activate makes the i-th repository the one the history and the diffs
// show, keeping the window laid out as it was.
func (ws *workspace) activate(i int) {
	if i == ws.active {
		return
	}
	from, to := ws.apps[ws.active], ws.apps[i]
	to.sidebarW, to.logH, to.filesW, to.stagedPct = from.sidebarW, from.logH, from.filesW, from.stagedPct
	ws.active = i
}

func (ws *workspace) view(c *ui.Context) { ws.apps[ws.active].view(c) }

// sidebar is each repository's changes, one under the other.
func (ws *workspace) sidebar(c *ui.Context) {
	ui.Scroll(c).Fill().Children(func() {
		for i, a := range ws.apps {
			ws.section(c, i, a)
		}
	})
	c.OnShortcut(ui.Cmd, ui.KeyEnter, ws.apps[ws.active].commit)
}

// section is a repository of the workspace: its name, its branch and its
// actions, then, unless folded, its message, its sync and its changes.
// Working in it makes it the one the history and the diffs show.
func (ws *workspace) section(c *ui.Context, i int, a *app) {
	t := c.Theme()
	st := a.status
	dir := a.repo.Dir
	open := !ws.closed[dir]
	sec := ui.Column(c.Key(dir)).BorderWidth(0, 0, 1, 0).BorderColor(t.Border)
	sec.Children(func() {
		head := ui.Row(c).Height(30).Padding(0, 6, 0, 4).Gap(5).AlignItems(ui.Center)
		if i == ws.active {
			head.Background(t.Selection.Alpha(0.5))
		}
		head.Children(func() {
			chevron := "chevron-right"
			if open {
				chevron = "chevron-down"
			}
			if iconButton(c, chevron, a.repo.Name()) {
				ws.closed[dir] = open
			}
			ui.Icon(c, icons["repo"]).Size(13, 13)
			ui.Text(c, a.repo.Name()).Bold().SingleLine().Shrink(0).Tooltip(a.repo.Location())
			branch := st.Branch
			if branch == "" {
				branch = loc.Detached()
			}
			ui.Text(c, "⎇ "+branch).FontSize(11).TextColor(t.TextMuted).SingleLine().Grow(1).Basis(0)
			if n := counts(st); n != "" {
				ui.Text(c, n).FontSize(11).TextColor(t.TextMuted).Shrink(0)
			}
			if n := len(a.staged) + len(a.unstaged); n > 0 && !open {
				ui.Badge(c, fmt.Sprint(n))
			}
			if a.busy != "" {
				ui.Spinner(c).Size(12, 12)
			}
			if iconButton(c, "check", loc.Commit()) {
				ws.activate(i)
				a.commit()
			}
			if iconButton(c, "refresh", loc.Refresh()) {
				a.refresh()
			}
			iconMenu(c, "more", loc.Repository(), func(m *ui.Menu) {
				ws.activate(i)
				a.repoMenu(m)
			})
		})
		if !open {
			return
		}
		ui.Column(c).Gap(6).Padding(4, 10, 8, 10).Children(func() {
			// A line, growing with what is typed, as VS Code's.
			ui.TextArea(c, &a.message).Placeholder(loc.MessagePlaceholder(shortcut("↩"))).Label(loc.Message()).Lines(1, 6).MinHeight(0)
			if st.Upstream != "" && counts(st) != "" {
				if ui.Button(c, loc.SyncChanges(counts(st))).FillWidth().Disabled(a.busy != "").Clicked() {
					a.sync()
				}
			}
		})
		if len(a.staged) > 0 {
			a.changeSection(c, loc.Staged(), true, false)
		}
		if len(a.unstaged) > 0 {
			a.changeSection(c, loc.Changes(), false, false)
		}
	})
	if sec.Pressed() || sec.FocusWithin() {
		ws.activate(i)
	}
}

// counts are the commits the branch is behind its upstream and ahead of
// it, those there are: 3↓ 1↑, 126↓.
func counts(st git.Status) string {
	var parts []string
	if st.Behind > 0 {
		parts = append(parts, fmt.Sprintf("%d↓", st.Behind))
	}
	if st.Ahead > 0 {
		parts = append(parts, fmt.Sprintf("%d↑", st.Ahead))
	}
	return strings.Join(parts, " ")
}

// sync brings the upstream's commits in, as git is set to pull, then
// sends the branch's, as VS Code's Sync Changes does.
func (a *app) sync() {
	a.run(loc.OpSync(), func(ctx context.Context) error {
		if a.status.Behind > 0 {
			if err := a.repo.Pull(ctx, git.PullConfigured); err != nil {
				return err
			}
		}
		st, err := a.repo.Status(ctx)
		if err != nil {
			return err
		}
		if st.Ahead > 0 {
			return a.repo.Push(ctx, st, false)
		}
		return nil
	}, nil)
}

// appAt is the window's repository at a location, of a window of its own
// or of a workspace's.
func appAt(location string) *app {
	if a := apps[location]; a != nil {
		return a
	}
	for _, ws := range workspaces {
		for _, a := range ws.apps {
			if a.repo.Location() == location {
				return a
			}
		}
	}
	return nil
}

// allApps are the repositories of every window.
func allApps() []*app {
	var all []*app
	for _, a := range apps {
		all = append(all, a)
	}
	for _, ws := range workspaces {
		all = append(all, ws.apps...)
	}
	return all
}
