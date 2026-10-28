package main

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mygit/internal/git"
	"mygit/internal/patch"
	"mygit/internal/remote"
	loc "mygit/locales"
)

const rowHeight = 26

func (a *app) view(c *ui.Context) {
	t := c.Theme()
	root := ui.Column(c).Fill().FontSize(12)
	root.Children(func() {
		a.toolbar(c)
		a.updateBar(c)
		if a.notice != "" {
			ui.Row(c).Padding(6, 12).Gap(8).Background(t.Accent.Alpha(0.1)).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.notice).Wrap().Grow(1)
				if ui.Button(c, loc.Close()).Clicked() {
					a.notice = ""
				}
			})
		}
		if a.errMsg != "" {
			ui.Row(c).Padding(6, 12).Gap(8).Background(t.Danger.Alpha(0.12)).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.errMsg).TextColor(t.Danger).Wrap().Grow(1)
				if a.trustDir != "" && ui.Button(c, loc.TrustOpen()).Tooltip(loc.TrustTooltip()).Clicked() {
					if err := git.Trust(a.trustDir); err != nil {
						a.errMsg = err.Error()
					} else {
						a.openDropped([]string{a.trustDir})
					}
				}
				if ui.Button(c, loc.Close()).Clicked() {
					a.errMsg, a.trustDir = "", ""
				}
			})
		}
		a.operationBar(c)
		ui.Split(c, &a.sidebarW, func() {
			if a.ws != nil {
				a.ws.sidebar(c)
			} else {
				a.leftPanel(c)
			}
		}, func() {
			ui.SplitVertical(c, &a.logH, func() { a.logView(c) }, func() { a.detailView(c) }).Fill()
		}).Grow(1)
		if root.FileDragOver() {
			dropHint(c)
		}
	})
	if paths := root.DroppedFiles(); len(paths) > 0 {
		a.openDropped(paths)
	}
	a.rebaseView(c)
	a.rewordView(c)
	a.pushView(c)
	a.promptView(c)
	a.worktreeView(c)
	a.remotesView(c)
	a.settingsView(c)
	a.confirmView(c)
	a.cred.view(c)
	a.paletteView(c)
	a.composerView(c)
	a.pullsView(c)
	a.submitView(c)
	c.OnShortcut(ui.Cmd, ui.KeyR, a.refresh)
	c.OnShortcut(ui.Cmd, ui.KeyP, a.openFiles)
	c.OnShortcut(ui.Cmd, ui.KeyK, a.openCommands)
	c.OnShortcut(0, ui.KeyF7, func() { a.stepChange(false) })
	c.OnShortcut(ui.Shift, ui.KeyF7, func() { a.stepChange(true) })
	if !macOS { // the menu bar's on macOS, which other systems' windows don't have
		c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyO, openSSH)
		c.OnShortcut(ui.Cmd, ui.KeyComma, func() { a.settings.open = true })
	}
}

// remoteBadge says the repository is on another computer, and which.
// hostName is how a host shows: WSL · Ubuntu for a distribution of WSL's,
// which is written wsl+Ubuntu, else as it is written.
func hostName(host string) string {
	if distro, ok := remote.WSL(host); ok {
		return "WSL · " + distro
	}
	return host
}

func (a *app) remoteBadge(c *ui.Context) {
	t := c.Theme()
	host, dir := a.repo.Host(), a.repo.Dir
	label, tip := "SSH · "+host, loc.RemoteBadge(host, dir)
	if distro, ok := remote.WSL(host); ok {
		label, tip = hostName(host), loc.RemoteBadgeWsl(distro, dir)
	}
	ui.Row(c).Gap(5).AlignItems(ui.Center).Padding(2, 8).Radius(10).Background(t.Accent.Alpha(0.16)).
		TextColor(t.AccentText).Tooltip(tip).Children(func() {
		ui.Icon(c, icons["server"]).Size(12, 12)
		ui.Text(c, label).FontSize(12).Bold().TextColor(t.AccentText).SingleLine()
	})
}

