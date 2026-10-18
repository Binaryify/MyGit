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
		if a.unsaved() {
			return // the patch is the file's as saved
		}
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

// lineMap numbers the patch's lines by the diff's: the lines of the whole
// files, or of the patch's own texts.
func (a *app) lineMap() (old, new func(i int) (patch.Line, bool)) {
	p := *a.detail.patch
	if a.detail.whole {
		om, nm := patch.FileLines(p)
		look := func(m map[int]patch.Line) func(int) (patch.Line, bool) {
			return func(i int) (patch.Line, bool) { l, ok := m[i]; return l, ok }
		}
		return look(om), look(nm)
	}
	oldLines, newLines := patch.Lines(p)
	look := func(s []patch.Line) func(int) (patch.Line, bool) {
		return func(i int) (patch.Line, bool) {
			if i < 0 || i >= len(s) {
				return patch.Line{}, false
			}
			return s[i], true
		}
	}
	return look(oldLines), look(newLines)
}

// changed tells whether a line of the patch is an addition or deletion.
func (a *app) changed(l patch.Line) bool {
	return a.detail.patch.Hunks[l.Hunk].Lines[l.Index][0] != ' '
}

// hunkLines are the changed lines of a change of the diff shown.
func (a *app) hunkLines(h editor.DiffHunk) map[patch.Line]bool {
	old, new := a.lineMap()
	chosen := map[patch.Line]bool{}
	add := func(at func(int) (patch.Line, bool), from, to int) {
		for i := from; i < to; i++ {
			if l, ok := at(i); ok && a.changed(l) {
				chosen[l] = true
			}
		}
	}
	add(old, h.OldStart, h.OldEnd)
	add(new, h.NewStart, h.NewEnd)
	if len(chosen) == 0 { // the diff slid the change past git's: look beside it
		add(old, h.OldStart-1, h.OldEnd+1)
		add(new, h.NewStart-1, h.NewEnd+1)
	}
	return a.wholeRuns(chosen)
}

// wholeRuns widens lines to the runs of changed lines they are in, as git
// cut the patch: a change at a time, as the diff and git may place a
// change's lines a line apart.
func (a *app) wholeRuns(chosen map[patch.Line]bool) map[patch.Line]bool {
	hunks := a.detail.patch.Hunks
	for l := range chosen {
		lines := hunks[l.Hunk].Lines
		for i := l.Index - 1; i >= 0 && lines[i] != "" && lines[i][0] != ' '; i-- {
			if lines[i][0] != '\\' {
				chosen[patch.Line{Hunk: l.Hunk, Index: i}] = true
			}
		}
		for i := l.Index + 1; i < len(lines) && lines[i] != "" && lines[i][0] != ' '; i++ {
			if lines[i][0] != '\\' {
				chosen[patch.Line{Hunk: l.Hunk, Index: i}] = true
			}
		}
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
	old, new := a.lineMap()
	side, other := new, old
	if r.Side == editor.DiffOld {
		side, other = old, new
	}
	chosen := map[patch.Line]bool{}
	for i := max(r.Start, 0); i <= r.End; i++ {
		if l, ok := side(i); ok && a.changed(l) {
			chosen[l] = true
		}
	}
	for _, h := range d.Hunks() {
		lo, hi, olo, ohi := h.NewStart, h.NewEnd, h.OldStart, h.OldEnd
		if r.Side == editor.DiffOld {
			lo, hi, olo, ohi = olo, ohi, lo, hi
		}
		if lo < hi && r.Start <= lo && hi-1 <= r.End {
			for i := olo; i < ohi; i++ {
				if l, ok := other(i); ok && a.changed(l) {
					chosen[l] = true
				}
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

// unsaved tells whether the file edited in the diff has changes not saved.
func (a *app) unsaved() bool {
	d := a.detail
	return d.whole && d.diff != nil && d.diff.New.Modified()
}
