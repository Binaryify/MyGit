package main

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"

	loc "mygit/locales"
)

// canCommitAndPush tells whether the staged changes can be committed and
// pushed at once (#27): there are some, a message or a model to write one,
// a branch to push, and nothing running. Amending isn't, as its push would
// have to force.
func (a *app) canCommitAndPush() bool {
	return a.busy == "" && len(a.staged) > 0 && !a.amend && a.status.Branch != "" &&
		(strings.TrimSpace(a.message) != "" || prefs.AI.on())
}

// commitAndPush commits the staged changes and pushes the branch, as VS
// Code's Commit & Push: with the message typed, or, the box empty, the one
// the model writes, which shows in the box as it comes. A push that fails
// leaves the commit made, saying so.
func (a *app) commitAndPush() {
	if a.canCommitAndPush() && a.savingFirst(a.commitAndPush) {
		return
	}
	if !a.canCommitAndPush() {
		return
	}
	msg, s := a.message, prefs.AI
	a.run(loc.OpCommitAndPush(), func(ctx context.Context) error {
		if strings.TrimSpace(msg) == "" {
			m, err := a.writeMessage(ctx, s)
			if err != nil {
				return err
			}
			msg = m
			a.update(func() { a.message = m })
		}
		if err := a.repo.Commit(ctx, msg, false); err != nil {
			return err
		}
		a.update(func() { a.message, a.recall = "", 0 })
		st, err := a.repo.Status(ctx)
		if err == nil {
			err = a.repo.Push(ctx, st, false)
		}
		if err != nil { // said as it is: the commit is made, the push not
			msg := loc.ErrPushAfterCommit(err.Error())
			a.update(func() { a.errMsg = msg })
		}
		return nil
	}, nil)
}

// commitButton is the Commit button, the width of the sidebar as VS
// Code's, split: its arrow's menu commits and pushes at once.
func (a *app) commitButton(c *ui.Context) {
	label := loc.Commit()
	if n := len(a.staged); n > 0 {
		label = loc.CommitFiles(n)
	}
	ui.Row(c).Gap(1).Children(func() {
		if ui.PrimaryButton(c, label).Disabled(!a.canCommit()).Tooltip(shortcut("↩")).Grow(1).Clicked() {
			a.commit()
		}
		more := ui.PrimaryButton(c, "").Label(loc.MoreCommitActions()).Tooltip(loc.CommitAndPush() + "  " + shiftShortcut("↩")).
			Disabled(a.busy != "").PaddingX(8)
		more.Children(func() { ui.Icon(c, icons["chevron-down"]).Size(14, 14) })
		more.Menu(a.commitActions)
	})
}

// commitActions is the menu of the Commit button's arrow: committing and
// pushing at once.
func (a *app) commitActions(m *ui.Menu) {
	if m.Item(loc.CommitAndPush()).Shortcut(ui.Cmd|ui.Shift, ui.KeyEnter).Disabled(!a.canCommitAndPush()).Chosen() {
		a.commitAndPush()
	}
}