func (a *app) toolbar(c *ui.Context) {
	t := c.Theme()
	st := a.status
	titleBar(c).Background(t.Surface).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		ui.Image(c, icon).Size(22, 22)
		if a.ws != nil {
			ui.Text(c, a.ws.name()+" /").FontSize(13).TextColor(t.TextMuted)
		}
		ui.Text(c, a.repo.Name()).FontSize(13).Bold().Tooltip(a.repo.Location())
		if !a.local() {
			a.remoteBadge(c)
		}
		branch := st.Branch
		if branch == "" {
			branch = loc.Detached()
		}
		a.branchesButton(c, "⎇ "+branch+" ▾")
		if st.Upstream != "" {
			ui.Text(c, fmt.Sprintf("%s  ↑%d ↓%d", st.Upstream, st.Ahead, st.Behind)).TextColor(t.TextMuted)
		}
		ui.Box(c).Grow(1)
		if a.busy != "" {
			ui.Spinner(c).Size(14, 14)
			ui.Text(c, a.busy+"…").TextColor(t.TextMuted)
		}
		idle := a.busy == ""
		ui.Button(c, loc.Fetch()+" ▾").Label(loc.Fetch()).Disabled(!idle).Menu(func(m *ui.Menu) {
			if m.Item(loc.FetchAll()).Chosen() {
				a.run(loc.OpFetch(), a.repo.Fetch, nil)
			}
			if names := a.remoteNames(); len(names) > 1 {
				m.Separator()
				for _, name := range names {
					if m.Item(loc.FetchRemote(name)).Chosen() {
						a.run(loc.OpFetchRemote(name), func(ctx context.Context) error { return a.repo.FetchRemote(ctx, name) }, nil)
					}
				}
			}
		})
		ui.Button(c, loc.Pull()+" ▾").Label(loc.Pull()).Disabled(!idle || st.Upstream == "").Menu(func(m *ui.Menu) {
			pull := func(mode git.PullMode) {
				a.runPull(loc.OpPull(), func(ctx context.Context) error { return a.repo.Pull(ctx, mode) })
			}
			if m.Item(loc.PullFF()).Chosen() {
				pull(git.PullFastForward)
			}
			if m.Item(loc.PullRebase()).Chosen() {
				pull(git.PullRebase)
			}
			if m.Item(loc.PullMerge()).Chosen() {
				pull(git.PullMerge)
			}
		})
		if ui.Button(c, loc.PushMenu()).Tooltip(loc.PushTooltip()).Disabled(!idle || st.Branch == "").Clicked() {
			a.openPush()
		}
		a.commentsButton(c)
		if ui.Button(c, loc.Refresh()).Tooltip(shortcut("R")).Clicked() {
			a.refresh()
		}
		ui.Button(c, "⋯").Label(loc.Repository()).Menu(a.repoMenu)
	})
}

// changesPanel is the working tree's changes, staged and not, and the
// commit message under them.
func (a *app) changesPanel(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).Basis(0).Children(func() {
		// The message on top, as VS Code has it, where it shows however
		// many files changed.
		ui.Column(c).Gap(8).Padding(10).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
			a.messageField(c).Lines(2, 8)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if ui.Checkbox(c, &a.amend, loc.Amend()).Changed() {
					a.toggleAmend()
				}
				ui.Box(c).Grow(1)
				label := loc.Commit()
				if n := len(a.staged); n > 0 {
					label = loc.CommitFiles(n)
				}
				if ui.PrimaryButton(c, label).Disabled(!a.canCommit()).Tooltip(shortcut("↩")).Clicked() {
					a.commit()
				}
			})
		})
		if len(a.staged)+len(a.unstaged) > 0 || a.filter.query != "" {
			a.filterField(c)
		}
		// Dragged as the room the staged changes need, few or thousands.
		ui.SplitVerticalPercent(c.Key("changes"), &a.stagedPct, func() {
			ui.Column(c).Fill().Children(func() { a.changeSection(c, loc.Staged(), true, true) })
		}, func() {
			ui.Column(c).Fill().Children(func() { a.changeSection(c, loc.Changes(), false, true) })
		}).Grow(1).Basis(0)
	})
	c.OnShortcut(ui.Cmd, ui.KeyEnter, a.commit)
}

