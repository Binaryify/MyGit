package main

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mygit/internal/git"
	loc "mygit/locales"
)

// prompt asks for a name, of a branch or a tag, in a dialog.
type prompt struct {
	open                 bool
	title, button, value string
	message, msgVal      string // a second, optional field: a tag's message
	check                string // an option, as a checkbox, when given
	checked              bool
	// must, when given, is the value the field takes before the button
	// goes, which the dialog then says: a branch of a remote goes only when
	// its name is typed again, as a repository does on the web.
	must, note string
	ok         func(value, message string)
}

func (a *app) ask1(title, button, value string, ok func(string)) {
	a.prompt = prompt{open: true, title: title, button: button, value: value,
		ok: func(v, _ string) { ok(v) }}
}

func (a *app) promptView(c *ui.Context) {
	p := &a.prompt
	ui.Modal(c, &p.open, func() {
		ui.Column(c).Gap(12).Width(360).Children(func() {
			ui.Text(c, p.title).Bold().FontSize(14)
			if p.note != "" {
				ui.Text(c, p.note).TextColor(c.Theme().TextMuted).Wrap()
			}
			placeholder := ""
			if p.must != "" {
				placeholder = p.must
			}
			submitted := ui.TextInput(c, &p.value).Label(p.title).Placeholder(placeholder).AutoFocus().Submitted()
			if p.message != "" {
				ui.TextInput(c, &p.msgVal).Placeholder(p.message).Label(p.message)
			}
			if p.check != "" {
				ui.Checkbox(c, &p.checked, p.check)
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, loc.Cancel()).Clicked() {
					p.open = false
				}
				value := strings.TrimSpace(p.value)
				ok := value != "" && (p.must == "" || value == p.must)
				if (ui.PrimaryButton(c, p.button).Disabled(!ok).Clicked() || submitted) && ok {
					p.open = false
					p.value = value
					p.ok(value, strings.TrimSpace(p.msgVal))
				}
			})
		})
	})
}

// branchItems are what a local branch's menu does with it.
func (a *app) branchItems(m *ui.Menu, b string) {
	cur := a.status.Branch
	if b != cur && a.chose(m.Item(loc.BranchSwitch())) {
		a.switchTo(b)
	}
	if b != cur {
		a.mergeItems(m, b)
	}
	if a.chose(m.Item(loc.BranchNewFrom())) {
		a.newBranch(b)
	}
	if a.chose(m.Item(loc.WorktreeNew())) {
		a.newWorktree(b, true)
	}
	if b != cur && a.chose(m.Item(loc.BranchHistory())) { // the current one's is the history's own
		a.setScope(git.Scope{Ref: b})
	}
	m.Separator()
	if a.chose(m.Item(loc.RenameMenu())) {
		a.ask1(loc.BranchRenameTitle(b), loc.Rename(), b, func(to string) {
			a.run(loc.OpRenameBranch(), func(ctx context.Context) error { return a.repo.RenameBranch(ctx, b, to) }, nil)
		})
	}
	if b != cur && a.chose(m.Item(loc.Delete())) {
		a.deleteBranch(b)
	}
}

// remoteItems are what a remote branch's menu does with it.
func (a *app) remoteItems(m *ui.Menu, rb string) {
	if a.chose(m.Item(loc.BranchCheckoutLocal())) {
		a.run(loc.OpCheckout(rb), func(ctx context.Context) error { return a.repo.CheckoutRemote(ctx, rb) }, nil)
	}
	a.mergeItems(m, rb)
	if a.chose(m.Item(loc.BranchNewFrom())) {
		a.newBranch(rb)
	}
	if a.chose(m.Item(loc.WorktreeNew())) {
		a.newWorktree(rb, false)
	}
	m.Separator()
	if a.chose(m.Item(loc.BranchDeleteRemote())) {
		a.askDeleteRemote(rb)
	}
}

