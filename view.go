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
		a.undoBar(c)
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
		// The sidebar is left out rather than narrowed: a split keeps a
		// strip of its own, with a divider to drag, however small its size.
		history := func() {
			ui.SplitVertical(c, &a.logH, func() { a.logView(c) }, func() { a.detailView(c) }).Fill()
		}
		if prefs.SidebarHidden {
			ui.Column(c).Grow(1).Basis(0).Children(history)
		} else {
			ui.Split(c, &a.sidebarW, func() {
				if a.ws != nil {
					a.ws.sidebar(c)
				} else {
					a.leftPanel(c)
				}
			}, history).Grow(1)
		}
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
	a.identityView(c)
	a.settingsView(c)
	a.confirmView(c)
	a.cred.view(c)
	a.paletteView(c)
	a.composerView(c)
	a.pullsView(c)
	a.submitView(c)
	c.OnShortcut(ui.Cmd, ui.KeyR, a.refresh)
	c.OnShortcut(ui.Cmd, ui.KeyB, a.toggleSidebar)
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
		// Where the sidebar meets the window, so that its way back stays
		// in view while it is away.
		if iconButton(c, "panel-left", a.sidebarLabel(), a.sidebarLabel()+" · "+shortcut("B")) {
			a.toggleSidebar()
		}
		if a.ws != nil {
			ui.Text(c, a.ws.name()+" /").FontSize(13).TextColor(t.TextMuted)
		}
		a.projectButton(c)
		if !a.local() {
			a.remoteBadge(c)
		}
		branch := st.Branch
		if branch == "" {
			branch = loc.Detached()
		}
		a.branchesButton(c, "⎇ "+branch+" ▾")
		if st.Upstream != "" {
			a.upstreamCounts(c, st)
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
		// The commits there are to pull and to push, on the buttons that
		// would (#30).
		pullText, pushText := loc.Pull(), loc.PushMenu()
		if st.Behind > 0 {
			pullText = loc.PullBehind(st.Behind)
		}
		if st.Ahead > 0 {
			pushText = loc.PushMenuAhead(st.Ahead)
		}
		pullTip := loc.Pull()
		if st.Behind > 0 {
			pullTip = loc.CommitsToPull(st.Behind)
		}
		ui.Button(c, pullText+" ▾").Label(loc.Pull()).Tooltip(pullTip).Disabled(!idle || st.Upstream == "").Menu(func(m *ui.Menu) {
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
		pushTip := loc.PushTooltip()
		if st.Ahead > 0 {
			pushTip = loc.CommitsToPush(st.Ahead) + " · " + pushTip
		}
		if ui.Button(c, pushText).Label(loc.PushMenu()).Tooltip(pushTip).Disabled(!idle || st.Branch == "").Clicked() {
			a.openPush()
		}
		a.commentsButton(c)
		if ui.Button(c, loc.Refresh()).Tooltip(shortcut("R")).Clicked() {
			a.refresh()
		}
		ui.Button(c, "⋯").Label(loc.Repository()).Menu(a.repoMenu)
	})
}

// upstreamCounts tells how the branch stands against its upstream: the
// commits to push and to pull, colored and with what they mean, and left
// out when none (#39).
func (a *app) upstreamCounts(c *ui.Context, st git.Status) {
	t := c.Theme()
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, st.Upstream).FontSize(12).TextColor(t.TextMuted).SingleLine()
		if st.Ahead > 0 {
			ui.Text(c, fmt.Sprintf("↑%d", st.Ahead)).FontSize(12).Bold().TextColor(t.Accent).
				Tooltip(loc.CommitsToPush(st.Ahead))
		}
		if st.Behind > 0 {
			ui.Text(c, fmt.Sprintf("↓%d", st.Behind)).FontSize(12).Bold().TextColor(t.Warning).
				Tooltip(loc.CommitsToPull(st.Behind))
		}
	})
}

// toggleSidebar hides the sidebar, or shows it again, both the repository's
// and the workspace's, keeping the choice for the next launch.
func (a *app) toggleSidebar() {
	prefs.SidebarHidden = !prefs.SidebarHidden
	prefs.save()
}