// messageField is the commit message's box, where Up brings back the
// recent messages.
func (a *app) messageField(c *ui.Context) ui.Element {
	in := ui.TextArea(c, &a.message).Placeholder(loc.MessagePlaceholder(shortcut("↩"))).Label(loc.Message()).OnTextKey(a.recallKey)
	if a.recallEnd {
		n := utf8.RuneCountInString(a.message)
		in.SetTextSelection(n, n)
		a.recallEnd = false
	}
	return in
}

// changeSection lists the staged changes or the others, those the filter
// shows, filling the room left when fill, else as tall as its rows, as a
// workspace's repositories stack them.
func (a *app) changeSection(c *ui.Context, title string, staged, fill bool) {
	t := c.Theme()
	changes := a.changes(staged)
	list, sel, row := &a.unstagedList, &a.unstagedSel, &a.unstagedRow
	if staged {
		list, sel, row = &a.stagedList, &a.stagedSel, &a.stagedRow
	}
	// The list chooses rows; the app, changes, by their indices. A tree's
	// rows are folders too, which are no change.
	key := map[bool]string{true: "staged", false: "changes"}[staged]
	rows := a.shownRows(staged)
	items := rows
	if items == nil {
		items = make([]int, len(changes))
		for i := range items {
			items[i] = i
		}
	}
	tree, n, at := a.listRows(key, items, func(i int) string { return changes[i].Path })
	if tree != nil {
		*row = treeRowOf(tree, *sel)
	} else {
		*row = rowOf(rows, *sel)
	}
	// A fixed height, so the header doesn't move as its button comes and
	// goes; the button stays while git runs, doing nothing then.
	head := ui.Row(c).Height(32).Padding(4, 8, 0, 10).Gap(6).AlignItems(ui.Center)
	if !staged && !fill && len(a.staged) > 0 {
		// A line between the staged changes above and these, as a split
		// draws one between them filled.
		head.BorderWidth(1, 0, 0, 0).BorderColor(t.Border)
	}
	head.Children(func() {
		ui.Text(c, title).Bold().TextColor(t.TextMuted)
		count := fmt.Sprint(len(changes))
		if rows != nil {
			count = fmt.Sprintf("%d/%d", len(items), len(changes))
		}
		ui.Badge(c, count)
		ui.Box(c).Grow(1)
		if len(items) > 0 {
			treeToggle(c)
			idle := a.busy == ""
			if staged {
				if iconButton(c, "minus", loc.UnstageAll()) && idle {
					a.unstageAll()
				}
			} else if iconButton(c, "plus", loc.StageAll()) && idle {
				a.stageAll()
			}
		}
	})
	l := ui.List(c.Key(title), list, n, func(i int) {
		if at(i) < 0 {
			a.folderRow(c, key, tree[i])
			return
		}
		ch := changes[at(i)]
		chosen := i == *row
		// As VS Code's: the name, its folder, the actions while the
		// pointer is over the row, and the change's letter at the end;
		// in a tree, the name alone, in from its folder's.
		indent := float32(10)
		if tree != nil {
			indent = 22 + float32(tree[i].depth)*treeIndent
		}
		row := ui.Row(c).Height(24).Padding(0, 8, 0, indent).Gap(6).AlignItems(ui.Center)
		if chosen {
			row.TextColor(t.AccentText)
		}
		row.Children(func() {
			code := ch.Work
			if staged {
				code = ch.Staged
			}
			name, dir := path.Base(ch.Path), path.Dir(ch.Path)
			nameText := ui.Text(c, name).SingleLine().Shrink(0)
			if !chosen && isGenerated(ch.Path, a.genMarked, nil, nil) {
				nameText.TextColor(t.TextMuted)
			}
			muted := t.TextMuted
			if chosen {
				muted = t.AccentText.Alpha(0.75)
			}
			if dir != "." && tree == nil {
				ui.Text(c, dir).FontSize(11).TextColor(muted).SingleLine().Grow(1).Basis(0)
			} else {
				ui.Box(c).Grow(1)
			}
			if row.Hovered() || chosen {
				if iconButton(c, "open", loc.OpenFile()) {
					a.openInEditor(a.repo.Path(ch.Path), 0)
				}
				if !staged && !ch.Conflict && iconButton(c, "undo", loc.DiscardChanges()) {
					a.discard(ch)
				}
				icon, label := "plus", loc.Stage()
				if staged {
					icon, label = "minus", loc.Unstage()
				}
				if iconButton(c, icon, label) {
					a.toggleStage(ch, staged)
				}
			}
			letter := string(code)
			if code == '?' {
				letter = "U" // untracked, as VS Code says it
			}
			ui.Text(c, letter).Font("monospace").FontSize(11).Bold().TextColor(changeColor(t, code, chosen)).
				Width(14).TextAlign(ui.Center).Tooltip(changeName(code))
		})
		row.ContextMenu(func(m *ui.Menu) {
			if m.Item(map[bool]string{true: loc.Unstage(), false: loc.Stage()}[staged]).Chosen() {
				a.toggleStage(ch, staged)
			}
			if !staged && !ch.Conflict && m.Item(loc.DiscardMenu()).Chosen() {
				a.discard(ch)
			}
			m.Separator()
			if ch.Work != '?' {
				a.fileMenu(m, ch.Path, "WORKTREE")
			} else {
				a.ignoreItems(m, ch.Path)
			}
		})
	})
	if fill {
		l.Grow(1).Basis(0)
	} else {
		l.Height(float32(n * 24))
	}
	folder := *row >= 0 && *row < n && at(*row) < 0
	if (l.Changed() || l.Submitted()) && folder {
		a.toggleFolder(key, tree[*row].dir)
	} else if l.Changed() && *row >= 0 && *row < n {
		// One change is chosen at a time, of both lists.
		a.chooseChange(at(*row), staged)
	} else if l.Submitted() && *row >= 0 && *row < n {
		a.toggleStage(changes[at(*row)], staged)
	}
	// The keys of VS Code's and Finder's lists: space stages or unstages
	// the change chosen, delete discards it, asking first.
	chosen := func(fn func(git.Change)) func() {
		return func() {
			if *sel >= 0 && *sel < len(changes) && a.busy == "" {
				fn(changes[*sel])
			}
		}
	}
	list.Handle.OnShortcut(c, 0, ui.KeySpace, chosen(func(ch git.Change) { a.toggleStage(ch, staged) }))
	if !staged {
		discard := chosen(func(ch git.Change) {
			if !ch.Conflict {
				a.discard(ch)
			}
		})
		list.Handle.OnShortcut(c, 0, ui.KeyBackspace, discard)
		list.Handle.OnShortcut(c, 0, ui.KeyDelete, discard)
	}
}

