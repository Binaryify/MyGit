package main

import (
	"context"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mygit/internal/patch"
)

// reviewLines lets the lines of a change's diff be chosen, by their
// numbers or a change at a time, to stage, unstage or discard.
func (a *app) reviewLines(d *editor.DiffState) {
	d.LineSelection = true
	d.HunkActions = true
	d.RenderHunkActions = func(c *ui.Context, bar ui.Element, h *editor.DiffHunkActions) {
		chosen := a.hunkLines(h.Hunk)
		bar.Gap(4).Children(func() {
			if a.detail.staged {
				if ui.Button(c, "取消暂存").Clicked() {
					a.applyLines(chosen, lineUnstage)
				}
				return
			}
			if ui.Button(c, "丢弃").Clicked() {
				a.applyLines(chosen, lineDiscard)
			}
			if ui.PrimaryButton(c, "暂存").Clicked() {
				a.applyLines(chosen, lineStage)
			}
		})
	}
}

type lineAction int

const (
	lineStage lineAction = iota
	lineUnstage
	lineDiscard
)

// hunkLines are the lines of a change of the diff shown.
func (a *app) hunkLines(h editor.DiffHunk) map[patch.Line]bool {
	old, new := patch.Lines(*a.detail.patch)
	chosen := map[patch.Line]bool{}
	for i := h.OldStart; i < h.OldEnd && i < len(old); i++ {
		chosen[old[i]] = true
	}
	for i := h.NewStart; i < h.NewEnd && i < len(new); i++ {
		chosen[new[i]] = true
	}
	return chosen
}

// selectedLines are the changed lines whose numbers are selected on one
// side, and the other side's lines of each change selected whole, so that
// selecting a line changed in place takes its old line with it.
func (a *app) selectedLines() map[patch.Line]bool {
	d := a.detail.diff
	if a.detail.patch == nil || d == nil {
		return nil
	}
	r, ok := d.SelectedLines()
	if !ok {
		return nil
	}
	p := *a.detail.patch
	old, new := patch.Lines(p)
	side, other := new, old
	if r.Side == editor.DiffOld {
		side, other = old, new
	}
	chosen := map[patch.Line]bool{}
	for i := max(r.Start, 0); i <= r.End && i < len(side); i++ {
		if l := side[i]; p.Hunks[l.Hunk].Lines[l.Index][0] != ' ' {
			chosen[l] = true
		}
	}
	for _, h := range d.Hunks() {
		lo, hi, olo, ohi := h.NewStart, h.NewEnd, h.OldStart, h.OldEnd
		if r.Side == editor.DiffOld {
			lo, hi, olo, ohi = olo, ohi, lo, hi
		}
		if lo < hi && r.Start <= lo && hi-1 <= r.End {
			for i := olo; i < ohi && i < len(other); i++ {
				chosen[other[i]] = true
			}
		}
	}
	if len(chosen) == 0 {
		return nil
	}
	return chosen
}

// applyLines stages, unstages or discards lines of the change shown;
// discarding asks first, as it can't be undone.
func (a *app) applyLines(chosen map[patch.Line]bool, act lineAction) {
	if a.detail.patch == nil || len(chosen) == 0 {
		return
	}
	text := patch.Select(*a.detail.patch, chosen, act != lineStage)
	if text == "" {
		return
	}
	apply := func(label string, cached, reverse bool) {
		a.run(label, func(ctx context.Context) error { return a.repo.Apply(ctx, text, cached, reverse) }, nil)
	}
	switch act {
	case lineStage:
		apply("暂存", true, false)
	case lineUnstage:
		apply("取消暂存", true, true)
	case lineDiscard:
		a.ask("丢弃这些更改？", "选中的更改会从工作区删除，无法撤销。", "丢弃", func() { apply("丢弃", false, true) })
	}
}

// confirmation is a question an alert asks before an action that can't be
// undone.
type confirmation struct {
	open                   bool
	title, message, button string
	yes                    func()
}

func (a *app) ask(title, message, button string, yes func()) {
	a.confirm = confirmation{true, title, message, button, yes}
}

func (a *app) confirmView(c *ui.Context) {
	q := &a.confirm
	if ui.AlertDialog(c, &q.open, q.title, q.message, "取消", q.button) == 1 && q.yes != nil {
		q.yes()
	}
}
