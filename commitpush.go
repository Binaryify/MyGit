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

// commitButtons are Commit and Commit & Push side by side, half the
// sidebar each, as IDEA's: committing and pushing at once is as often
// what's wanted, so it shows rather than hides in a menu (#33).
func (a *app) commitButtons(c *ui.Context) {
	label := loc.Commit()
	if n := len(a.staged); n > 0 {
		label = loc.CommitFiles(n)
	}
	ui.Row(c).Gap(8).Children(func() {
		if ui.PrimaryButton(c, label).Disabled(!a.canCommit()).Tooltip(shortcut("↩")).Grow(1).Basis(0).Clicked() {
			a.commit()
		}
		if ui.Button(c, loc.CommitAndPush()).Disabled(!a.canCommitAndPush()).Tooltip(shiftShortcut("↩")).Grow(1).Basis(0).Clicked() {
			a.commitAndPush()
		}
	})
}