// sidebarLabel says what the toggle does now, as its button and the
// commands name it.
func (a *app) sidebarLabel() string {
	if prefs.SidebarHidden {
		return loc.ShowSidebar()
	}
	return loc.HideSidebar()
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
				// Only with a model set: without one, nothing of AI shows.
				if prefs.AI.on() && generateButton(c, !a.canGenerateMessage()) {
					a.generateMessage()
				}
			})
			a.commitButtons(c)
		})
		if len(a.staged)+len(a.unstaged) > 0 || a.filter.query != "" {
			a.filterField(c)
		}
		if prefs.OneList { // all in one, as IDEA's (#34)
			ui.Column(c).Grow(1).Basis(0).Children(func() { a.changeSection(c, loc.Changes(), false, true) })
			return
		}
		// Dragged as the room the staged changes need, few or thousands.
		ui.SplitVerticalPercent(c.Key("changes"), &a.stagedPct, func() {
			ui.Column(c).Fill().Children(func() { a.changeSection(c, loc.Staged(), true, true) })
		}, func() {
			ui.Column(c).Fill().Children(func() { a.changeSection(c, loc.Changes(), false, true) })
		}).Grow(1).Basis(0)
	})
	c.OnShortcut(ui.Cmd, ui.KeyEnter, a.commit)
	c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyEnter, a.commitAndPush)
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
	changes, rows := a.listed(staged)
	one := a.oneList(staged)
	list, sel, row := &a.unstagedList, &a.unstagedSel, &a.unstagedRow
	if staged {
		list, sel, row = &a.stagedList, &a.stagedSel, &a.stagedRow
	}
	if one { // the change shown, of whichever list
		at := -1
		if ch := a.detail.change; ch != nil {
			at = indexOf(changes, ch.Path)
		}
		sel = &at
	}
	// The list chooses rows; the app, changes, by their indices. A tree's
	// rows are folders too, which are no change.
	key := map[bool]string{true: "staged", false: "changes"}[staged]
	items := rows
	if items == nil {
		items = make([]int, len(changes))
		for i := range items {
			items[i] = i
		}
	}
	tree, n, group, at := a.changeRows(key, changes, items, staged)
	// The rows below the untracked files' group are theirs: their folders
	// fold, and act on their files, apart from the others'.
	inGroup := func(r int) bool { return group >= 0 && r > group }
	foldKey := func(r int) string {
		if inGroup(r) {
			return key + untrackedGroup
		}
		return key
	}
	itemsOf := func(r int) []int {
		if group < 0 {
			return items
		}
		return slices.DeleteFunc(slices.Clone(items), func(i int) bool { return isNewFile(changes[i], staged) != inGroup(r) })
	}
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
		if len(items) > 0 && prefs.TreeView {
			if folders := changeFolders(key, changes, items, staged); len(folders) > 0 {
				a.foldAllButton(c, folders)
			}
		}
		// The way to a tree shows while the list is empty too, so that it
		// is found before the files come (#31); a workspace's repositories,
		// stacked, keep theirs to those with files.
		if len(items) > 0 || fill {
			treeToggle(c)
		}
		if len(items) > 0 && iconButton(c, "rows", loc.ShowAllChanges()) {
			a.showAll(staged) // one after another, to read them through (#44)
		}
		if len(items) > 0 {
			idle := a.busy == ""
			switch {
			case one:
				// A check of the commit's, not a staging (#37): all of the
				// list in it, or none.
				all := a.allInCommit()
				icon, label := "check", loc.SelectAllChanges()
				if all {
					icon, label = "minus", loc.DeselectAllChanges()
				}
				if iconButton(c, icon, label) && idle {
					a.chooseAllCommits(!all)
				}
				if iconButton(c, "undo", loc.DiscardAll()) && idle {
					a.discardAll()
				}
			case staged:
				if len(a.staged) > 0 {
					if iconButton(c, "minus", loc.UnstageAll()) && idle {
						a.unstageAll()
					}
				}
			default:
				if len(a.unstaged) > 0 {
					if iconButton(c, "undo", loc.DiscardAll()) && idle {
						a.discardAll()
					}
					if iconButton(c, "plus", loc.StageAll()) && idle {
						a.stageAll()
					}
				}
			}
		}
	})
	l := ui.List(c.Key(title), list, n, func(i int) {
		if i == group {
			a.untrackedRow(c, key, changes, itemsOf(i+1))
			return
		}
		if at(i) < 0 {
			// A folder, with the actions of its files while the pointer
			// is over it, as VS Code's changes tree: to stage or unstage
			// what is in it, or to discard it.
			folder, items := tree[i].dir, itemsOf(i)
			row := a.folderRow(c, foldKey(i), tree[i])
			if row.Hovered() {
				row.Children(func() {
					if one {
						paths := folderPaths(changes, items, folder)
						if iconButton(c, "check", loc.IncludeInCommitMenu()) {
							a.choosePathsCommits(true, paths...)
						}
						if iconButton(c, "undo", loc.DiscardFolder()) {
							if chosen := folderChanges(changes, items, folder); len(chosen) > 0 {
								a.discardFolder(folder, chosen)
							}
						}
						if iconButton(c, "minus", loc.ExcludeFromCommitMenu()) {
							a.choosePathsCommits(false, paths...)
						}
						return
					}
					if staged {
						if iconButton(c, "minus", loc.UnstageFolder()) {
							a.unstagePaths(folderPaths(changes, items, folder)...)
						}
						return
					}
					if iconButton(c, "undo", loc.DiscardFolder()) {
						if chosen := folderChanges(changes, items, folder); len(chosen) > 0 {
							a.discardFolder(folder, chosen)
						}
					}
					if iconButton(c, "plus", loc.StageFolder()) {
						a.stagePaths(folderPaths(changes, items, folder)...)
					}
				})
			}
			if !staged {
				row.ContextMenu(func(m *ui.Menu) {
					if one {
						paths := folderPaths(changes, items, folder)
						if m.Item(loc.IncludeInCommitMenu()).Chosen() {
							a.choosePathsCommits(true, paths...)
						}
						if m.Item(loc.ExcludeFromCommitMenu()).Chosen() {
							a.choosePathsCommits(false, paths...)
						}
						m.Separator()
					}
					chosen := folderChanges(changes, items, folder)
					if len(chosen) > 0 && m.Item(loc.DiscardFolderMenu()).Chosen() {
						a.discardFolder(folder, chosen)
					}
				})
			}
			return
		}
		ch := changes[at(i)]
		chosen := i == *row
		// As VS Code's: the name, its folder, the actions while the
		// pointer is over the row, and the change's letter at the end;
		// in a tree, the name alone, in from its folder's. A check box
		// leads the row, as IDEA's commit dialog: checked when the
		// change is staged, so a click stages or unstages it.
		indent := float32(8)
		if tree != nil {
			indent = 6 + float32(tree[i].depth)*treeIndent
			if !prefs.TreeView { // a list's, under the untracked files' group or not
				indent += 2
			}
		}
		row := ui.Row(c).Height(24).Padding(0, 8, 0, indent).Gap(6).AlignItems(ui.Center)
		if chosen {
			row.TextColor(t.AccentText)
		}
		// In the one list, the box says whether the commit includes the
		// change, and checking it stages nothing (#37).
		on, partly := stagedIn(ch, staged), false
		if one {
			on = a.inCommit(ch)
		}
		row.Children(func() {
			label := loc.StageFile(ch.Path)
			if on {
				label = loc.UnstageFile(ch.Path)
			}
			if one {
				label = loc.IncludeInCommit(ch.Path)
				if on {
					label = loc.ExcludeFromCommit(ch.Path)
				}
			}
			if changeCheckbox(c, on, partly, chosen, label) {
				if one {
					a.toggleCommit(ch)
				} else {
					a.toggleStage(ch, on)
				}
			}
			code := ch.Work
			if staged || one && ch.Staged != 0 { // in the one list, as against HEAD
				code = ch.Staged
			}
			name, dir := path.Base(ch.Path), path.Dir(ch.Path)
			// The name in its change's color too, as VS Code's and IDEA's
			// (#29); a generated file's stays muted, as wanted the less.
			nameText := ui.Text(c, name).SingleLine().Shrink(0)
			if !chosen && isGenerated(ch.Path, a.genMarked, nil, nil) {
				nameText.TextColor(t.TextMuted)
			} else if !chosen {
				nameText.TextColor(changeColor(t, code, false))
			}
			muted := t.TextMuted
			if chosen {
				muted = t.AccentText.Alpha(0.75)
			}
			if dir != "." && !prefs.TreeView {
				ui.Text(c, dir).FontSize(11).TextColor(muted).SingleLine().Grow(1).Basis(0)
			} else {
				ui.Box(c).Grow(1)
			}
			if row.Hovered() || chosen {
				if iconButton(c, "open", loc.OpenFile()) {
					a.openInEditor(a.repo.Path(ch.Path), 0)
				}
				if (one || !on) && !ch.Conflict && iconButton(c, "undo", loc.DiscardChanges()) {
					a.discard(ch)
				}
				if !one {
					icon, label := "plus", loc.Stage()
					if on {
						icon, label = "minus", loc.Unstage()
					}
					if iconButton(c, icon, label) {
						a.toggleStage(ch, on)
					}
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
			if one {
				if m.Item(map[bool]string{true: loc.ExcludeFromCommitMenu(), false: loc.IncludeInCommitMenu()}[on]).Chosen() {
					a.toggleCommit(ch)
				}
			} else if m.Item(map[bool]string{true: loc.Unstage(), false: loc.Stage()}[on]).Chosen() {
				a.toggleStage(ch, on)
			}
			if (one || !on) && !ch.Conflict && m.Item(loc.DiscardMenu()).Chosen() {
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
		a.toggleFolder(foldKey(*row), tree[*row].dir)
	} else if l.Changed() && *row >= 0 && *row < n {
		// One change is chosen at a time, of both lists.
		a.chooseListed(changes[at(*row)], staged)
	} else if l.Submitted() && *row >= 0 && *row < n {
		ch := changes[at(*row)]
		if one {
			a.toggleCommit(ch)
		} else {
			a.toggleStage(ch, stagedIn(ch, staged))
		}
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
	list.Handle.OnShortcut(c, 0, ui.KeySpace, chosen(func(ch git.Change) {
		if one {
			a.toggleCommit(ch)
		} else {
			a.toggleStage(ch, stagedIn(ch, staged))
		}
	}))
	if !staged {
		discard := chosen(func(ch git.Change) {
			if !ch.Conflict && (one || ch.Work != 0) { // the one list's are all against HEAD
				a.discard(ch)
			}
		})
		list.Handle.OnShortcut(c, 0, ui.KeyBackspace, discard)
		list.Handle.OnShortcut(c, 0, ui.KeyDelete, discard)
	}
}

// untrackedRow is the row the untracked files show under, items of
// changes: a chevron that folds them, how many, and while the pointer is
// over it, the actions of them all.
func (a *app) untrackedRow(c *ui.Context, key string, changes []git.Change, items []int) {
	t := c.Theme()
	icon := map[bool]string{true: "chevron-right", false: "chevron-down"}[a.folded(key, untrackedGroup)]
	row := ui.Row(c).Height(24).Padding(0, 8, 0, 6).Gap(6).AlignItems(ui.Center)
	row.Children(func() {
		ui.Icon(c, icons[icon]).Size(12, 12).TextColor(t.TextMuted)
		ui.Text(c, newFilesName()).SingleLine().TextColor(t.TextMuted)
		ui.Badge(c, fmt.Sprint(len(items)))
		ui.Box(c).Grow(1)
	})
	if !row.Hovered() {
		return
	}
	paths := make([]string, len(items))
	for i, it := range items {
		paths[i] = changes[it].Path
	}
	row.Children(func() {
		if iconButton(c, "undo", loc.DiscardUntracked()) && a.busy == "" {
			var chosen []git.Change // the one list's added files are kept
			for _, it := range items {
				if changes[it].Work == '?' {
					chosen = append(chosen, changes[it])
				}
			}
			a.ask(loc.DiscardUntrackedTitle(), loc.DiscardUntrackedMessage(len(chosen)), loc.Discard(), func() {
				a.run(loc.OpDiscard(), func(ctx context.Context) error { return a.discardChanges(ctx, chosen) }, nil)
			})
		}
		if prefs.OneList {
			if iconButton(c, "check", loc.IncludeInCommitMenu()) && a.busy == "" {
				a.choosePathsCommits(true, paths...)
			}
		} else if iconButton(c, "plus", loc.StageUntracked()) && a.busy == "" {
			a.stagePaths(paths...)
		}
	})
}

// changeCheckbox is the check box that leads a change's row: checked
// when the change is staged, as the commit dialog of IDEA checks the
// changes to commit, and a click stages it or unstages it. A chosen row
// is white on the accent, so the box turns with it.
func changeCheckbox(c *ui.Context, staged, partly, chosen bool, label string) bool {
	t := c.Theme()
	on := staged || partly // the click flips it; the change itself is staged after
	box := ui.CheckboxBase(c, &on).Size(14, 14).Radius(3).Center().Shrink(0).
		Label(label).Tooltip(label).Cursor(ui.CursorPointer)
	mark, ink := t.Accent, t.AccentText
	if chosen {
		mark, ink = t.AccentText, t.Accent
	}
	if on {
		box.Background(mark)
	} else {
		box.Border(1, t.Border.Mix(t.Text, 0.25))
		if box.Hovered() || chosen {
			box.BorderColor(mark)
		}
	}
	box.Children(func() {
		if on {
			ui.Icon(c, icons[map[bool]string{true: "minus", false: "check"}[partly]]).Size(10, 10).TextColor(ink)
		}
	})
	return box.Changed()
}

// stagedIn tells whether a change of a section reads staged: all of the
// staged changes do, and of the one list those with nothing more in the
// working tree. The changes not staged otherwise all have some.
func stagedIn(ch git.Change, staged bool) bool { return staged || ch.Work == 0 }

// chooseListed chooses the change of a section's row, in the one list of
// the staged changes when it reads staged, else of the working tree's.
func (a *app) chooseListed(ch git.Change, staged bool) {
	if prefs.OneList { // the one list always shows the working tree's side
		a.chooseChange(indexOf(a.status.Changes, ch.Path), false)
		return
	}
	s := stagedIn(ch, staged)
	a.chooseChange(indexOf(a.changes(s), ch.Path), s)
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
		case d.all != nil:
			ui.Row(c).Padding(8, 12).Gap(8).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
				ui.Text(c, loc.AllChangesTitle(len(d.all.files))).Bold().SingleLine()
				side := loc.SideWorktree()
				if d.all.staged {
					side = loc.SideStaged()
				}
				ui.Badge(c, side)
				ui.Box(c).Grow(1)
				if diffs := a.shownDiffs(); len(diffs) > 0 {
					wrapToggle(c, diffs...)
				}
				a.whitespaceMenu(c)
			})
			a.diffView(c)
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
				a.foldToggle(c)
				a.popButton(c)
				if d.whole && d.diff != nil && ui.Toggle(c, &prefs.FoldUnchanged, loc.FoldUnchanged()).Changed() {
					d.diff.ExpandUnchanged = !prefs.FoldUnchanged
					prefs.save()
				}
				if d.diff != nil {
					wrapToggle(c, d.diff)
				}
				a.whitespaceMenu(c)
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
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, loc.FilesDiffer(len(d.files))).TextColor(t.TextMuted)
				ui.Box(c).Grow(1)
				a.foldToggle(c)
				a.popButton(c)
				a.whitespaceMenu(c)
				if diffs := a.shownDiffs(); len(diffs) > 0 {
					wrapToggle(c, diffs...)
				}
			})
			return
		}
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, cm.Short()).Font("monospace").TextColor(t.Accent).Tooltip(cm.Hash)
			ui.Text(c, cm.Author).TextColor(t.TextMuted)
			ui.Text(c, cm.Time.Format("2006-01-02 15:04")).TextColor(t.TextMuted)
			ui.Box(c).Grow(1)
			a.changeNav(c)
			a.foldToggle(c)
			a.popButton(c)
			a.whitespaceMenu(c)
			if diffs := a.shownDiffs(); len(diffs) > 0 {
				wrapToggle(c, diffs...)
			}
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