// askDeleteRemote asks to delete a remote branch, and takes its name typed
// again to go on: it goes for everyone who fetches the repository, and
// nothing here brings it back, so a plain Delete is too easy to hit.
func (a *app) askDeleteRemote(rb string) {
	a.prompt = prompt{open: true, title: loc.BranchDeleteRemoteTitle(rb), button: loc.Delete(),
		note: loc.BranchDeleteRemoteWarning(rb), must: rb,
		ok: func(string, string) {
			a.run(loc.OpDeleteRemoteBranch(), func(ctx context.Context) error { return a.repo.DeleteRemoteBranch(ctx, rb) }, nil)
		}}
}

// tagItems are what a tag's menu does with it.
func (a *app) tagItems(m *ui.Menu, tag string) {
	if a.chose(m.Item(loc.TagCheckout())) {
		a.run(loc.OpCheckout(tag), func(ctx context.Context) error { return a.repo.CheckoutDetached(ctx, tag) }, nil)
	}
	a.mergeItems(m, tag)
	if a.chose(m.Item(loc.TagNewBranch())) {
		a.newBranch(tag)
	}
	if a.chose(m.Item(loc.WorktreeNew())) {
		a.newWorktree(tag, false)
	}
	if a.chose(m.Item(loc.TagHistory())) {
		a.setScope(git.Scope{Ref: tag})
	}
	if a.chose(m.Item(loc.TagCopyName())) {
		mygo.Clipboard.WriteText(tag)
	}
	m.Separator()
	if a.chose(m.Item(loc.TagPush())) {
		a.run(loc.OpPushTag(tag), func(ctx context.Context) error { return a.repo.PushTag(ctx, tag) }, nil)
	}
	if a.chose(m.Item(loc.DeleteTag(tag))) {
		a.run(loc.OpDeleteTag(tag), func(ctx context.Context) error { return a.repo.DeleteTag(ctx, tag) }, nil)
	}
}

// chose reports the item chosen, closing the branches' popover then, as
// whatever its branches' menus do is done with it.
func (a *app) chose(it *ui.MenuItem) bool {
	if !it.Chosen() {
		return false
	}
	a.branchesOpen = false
	return true
}

func (a *app) switchTo(b string) {
	a.run(loc.OpSwitchTo(b), func(ctx context.Context) error { return a.repo.Switch(ctx, b) }, nil)
}

// branchesButton opens the branches' popover, as IntelliJ's: a search,
// a new branch, and the local and remote branches, the tags and the
// working trees the search matches, each with its menu. Enter switches to
// the first local one.
func (a *app) branchesButton(c *ui.Context, label string) {
	b := ui.Button(c, label).Label(loc.Branches()).Tooltip(loc.BranchesTip())
	if b.Clicked() {
		a.branchesOpen, a.branchFilter = !a.branchesOpen, ""
		if a.branchesOpen {
			a.loadWorktrees()
		}
	}
	ui.Popover(c, b, &a.branchesOpen, func() {
		t := c.Theme()
		q := strings.ToLower(strings.TrimSpace(a.branchFilter))
		match := func(name string) bool { return q == "" || strings.Contains(strings.ToLower(name), q) }
		matching := func(names []string) []string {
			var m []string
			for _, n := range names {
				if match(n) {
					m = append(m, n)
				}
			}
			return m
		}
		local, remote, tags := matching(a.branches), matching(a.remotes), matching(a.tags)
		var trees []git.Worktree
		if len(a.worktrees) > 1 { // the repository's own alone is no news
			for _, w := range a.worktrees {
				if match(path.Base(filepath.ToSlash(w.Dir))) || match(w.Branch) {
					trees = append(trees, w)
				}
			}
		}
		ui.Column(c).Width(320).Gap(4).Children(func() {
			in := ui.TextInput(c, &a.branchFilter).Placeholder(loc.BranchSearch()).Label(loc.BranchSearch()).AutoFocus()
			if in.Submitted() && len(local) > 0 && local[0] != a.status.Branch {
				a.branchesOpen = false
				a.switchTo(local[0])
			}
			if branchRow(c, loc.BranchNew(), false) {
				a.branchesOpen = false
				a.newBranch("")
			}
			ui.Scroll(c).MaxHeight(420).Gap(1).Children(func() {
				// cur is the section's current one, checked: the current branch
				// among the local ones.
				section := func(kind, title string, names []string, cur string, items func(*ui.Menu, string)) {
					if len(names) == 0 || !sectionHeader(c, kind, title, len(names), q != "") {
						return
					}
					for _, n := range names {
						ui.Box(c.Key(title + n)).Children(func() {
							branchMenuRow(c, n, n == cur, func(m *ui.Menu) { items(m, n) })
						})
					}
				}
				section("local", loc.BranchesLocal(), local, a.status.Branch, a.branchItems)
				section("remote", loc.BranchesRemote(), remote, "", a.remoteItems)
				section("tags", loc.BranchesTags(), tags, "", a.tagItems)
				// A submodule's working trees are its own, apart from the
				// superproject's.
				title := loc.BranchesWorktrees()
				if a.super != "" {
					title += " · " + loc.WorktreeSubmodule()
				}
				if len(trees) > 0 && sectionHeader(c, "worktrees", title, len(trees), q != "") {
					for _, w := range trees {
						ui.Box(c.Key("worktree" + w.Dir)).Children(func() {
							current := w.Dir == a.repo.Dir
							items := func(m *ui.Menu) { a.worktreeItems(m, w) }
							branchRowElement(c, path.Base(filepath.ToSlash(w.Dir)), worktreeNote(w), current).Tooltip(w.Dir).
								Menu(items).ContextMenu(items)
						})
					}
				}
				if len(local)+len(remote)+len(tags)+len(trees) == 0 {
					ui.Text(c, loc.BranchesNone()).TextColor(t.TextMuted).Padding(8)
				}
			})
		})
	})
}

