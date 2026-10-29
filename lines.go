package main

import (
	"context"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mygit/internal/patch"
	loc "mygit/locales"
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
		// The lines only once clicked: a change of a whole file is tens of
		// thousands of them, and the bar is built every frame.
		bar.Gap(4).Children(func() {
			if a.detail.staged {
				if ui.Button(c, loc.Unstage()).Clicked() {
					a.applyLines(a.hunkLines(h.Hunk), lineUnstage)
				}
				return
			}
			if ui.Button(c, loc.Discard()).Clicked() {
				a.applyLines(a.hunkLines(h.Hunk), lineDiscard)
			}
			if ui.PrimaryButton(c, loc.Stage()).Clicked() {
				a.applyLines(a.hunkLines(h.Hunk), lineStage)
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
// change's lines a line apart. Each run is walked once, from the first of
// its lines met: a whole file changed is one run of all its lines.
func (a *app) wholeRuns(chosen map[patch.Line]bool) map[patch.Line]bool {
	hunks := a.detail.patch.Hunks
	runs := map[patch.Line]bool{}
	in := func(lines []string, i int) bool { return lines[i] != "" && lines[i][0] != ' ' }
	for l := range chosen {
		if runs[l] {
			continue
		}
		lines := hunks[l.Hunk].Lines
		from, to := l.Index, l.Index+1
		for from > 0 && in(lines, from-1) {
			from--
		}
		for to < len(lines) && in(lines, to) {
			to++
		}
		for i := from; i < to; i++ {
			if lines[i][0] != '\\' {
				runs[patch.Line{Hunk: l.Hunk, Index: i}] = true
			}
		}
		runs[l] = true
	}
	return runs
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
		apply(loc.OpStage(), true, false)
	case lineUnstage:
		apply(loc.OpUnstage(), true, true)
	case lineDiscard:
		a.ask(loc.DiscardLinesTitle(), loc.DiscardLinesMessage(), loc.Discard(), func() { apply(loc.OpDiscard(), false, true) })
	}
}

// confirmation is a question an alert asks before an action that can't be
// undone.
type confirmation struct {
	open                   bool
	title, message, button string
	yes                    func()
	// A third button, between Cancel and the default one, and what
	// Cancel (or Escape) does, when given.
	other   string
	otherFn func()
	cancel  func()
}

func (a *app) ask(title, message, button string, yes func()) {
	a.confirm = confirmation{open: true, title: title, message: message, button: button, yes: yes}
}

func (a *app) confirmView(c *ui.Context) {
	q := &a.confirm
	buttons := []string{loc.Cancel(), q.button}
	if q.other != "" {
		buttons = []string{loc.Cancel(), q.other, q.button}
	}
	switch i := ui.AlertDialog(c, &q.open, q.title, q.message, buttons...); {
	case i == len(buttons)-1 && q.yes != nil:
		q.yes()
	case i == 1 && q.other != "" && q.otherFn != nil:
		q.otherFn()
	case (i == 0 || i == ui.AlertDismissed) && q.cancel != nil:
		q.cancel()
	}
}

// leaving asks, while the file shown is edited and not saved, whether to
// save it before go shows something else, and says it asked: go runs once
// the edit is saved or given up, and Cancel stays with the file, its row
// chosen again. A caller returns when it asked, as go calls it again.
func (a *app) leaving(next func()) bool {
	if !a.unsaved() {
		return false
	}
	path := a.detail.change.Path
	a.confirm = confirmation{open: true, title: loc.UnsavedTitle(path), message: loc.UnsavedMessage(),
		button: loc.Save(), other: loc.DontSave(),
		yes: func() {
			a.saveFile()
			if !a.unsaved() { // saved
				next()
			}
		},
		otherFn: func() {
			a.detail.diff.New.MarkSaved() // given up, as the file is left
			a.detail.whole = false
			next()
		},
		cancel: a.chooseShown,
	}
	return true
}

// savingFirst asks, while the file shown is edited and not saved, whether
// to save it before next, which reads the files on disk (staging,
// committing, stashing), and says it asked, as VS Code asks before a
// commit. Not saving goes on with the file as saved, the edits kept.
func (a *app) savingFirst(next func()) bool {
	if !a.unsaved() || a.goingOn {
		return false
	}
	path := a.detail.change.Path
	a.confirm = confirmation{open: true, title: loc.SaveFirstTitle(path), message: loc.SaveFirstMessage(),
		button: loc.SaveAndGoOn(), other: loc.GoOnUnsaved(),
		yes: func() {
			a.saveFile()
			if !a.unsaved() {
				next()
			}
		},
		otherFn: func() {
			a.goingOn = true // next asks no more
			defer func() { a.goingOn = false }()
			next()
		},
	}
	return true
}

// chooseShown chooses the row of the change shown again, as a choice of
// another was given up.
func (a *app) chooseShown() {
	ch := a.detail.change
	if ch == nil {
		return
	}
	if a.detail.staged {
		a.stagedSel, a.unstagedSel = indexOf(a.staged, ch.Path), -1
	} else {
		a.unstagedSel, a.stagedSel = indexOf(a.unstaged, ch.Path), -1
	}
	a.logSel, a.stashSel = -1, -1
}

// unsaved tells whether the file edited in the diff has changes not saved.
func (a *app) unsaved() bool {
	d := a.detail
	return d.whole && d.diff != nil && d.diff.New.Modified()
}