func (a *app) toggleStage(ch git.Change, staged bool) {
	if staged {
		a.unstage(ch)
	} else {
		a.stage(ch)
	}
}

// changeColor colors a change's letter as VS Code does, white on a
// chosen row's accent.
func changeColor(t *ui.Theme, code byte, chosen bool) ui.Color {
	if chosen {
		return t.AccentText
	}
	// VS Code's colors of its dark themes, and of its light ones.
	pick := func(dark, light ui.Color) ui.Color {
		if t.Dark {
			return dark
		}
		return light
	}
	switch code {
	case 'A', '?', 'R', 'C':
		return pick(ui.RGB(115, 201, 145), ui.RGB(88, 124, 12))
	case 'D':
		return pick(ui.RGB(199, 78, 57), ui.RGB(173, 7, 7))
	case 'U':
		return pick(ui.RGB(228, 103, 107), ui.RGB(173, 7, 7))
	}
	return pick(ui.RGB(226, 192, 141), ui.RGB(137, 85, 3)) // modified
}

func changeName(code byte) string {
	switch code {
	case 'M':
		return loc.StatusM()
	case 'A':
		return loc.StatusA()
	case 'D':
		return loc.StatusD()
	case 'R':
		return loc.StatusR()
	case 'C':
		return loc.StatusC()
	case '?':
		return loc.StatusU()
	case 'U':
		return loc.StatusConflict()
	case 'T':
		return loc.StatusT()
	}
	return ""
}

