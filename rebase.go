package main

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"mygit/internal/git"
	loc "mygit/locales"
)

// rebasePlan is an interactive rebase being planned: the commits from one
// on, oldest first, to reorder and to pick, reword, squash or drop.
type rebasePlan struct {
	open  bool
	base  string // the commit they go on, "" for the root
	steps []git.RebaseStep
	list  ui.ListState
}

var rebaseActions = []git.RebaseAction{git.Pick, git.Reword, git.Edit, git.Squash, git.Fixup, git.Drop}

// rebaseLabels name the actions, in the language shown.
func rebaseLabels() []string {
	return []string{loc.RebasePick(), loc.RebaseReword(), loc.RebaseEdit(), loc.RebaseSquash(), loc.RebaseFixup(), loc.RebaseDrop()}
}

// planRebase lists HEAD's commits from cm on, if cm is in HEAD's history.
func (a *app) planRebase(cm git.Commit) {
	base := ""
	if len(cm.Parents) > 0 {
		base = cm.Parents[0]
	}
	work(a, func(ctx context.Context) ([]git.RebaseStep, error) {
		if !a.repo.IsAncestor(ctx, cm.Hash) {
			return nil, errors.New(loc.ErrNotInHead())
		}
		return a.repo.RebaseSteps(ctx, base)
	}, func(steps []git.RebaseStep, err error) {
		if err != nil {
			a.errMsg = err.Error()
			return
		}
		a.rebase = rebasePlan{open: true, base: base, steps: steps}
		a.rebase.list.Reorder = func(rows []int, to int) { a.rebase.steps = move(a.rebase.steps, rows, to) }
	})
}

// move moves the items at rows before the item at to, as a list's
// reordering asks.
func move[T any](items []T, rows []int, to int) []T {
	var moved, rest []T
	before := 0
	for i, it := range items {
		if slices.Contains(rows, i) {
			moved = append(moved, it)
			if i < to {
				before++
			}
		} else {
			rest = append(rest, it)
		}
	}
	at := to - before
	return slices.Concat(rest[:at], moved, rest[at:])
}

func (a *app) rebaseView(c *ui.Context) {
	p := &a.rebase
	if !p.open {
		return
	}
	t := c.Theme()
	ui.Modal(c, &p.open, func() {
		ui.Column(c).Gap(10).Width(760).Height(520).Children(func() {
			ui.Text(c, loc.RebaseTitle()).Bold().FontSize(15)
			ui.Text(c, loc.RebaseHint()).TextColor(t.TextMuted)
			ui.List(c.Key("steps"), &p.list, len(p.steps), func(i int) {
				s := &p.steps[i]
				ui.Column(c).Padding(6, 4).Gap(6).Children(func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Text(c, s.Hash[:8]).Font("monospace").TextColor(t.Accent)
						subject := ui.Text(c, s.Subject).SingleLine().Grow(1).Basis(0)
						if s.Action == git.Drop {
							subject.TextColor(t.TextMuted)
						}
						labels := rebaseLabels()
						label := labels[slices.Index(rebaseActions, s.Action)]
						if ui.Select(c.Key(s.Hash), &label, labels).Label(loc.RebaseAction()).Width(200).Changed() {
							s.Action = rebaseActions[slices.Index(labels, label)]
							if s.Action == git.Reword && s.Message == "" {
								s.Message = s.Subject
							}
						}
					})
					if s.Action == git.Reword {
						ui.TextArea(c.Key("msg"+s.Hash), &s.Message).Label(loc.RebaseNewMessage()).Lines(1, 4)
					}
				})
			}).Grow(1).Basis(0)
			err := git.CheckSteps(p.steps)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if err != nil {
					ui.Text(c, err.Error()).TextColor(t.Danger).Grow(1)
				} else {
					ui.Box(c).Grow(1)
				}
				if ui.Button(c, loc.Cancel()).Clicked() {
					p.open = false
				}
				if ui.PrimaryButton(c, loc.RebaseStart()).Disabled(err != nil || a.busy != "").Clicked() {
					p.open = false
					base, steps := p.base, slices.Clone(p.steps)
					a.run(loc.OpRebaseInteractive(), func(ctx context.Context) error { return a.repo.RebaseInteractive(ctx, base, steps) }, nil)
				}
			})
		})
	})
}