// foldToggle folds the unchanged lines of the whole files shown away from
// their changes, and shows them all again, as IntelliJ's button does;
// the choice is kept for the files opened next (#47).
func (a *app) foldToggle(c *ui.Context) {
	d := &a.detail
	if !d.patchWhole || d.diff == nil {
		return
	}
	if ui.Toggle(c, &d.fold, loc.FoldUnchanged()).Changed() {
		d.diff.ExpandUnchanged = !d.fold
		prefs.FoldUnchanged = d.fold
		prefs.save()
	}
}

// whitespaceMenu is a pop-up button over the diff, as IntelliJ's, naming
// what the diffs leave out of whitespace changes and choosing one of the
// modes.
func (a *app) whitespaceMenu(c *ui.Context) {
	b := ui.ButtonBase(c.Key("whitespace")).Padding(3, 6).Gap(2).Radius(5).AlignItems(ui.Center).Shrink(0).
		Tooltip(loc.WhitespaceChanges()).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(c.Theme().Text.Alpha(0.12))
	}
	chosen := prefs.whitespaceMode()
	b.Children(func() {
		ui.Text(c, chosen.label()).SingleLine()
		ui.Icon(c, icons["chevron-down"]).Size(12, 12)
	})
	b.Menu(func(m *ui.Menu) {
		for _, mode := range whitespaceModes {
			if m.Item(mode.label()).Checked(mode.key == chosen.key).Chosen() && mode.key != chosen.key {
				prefs.Whitespace = mode.key
				prefs.save()
				a.reloadDiff()
			}
		}
	})
}