// detailView is the diff of what was chosen last: a commit, with its files
// beside, or a change.
func (a *app) detailView(c *ui.Context) {
	t := c.Theme()
	d := &a.detail
	ui.Column(c).Fill().BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
		switch {
		case d.commit != nil:
			if d.pr != nil {
				a.pullHeader(c, d.pr)
			} else {
				a.commitHeader(c, d.commit)
			}
			ui.Split(c.Key("files"), &a.filesW, func() { a.filesPane(c) }, func() { a.diffView(c) }).Grow(1)
		case d.blame != nil:
			a.blameView(c, d.blame)
		case d.merge != nil:
			a.mergeView(c, d.merge)
		case d.change != nil && d.change.Conflict:
			ui.Column(c).Fill().Center().Children(func() { ui.Spinner(c).Size(18, 18) })
		case d.change != nil:
			ui.Row(c).Padding(8, 12).Gap(8).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
				ui.Text(c, d.change.Path).Bold().SingleLine()
				side := loc.SideWorktree()
				if d.staged {
					side = loc.SideStaged()
				}
				ui.Badge(c, side)
				ui.Box(c).Grow(1)
				a.changeNav(c)
				if d.whole && d.diff != nil && ui.Toggle(c, &prefs.FoldUnchanged, loc.FoldUnchanged()).Changed() {
					d.diff.ExpandUnchanged = !prefs.FoldUnchanged
					prefs.save()
				}
				a.lineButtons(c)
			})
			a.diffView(c)
		default:
			ui.Column(c).Fill().Center().Children(func() {
				ui.Text(c, loc.DetailEmpty()).TextColor(t.TextMuted)
			})
		}
	})
}

func (a *app) commitHeader(c *ui.Context, cm *git.Commit) {
	t := c.Theme()
	d := &a.detail
	ui.Column(c).Padding(8, 12).Gap(3).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		ui.Text(c, cm.Subject).Bold().SingleLine()
		if comparing(cm) {
			ui.Text(c, loc.FilesDiffer(len(d.files))).TextColor(t.TextMuted)
			return
		}
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, cm.Short()).Font("monospace").TextColor(t.Accent).Tooltip(cm.Hash)
			ui.Text(c, cm.Author).TextColor(t.TextMuted)
			ui.Text(c, cm.Time.Format("2006-01-02 15:04")).TextColor(t.TextMuted)
			ui.Box(c).Grow(1)
			a.changeNav(c)
			if n := len(cm.Parents); n > 1 {
				labels := make([]string, n)
				for i, p := range cm.Parents {
					labels[i] = loc.ParentN(i+1, p[:8])
				}
				label := labels[d.parent]
				if ui.Select(c.Key("parent"), &label, labels).Label(loc.ParentLabel()).Changed() {
					a.showParent(slices.Index(labels, label))
				}
			}
		})
		if body := strings.TrimSpace(strings.TrimPrefix(d.message, cm.Subject)); body != "" {
			ui.Text(c, body).TextColor(t.TextMuted).Wrap().MaxLines(6)
		}
	})
}