// rewordPlan is a commit's message being edited, or that of commits
// being squashed into one.
type rewordPlan struct {
	open    bool
	title   string
	op      string // the operation's name, and its button's
	message string
	pushed  bool // already upstream, so the branch is force pushed after
	save    func(ctx context.Context, message string) error
}

// planReword opens cm's whole message to edit, if cm is in HEAD's history.
func (a *app) planReword(cm git.Commit) {
	a.planMessage(loc.RewordTitle(cm.Short()), loc.Save(), []git.Commit{cm}, func(ctx context.Context, msg string) error {
		return a.repo.Reword(ctx, cm, msg)
	})
}

// planSquash opens the messages of the commits chosen in the history,
// oldest first, to squash them into one with.
func (a *app) planSquash() {
	var chosen []git.Commit // newest first, as the history
	for _, cm := range a.commits {
		if a.logChosen.Has(cm.Hash) {
			chosen = append(chosen, cm)
		}
	}
	slices.Reverse(chosen)
	oldest, others := chosen[0], make([]string, 0, len(chosen)-1)
	for _, cm := range chosen[1:] {
		others = append(others, cm.Hash)
	}
	a.planMessage(loc.SquashTitle(len(chosen)), loc.Squash(), chosen, func(ctx context.Context, msg string) error {
		return a.repo.Squash(ctx, oldest, others, msg)
	})
}

// planMessage opens the dialog with the commits' messages, oldest first,
// if they are in HEAD's history.
func (a *app) planMessage(title, op string, commits []git.Commit, save func(context.Context, string) error) {
	work(a, func(ctx context.Context) (rewordPlan, error) {
		var msgs []string
		for _, cm := range commits {
			if !a.repo.IsAncestor(ctx, cm.Hash) {
				return rewordPlan{}, errors.New(loc.ErrRewordNotInHead())
			}
			msgs = append(msgs, a.repo.Message(ctx, cm.Hash))
		}
		return rewordPlan{open: true, title: title, op: op, message: strings.Join(msgs, "\n\n"),
			pushed: a.repo.Pushed(ctx, commits[0].Hash), save: save}, nil
	}, func(p rewordPlan, err error) {
		if err != nil {
			a.errMsg = err.Error()
			return
		}
		a.reword = p
	})
}

func (a *app) rewordView(c *ui.Context) {
	p := &a.reword
	if !p.open {
		return
	}
	ui.Modal(c, &p.open, func() {
		ui.Column(c).Gap(10).Width(560).Children(func() {
			ui.Text(c, p.title).Bold().FontSize(15)
			ui.TextArea(c.Key("msg"), &p.message).Label(loc.RebaseNewMessage()).AutoFocus().Lines(4, 12)
			if p.pushed {
				ui.Text(c, loc.RewordPushed()).TextColor(c.Theme().Danger)
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, loc.Cancel()).Clicked() {
					p.open = false
				}
				msg := strings.TrimSpace(p.message)
				if ui.PrimaryButton(c, p.op).Disabled(msg == "" || a.busy != "").Clicked() {
					p.open = false
					save, pushed := p.save, p.pushed
					a.run(p.title, func(ctx context.Context) error { return save(ctx, msg) }, func() {
						a.logChosen.Clear()
						if pushed {
							a.askForcePush()
						}
					})
				}
			})
		})
	})
}

// askForcePush offers to force push the branch whose pushed commits were
// rewritten.
func (a *app) askForcePush() {
	st := a.status
	a.ask(loc.ForcePushTitle(st.Branch), loc.ForcePushMessage(), loc.ForcePushButton(), func() {
		a.run(loc.ForcePushButton(), func(ctx context.Context) error { return a.repo.Push(ctx, st, true) }, nil)
	})
}
