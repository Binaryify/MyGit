package main

import (
	"context"

	"github.com/egoist/mygo/ui"

	"mygit/internal/git"
	loc "mygit/locales"
)

// leftPanel is the working tree's changes, or the stash.
func (a *app) leftPanel(c *ui.Context) {
	ui.Column(c).Fill().Children(func() {
		ui.Box(c).Padding(8, 10, 0, 10).Children(func() {
			ui.Segmented(c, &a.leftTab, loc.ChangesTab(), loc.StashTab(len(a.stashes))).FillWidth()
		})
		if a.leftTab == 1 {
			a.stashPanel(c)
		} else {
			a.changesPanel(c)
		}
	})
}

func (a *app) stashPanel(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).Basis(0).Children(func() {
		ui.Row(c).Padding(8, 10).Children(func() {
			changed := len(a.staged)+len(a.unstaged) > 0
			if ui.Button(c, loc.StashChanges()).Disabled(!changed || a.busy != "").Grow(1).Clicked() {
				a.stashChanges()
			}
		})
		stashes := a.stashes
		l := ui.List(c.Key("stash"), &a.stashList, len(stashes), func(i int) {
			s := stashes[i]
			row := ui.Column(c).Padding(6, 10).Gap(2)
			row.Children(func() {
				ui.Text(c, s.Subject).SingleLine()
				ui.Text(c, s.Ref+" · "+s.Time.Format("2006-01-02 15:04")).FontSize(11).TextColor(t.TextMuted)
			})
			row.ContextMenu(func(m *ui.Menu) { a.stashMenu(m, s) })
		}).Grow(1).Basis(0).Children(func() {
			if len(stashes) == 0 {
				ui.Text(c, loc.StashNone()).TextColor(t.TextMuted).Padding(12)
			}
		})
		if l.Changed() && a.stashSel >= 0 && a.stashSel < len(stashes) {
			a.showStash(stashes[a.stashSel])
			a.logSel = -1
		}
		if l.Submitted() && a.stashSel >= 0 && a.stashSel < len(stashes) {
			s := stashes[a.stashSel]
			a.run(loc.OpApply(s.Ref), func(ctx context.Context) error { return a.repo.StashApply(ctx, s.Ref, false) }, nil)
		}
	})
}

// stashChanges asks for a stash's message, then stashes the changes.
func (a *app) stashChanges() {
	a.prompt = prompt{open: true, title: loc.StashTitle(), button: loc.OpStash(), check: loc.StashUntracked(), checked: true,
		ok: func(msg, _ string) {
			untracked := a.prompt.checked
			a.run(loc.OpStash(), func(ctx context.Context) error { return a.repo.StashPush(ctx, msg, untracked) }, nil)
		}}
	a.prompt.value = "WIP"
}

func (a *app) stashMenu(m *ui.Menu, s git.Stash) {
	if m.Item(loc.StashApply()).Chosen() {
		a.run(loc.OpApply(s.Ref), func(ctx context.Context) error { return a.repo.StashApply(ctx, s.Ref, false) }, nil)
	}
	if m.Item(loc.StashPop()).Chosen() {
		a.run(loc.OpPop(s.Ref), func(ctx context.Context) error { return a.repo.StashApply(ctx, s.Ref, true) }, nil)
	}
	if m.Item(loc.StashBranch()).Chosen() {
		a.ask1(loc.StashBranchTitle(s.Ref), loc.Create(), "", func(name string) {
			a.run(loc.OpNewBranch(name), func(ctx context.Context) error { return a.repo.StashBranch(ctx, name, s.Ref) }, nil)
		})
	}
	m.Separator()
	if m.Item(loc.DeleteMenu()).Chosen() {
		a.ask(loc.StashDeleteTitle(s.Ref), loc.StashDeleteMessage(s.Subject), loc.Delete(), func() {
			a.run(loc.OpDrop(s.Ref), func(ctx context.Context) error { return a.repo.StashDrop(ctx, s.Ref) }, nil)
		})
	}
}

// operationName names an operation, in the language shown.
func operationName(op git.Operation) string {
	switch op {
	case git.Merging:
		return loc.OpNameMerge()
	case git.Rebasing:
		return loc.OpNameRebase()
	case git.CherryPick:
		return loc.OpNameCherryPick()
	}
	return loc.OpNameRevert()
}

// operationBar says the repository is in the middle of a merge, a rebase,
// a cherry-pick or a revert, and goes on with it or gives it up.
func (a *app) operationBar(c *ui.Context) {
	if a.op == git.NoOperation {
		return
	}
	op, name := a.op, operationName(a.op)
	conflicts := 0
	for _, ch := range a.unstaged {
		if ch.Conflict {
			conflicts++
		}
	}
	ui.Row(c).Padding(6, 12).Gap(8).AlignItems(ui.Center).Background(ui.RGB(230, 126, 34).Alpha(0.15)).Children(func() {
		msg := loc.OpResolved(name)
		if conflicts > 0 {
			msg = loc.OpConflicts(name, conflicts)
		}
		ui.Text(c, msg).Grow(1).Wrap()
		idle := a.busy == ""
		if ui.Button(c, loc.AbortMenu()).Disabled(!idle).Clicked() {
			a.ask(loc.AbortTitle(name), loc.AbortMessage(name), loc.Abort(), func() {
				a.run(loc.OpAbort(name), func(ctx context.Context) error { return a.repo.Abort(ctx, op) }, nil)
			})
		}
		if op == git.Rebasing || op == git.CherryPick {
			if ui.Button(c, loc.SkipCommit()).Disabled(!idle).Clicked() {
				a.run(loc.OpSkip(), func(ctx context.Context) error { return a.repo.Skip(ctx, op) }, nil)
			}
		}
		if ui.PrimaryButton(c, loc.Continue()).Disabled(!idle || conflicts > 0).Clicked() {
			a.run(loc.OpContinue(name), func(ctx context.Context) error { return a.repo.Continue(ctx, op) }, nil)
		}
	})
}