// sectionHeader is the title of a section of the branches' popover, with
// how many it lists, which folds or unfolds it, as kept; it reports
// whether the section shows, as it always does while searching.
func sectionHeader(c *ui.Context, kind, title string, n int, searching bool) bool {
	t := c.Theme()
	open := searching || !prefs.FoldedBranches[kind]
	icon := map[bool]string{true: "chevron-down", false: "chevron-right"}[open]
	h := ui.ButtonBase(c.Key("section "+kind)).Padding(8, 8, 2, 4).Gap(4).AlignItems(ui.Center).Justify(ui.Start).
		Label(title).Cursor(ui.CursorPointer)
	h.Children(func() {
		ui.Icon(c, icons[icon]).Size(12, 12).TextColor(t.TextMuted)
		ui.Text(c, title).FontSize(11).Bold().TextColor(t.TextMuted)
		ui.Text(c, fmt.Sprint(n)).FontSize(11).TextColor(t.TextMuted)
	})
	if h.Clicked() && !searching {
		if prefs.FoldedBranches == nil {
			prefs.FoldedBranches = map[string]bool{}
		}
		prefs.FoldedBranches[kind] = open
		prefs.save()
	}
	return open
}

// branchRow is a row of the branches' popover, the current branch's
// checked; it reports a click.
func branchRow(c *ui.Context, name string, current bool) bool {
	return branchRowElement(c, name, currentNote(current), current).Clicked()
}

// currentNote says a row is the current one.
func currentNote(current bool) string {
	if current {
		return loc.BranchCurrent()
	}
	return ""
}

// branchRowElement is a row with name, checked when current, and a note
// after it.
func branchRowElement(c *ui.Context, name, note string, current bool) ui.Element {
	t := c.Theme()
	row := ui.ButtonBase(c).Height(26).PaddingX(8).Gap(6).Radius(5).AlignItems(ui.Center).Label(name)
	if row.Hovered() {
		row.Background(t.Text.Alpha(0.08))
	}
	row.Children(func() {
		mark := ""
		if current {
			mark = "✓"
		}
		ui.Text(c, mark).Width(12).TextColor(t.Accent)
		ui.Text(c, name).SingleLine().Grow(1).Basis(0)
		if note != "" {
			ui.Text(c, note).FontSize(11).TextColor(t.TextMuted).SingleLine()
		}
	})
	return row
}

// branchMenuRow is a branch's row, which opens its menu on a press, as a
// menu button, and on a right click too, as a context menu.
func branchMenuRow(c *ui.Context, name string, current bool, items func(*ui.Menu)) {
	branchRowElement(c, name, currentNote(current), current).Menu(items).ContextMenu(items)
}