// fileMenu is a file's context menu, of a commit's files or the changes.
func (a *app) fileMenu(m *ui.Menu, path, rev string) {
	if m.Item(loc.OpenInEditor()).Chosen() {
		a.openInEditor(a.repo.Path(path), 0)
	}
	if m.Item(loc.FileHistory()).Chosen() {
		a.setScope(git.Scope{Ref: a.scope.Ref, Path: path})
	}
	if m.Item(loc.Blame()).Chosen() {
		a.showBlame(rev, path)
	}
	m.Separator()
	if m.Item(loc.CopyPath()).Chosen() {
		mygo.Clipboard.WriteText(path)
	}
	if a.local() && m.Item(loc.ShowInFolder()).Chosen() {
		mygo.Shell.ShowItemInFolder(a.repo.Path(path))
	}
}

func (a *app) filesPane(c *ui.Context) {
	t := c.Theme()
	files := a.detail.files
	ui.Column(c).Fill().Children(func() {
		if len(files) > 1 {
			ui.Row(c).Height(26).Padding(0, 10).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
				ui.Text(c, loc.ViewedCount(a.viewedCount(), len(files))).FontSize(11).TextColor(t.TextMuted).Grow(1)
				treeToggle(c)
			})
		}
		a.filesList(c)
	})
}

func (a *app) filesList(c *ui.Context) {
	t := c.Theme()
	files := a.detail.files
	items := make([]int, len(files))
	for i := range items {
		items[i] = i
	}
	tree, n, at := a.listRows("files", items, func(i int) string { return fileName(files[i]) })
	a.fileRow = a.detail.fileSel
	if tree != nil {
		a.fileRow = treeRowOf(tree, a.detail.fileSel)
	}
	l := ui.List(c, &a.fileList, n, func(r int) {
		if at(r) < 0 {
			a.folderRow(c, "files", tree[r])
			return
		}
		i := at(r)
		f := files[i]
		code, name := byte('M'), f.NewName
		switch {
		case f.OldName == "":
			code = 'A'
		case f.NewName == "":
			code, name = 'D', f.OldName
		case f.OldName != f.NewName:
			code = 'R'
		}
		rev := ""
		if a.detail.commit != nil {
			rev = a.detail.commit.Hash
		}
		viewed, gen := a.isViewed(i), a.detail.generated[i]
		indent := float32(4)
		if tree != nil {
			indent = 16 + float32(tree[r].depth)*treeIndent
		}
		row := ui.Row(c).Height(24).Padding(0, 8, 0, indent).Gap(6).AlignItems(ui.Center).Tooltip(name)
		row.ContextMenu(func(m *ui.Menu) {
			if m.Item(map[bool]string{true: loc.MarkUnviewed(), false: loc.MarkViewed()}[viewed]).Chosen() {
				a.toggleViewed(i)
			}
			m.Separator()
			a.fileMenu(m, name, rev)
		}).Children(func() {
			chosen := i == a.detail.fileSel
			muted := t.TextMuted
			if chosen {
				muted = t.AccentText.Alpha(0.75)
			}
			// Viewed, as GitHub's checkbox: a check, or a ring while the
			// pointer is over the row.
			mark, label := "check", loc.MarkUnviewed()
			if !viewed {
				mark, label = "circle", loc.MarkViewed()
			}
			if viewed || row.Hovered() || chosen {
				if iconButton(c, mark, label) {
					a.toggleViewed(i)
				}
			} else {
				ui.Box(c).Size(22, 22)
			}
			nameText := ui.Text(c, path.Base(name)).SingleLine().Shrink(0)
			if (viewed || gen) && !chosen {
				nameText.TextColor(t.TextMuted)
			}
			if dir := path.Dir(name); dir != "." && tree == nil {
				ui.Text(c, dir).FontSize(11).TextColor(muted).SingleLine().Grow(1).Basis(0)
			} else {
				ui.Box(c).Grow(1)
			}
			if gen {
				ui.Text(c, loc.GeneratedTag()).FontSize(10).TextColor(muted).Shrink(0)
			}
			ui.Text(c, string(code)).Font("monospace").FontSize(11).Bold().TextColor(changeColor(t, code, chosen)).
				Width(14).TextAlign(ui.Center).Tooltip(changeName(code))
		})
	}).Grow(1).Basis(0)
	if r := a.fileRow; l.Changed() && r >= 0 && r < n {
		if at(r) < 0 {
			a.toggleFolder("files", tree[r].dir)
		} else {
			a.detail.fileSel = at(r)
			a.showFile(a.detail.fileSel)
		}
	}
	// Space marks the file chosen viewed and goes on to the next, as
	// GitHub's review does with its checkbox.
	a.fileList.Handle.OnShortcut(c, 0, ui.KeySpace, func() { a.toggleViewed(a.detail.fileSel) })
}