// wrapToggle turns wrapping on or off for the diffs shown, one or those
// one after another, kept for the diffs opened next too.
func wrapToggle(c *ui.Context, diffs ...*editor.DiffState) {
	if ui.Toggle(c, &prefs.Wrap, loc.WrapLines()).Changed() {
		for _, d := range diffs {
			d.Wrap = prefs.Wrap
		}
		prefs.save()
	}
}

// compareTake tells the revision a comparison of two takes a file from,
// the one compared with, when it has the file: a file deleted there has
// none to take (#36).
func compareTake(c *git.Commit, f editor.FilePatch) (string, bool) {
	if c == nil || !comparing(c) {
		return "", false
	}
	_, to, _ := strings.Cut(c.Hash, "..")
	if to == "" || f.NewName == "" {
		return "", false
	}
	return to, true
}

// getFrom overwrites a file of the working tree with the version of a
// revision, as IntelliJ's Get does, asking first when the file has changes
// of its own to lose.
func (a *app) getFrom(rev, path string) {
	get := func() {
		a.run(loc.OpGetFile(path), func(ctx context.Context) error {
			return a.repo.RestoreFrom(ctx, rev, path)
		}, func() { a.notice = loc.GotFile(path, shortRev(rev)) })
	}
	if _, changed := find(a.status.Changes, path); !changed {
		get()
		return
	}
	a.ask(loc.GetFileTitle(path), loc.GetFileMessage(shortRev(rev)), loc.GetFile(), get)
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
		if len(files) > 1 || prefs.CommitAll { // the way back from one after another too
			ui.Row(c).Height(26).Padding(0, 10).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
				ui.Text(c, loc.ViewedCount(a.viewedCount(), len(files))).FontSize(11).TextColor(t.TextMuted).Grow(1)
				a.commitAllToggle(c)
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
			// A comparison takes the file from the branch compared with,
			// as IntelliJ's Get does (#36).
			if take, ok := compareTake(a.detail.commit, f); ok {
				if m.Item(loc.GetFromRev(shortRev(take))).Chosen() {
					a.getFrom(take, name)
				}
				m.Separator()
			}
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
			} else if !chosen {
				nameText.TextColor(changeColor(t, code, false)) // as the changes' (#29)
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
		} else if prefs.CommitAll { // to the file, one after another
			a.detail.fileSel = at(r)
			a.detail.allReveal = at(r) + 1
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
	case d.all != nil:
		a.allView(c, d.all)
	case d.commit != nil && prefs.CommitAll && d.commitAll != nil:
		a.allFiles(c, d.commitAll, func(i int) { // the file alone
			prefs.CommitAll = false
			prefs.save()
			d.fileSel = i
			a.commitAllChanged()
		})
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
		row := ui.Row(c).Fill()
		row.Children(func() {
			dv := editor.DiffView(c.Key(d.gen*1000+d.fileSel), d.diff).Grow(1).Basis(0).AlignSelf(ui.Stretch)
			if d.whole {
				dv.OnShortcut(ui.Cmd, ui.KeyS, a.saveFile) // as the file is typed in
			}
			overview(c, d.diff, row, d.starts, func(i int) int { return a.fileLine(editor.DiffNew, i) })
		})
		a.revealChange()
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
	if a.detail.patch == nil || !patch.Partial(*a.detail.patch) || prefs.whitespace().Any() || prefs.OneList {
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