// mergeItems merge a branch into the current one, or rebase the current
// one onto it.
func (a *app) mergeItems(m *ui.Menu, rev string) {
	cur := a.status.Branch
	if cur == "" {
		cur = "HEAD"
	}
	if a.chose(m.Item(loc.CompareWith(cur))) {
		a.showCompare(loc.CompareTitle(cur, shortRev(rev)), "HEAD", rev)
	}
	if a.chose(m.Item(loc.MergeInto(cur))) {
		a.run(loc.OpMerge(shortRev(rev)), func(ctx context.Context) error { return a.repo.Merge(ctx, rev) }, nil)
	}
	if a.chose(m.Item(loc.RebaseOnto(cur))) {
		a.run(loc.OpRebaseOnto(shortRev(rev)), func(ctx context.Context) error { return a.repo.Rebase(ctx, rev) }, nil)
	}
}

// newBranch asks for a branch's name, made at start (HEAD when empty) and
// checked out.
func (a *app) newBranch(start string) {
	title := loc.NewBranch()
	if start != "" {
		title = loc.NewBranchFrom(shortRev(start))
	}
	a.ask1(title, loc.CreateAndSwitch(), "", func(name string) {
		a.run(loc.OpNewBranch(name), func(ctx context.Context) error {
			return a.repo.CreateBranch(ctx, name, start, true)
		}, nil)
	})
}

// deletedBranch is a local branch deleted while the app runs, and the commit
// it was at, to bring it back until the app quits, as IDEA does.
type deletedBranch struct {
	name, hash string
}

// rememberDeletedBranch keeps a branch just deleted, to bring back, as the
// last few of them.
func (a *app) rememberDeletedBranch(name, hash string) {
	a.undoBranches = append(a.undoBranches, deletedBranch{name, hash})
	if n := len(a.undoBranches); n > maxUndoBranches {
		a.undoBranches = a.undoBranches[n-maxUndoBranches:]
	}
}

// maxUndoBranches is how many of the branches deleted are kept to bring back.
const maxUndoBranches = 3

// undoBranch brings back a branch deleted, at the commit it was at.
func (a *app) undoBranch(i int) {
	u := a.undoBranches[i]
	a.run(loc.OpRestoreBranch(u.name), func(ctx context.Context) error {
		return a.repo.CreateBranch(ctx, u.name, u.hash, false)
	}, func() {
		a.update(func() { a.forgetUndoBranch(i) })
	})
}

// forgetUndoBranch takes a branch off the ones to bring back.
func (a *app) forgetUndoBranch(i int) {
	if i >= 0 && i < len(a.undoBranches) {
		a.undoBranches = slices.Delete(a.undoBranches, i, i+1)
	}
}

// undoBar offers to bring back the local branches deleted while the app runs,
// each until its row is closed: git keeps the commit until it prunes it, so
// the branch is remade at it, as IDEA does for a delete just made.
func (a *app) undoBar(c *ui.Context) {
	t := c.Theme()
	for i := range a.undoBranches {
		name := a.undoBranches[i].name
		ui.Row(c).Padding(6, 12).Gap(8).Background(t.Accent.Alpha(0.1)).AlignItems(ui.Center).Children(func() {
			ui.Text(c, loc.BranchDeleted(name)).Wrap().Grow(1)
			if ui.Button(c, loc.Undo()).Tooltip(loc.BranchUndoTip()).Clicked() {
				a.undoBranch(i)
			}
			if ui.Button(c, loc.Close()).Clicked() {
				a.forgetUndoBranch(i)
			}
		})
	}
}

// deleteBranch deletes a merged branch at once, and asks before deleting
// one whose commits would be lost. Either way it keeps the commit the branch
// was at, so undoBar brings it back until the app quits.
func (a *app) deleteBranch(b string) {
	var del func(force bool)
	del = func(force bool) {
		a.run(loc.OpDeleteBranch(b), func(ctx context.Context) error {
			hash, _ := a.repo.BranchHash(ctx, b) // before it goes, so undo has the commit
			err := a.repo.DeleteBranch(ctx, b, force)
			if git.NotMerged(err) && !force {
				a.update(func() {
					a.ask(loc.BranchNotMergedTitle(b), loc.BranchNotMergedMessage(), loc.DeleteAnyway(), func() { del(true) })
				})
				return nil
			}
			if err == nil && hash != "" {
				a.update(func() { a.rememberDeletedBranch(b, hash) })
			}
			return err
		}, nil)
	}
	del(false)
}