func (a *app) diffView(c *ui.Context) {
	t := c.Theme()
	d := &a.detail
	switch {
	case d.loading:
		ui.Column(c).Fill().Center().Children(func() { ui.Spinner(c).Size(18, 18) })
	case d.images != nil:
		a.imageView(c, d.images)
	case d.folded:
		var p *editor.FilePatch
		if d.commit != nil && d.fileSel >= 0 && d.fileSel < len(d.files) {
			p = &d.files[d.fileSel]
		} else {
			p = d.patch
		}
		a.foldedView(c, p, a.unfold)
	case d.binary, d.commit != nil && d.fileSel >= 0 && d.fileSel < len(d.files) && d.files[d.fileSel].Binary:
		ui.Column(c).Fill().Center().Children(func() { ui.Text(c, loc.BinaryFile()).TextColor(t.TextMuted) })
	case d.diff != nil:
		a.reviewDiff(c, d.diff)
		ui.Row(c).Fill().Children(func() {
			dv := editor.DiffView(c.Key(d.gen*1000+d.fileSel), d.diff).Grow(1).Basis(0).AlignSelf(ui.Stretch)
			if d.whole {
				dv.OnShortcut(ui.Cmd, ui.KeyS, a.saveFile) // as the file is typed in
			}
			overview(c, d.diff)
		})
	default:
		ui.Column(c).Fill().Center().Children(func() { ui.Text(c, loc.NoDiff()).TextColor(t.TextMuted) })
	}
}

// refLabel is how a ref shows: a tag without its "tag: ".
func refLabel(ref string) (string, bool) {
	if tag, ok := strings.CutPrefix(ref, "tag: "); ok {
		return tag, true
	}
	return ref, false
}

// lineButtons act on the lines selected in the diff of a change.
func (a *app) lineButtons(c *ui.Context) {
	if ui.Button(c, loc.OpenInEditor()).Clicked() {
		a.openChangeInEditor()
	}
	if a.detail.whole {
		if ui.PrimaryButton(c, loc.Save()).Disabled(!a.unsaved()).Tooltip(shortcut("S")).Clicked() {
			a.saveFile()
		}
		c.OnShortcut(ui.Cmd, ui.KeyS, a.saveFile)
	}
	if a.detail.patch == nil || !patch.Partial(*a.detail.patch) || prefs.IgnoreWhitespace {
		return
	}
	chosen := a.selectedLines()
	none := chosen == nil || a.busy != "" || a.unsaved()
	if a.detail.staged {
		if ui.Button(c, loc.UnstageLines()).Disabled(none).Clicked() {
			a.applyLines(chosen, lineUnstage)
		}
		return
	}
	if ui.Button(c, loc.DiscardLines()).Disabled(none).Clicked() {
		a.applyLines(chosen, lineDiscard)
	}
	if ui.Button(c, loc.StageLines()).Disabled(none).Tooltip(loc.StageLinesTooltip()).Clicked() {
		a.applyLines(chosen, lineStage)
	}
}