// commitMenu is a commit's context menu in the history.
func (a *app) commitMenu(m *ui.Menu, cm git.Commit) {
	if m.Item(loc.CopyHash()).Chosen() {
		mygo.Clipboard.WriteText(cm.Hash)
	}
	if m.Item(loc.CopyMessage()).Chosen() {
		mygo.Clipboard.WriteText(cm.Subject)
	}
	m.Separator()
	if m.Item(loc.CommitNewBranch()).Chosen() {
		a.newBranch(cm.Hash)
	}
	if m.Item(loc.CommitNewTag()).Chosen() {
		a.prompt = prompt{open: true, title: loc.NewTagTitle(cm.Short()), button: loc.Create(), message: loc.TagMessage(),
			ok: func(name, msg string) {
				a.run(loc.OpNewTag(name), func(ctx context.Context) error { return a.repo.Tag(ctx, name, cm.Hash, msg) }, nil)
			}}
	}
	if m.Item(loc.CompareWorktree()).Chosen() {
		a.showCompare(loc.CompareTitle(cm.Short(), loc.Worktree()), cm.Hash, "")
	}
	if m.Item(loc.CheckoutCommit()).Chosen() {
		a.run(loc.OpCheckout(cm.Short()), func(ctx context.Context) error { return a.repo.CheckoutDetached(ctx, cm.Hash) }, nil)
	}
	m.Separator()
	if m.Item(loc.CherryPick()).Chosen() {
		a.run(loc.CherryPick()+" "+cm.Short(), func(ctx context.Context) error { return a.repo.CherryPick(ctx, cm) }, nil)
	}
	if m.Item(loc.RevertCommit()).Chosen() {
		a.run(loc.OpRevert(cm.Short()), func(ctx context.Context) error { return a.repo.Revert(ctx, cm) }, nil)
	}
	a.mergeItems(m, cm.Hash)
	if m.Item(loc.EditCommitMessage()).Shortcut(0, ui.KeyF2).Chosen() {
		a.planReword(cm)
	}
	if n := a.logChosen.Len(); n > 1 && a.logChosen.Has(cm.Hash) {
		if m.Item(loc.SquashCommits(n)).Chosen() {
			a.planSquash()
		}
		if m.Item(loc.DropCommits(n)).Chosen() {
			a.planDrop(a.chosenCommits())
		}
	} else if m.Item(loc.DropCommit()).Chosen() {
		a.planDrop([]git.Commit{cm})
	}
	if m.Item(loc.RebaseFromHere()).Chosen() {
		a.planRebase(cm)
	}
	if a.status.Branch != "" {
		m.Submenu(loc.ResetTo(a.status.Branch), func(m *ui.Menu) {
			reset := func(mode git.ResetMode) {
				a.run(loc.OpReset(), func(ctx context.Context) error { return a.repo.Reset(ctx, cm.Hash, mode) }, nil)
			}
			if m.Item(loc.ResetSoft()).Chosen() {
				reset(git.ResetSoft)
			}
			if m.Item(loc.ResetMixed()).Chosen() {
				reset(git.ResetMixed)
			}
			if m.Item(loc.ResetHard()).Chosen() {
				a.ask(loc.ResetHardTitle(cm.Short()), loc.ResetHardMessage(), loc.ResetHardButton(), func() { reset(git.ResetHard) })
			}
		})
	}
	for _, ref := range cm.Refs {
		if tag, ok := strings.CutPrefix(ref, "tag: "); ok && m.Item(loc.DeleteTag(tag)).Chosen() {
			a.run(loc.OpDeleteTag(tag), func(ctx context.Context) error { return a.repo.DeleteTag(ctx, tag) }, nil)
		}
	}
}

// shortRev shortens a hash, and leaves a branch's name as it is.
func shortRev(rev string) string {
	if len(rev) == 40 && strings.Trim(rev, "0123456789abcdef") == "" {
		return rev[:8]
	}
	return rev
}
