package main

import (
	"github.com/egoist/mygo/ui"

	loc "mygit/locales"
)

// changeNav steps through the changes of the diff shown, and on to the
// next file's, as F7 and ⇧F7 do, and says which change the caret is in.
func (a *app) changeNav(c *ui.Context) {
	d := a.detail.diff
	n := 0
	if d != nil {
		n = len(d.Hunks())
	}
	if n == 0 && a.fileStep(1) == nil && a.fileStep(-1) == nil {
		return
	}
	t := c.Theme()
	text, color := "", t.TextMuted
	switch edge := a.armed(); {
	case edge > 0:
		text, color = loc.PressAgainNext("F7"), t.Accent
	case edge < 0:
		text, color = loc.PressAgainPrevious(shiftKey("F7")), t.Accent
	case n > 0:
		text = loc.ChangeCount(n)
		if i, inside := d.CurrentHunk(); inside {
			text = loc.ChangeOf(i+1, n)
		}
	}
	ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
		if text != "" {
			ui.Text(c, text).FontSize(11).TextColor(color).Padding(0, 4, 0, 0)
		}
		if iconButton(c, "up", loc.PreviousChange()+" "+shiftKey("F7")) {
			a.stepChange(true)
		}
		if iconButton(c, "chevron-down", loc.NextChange()+" F7") {
			a.stepChange(false)
		}
	})
}

// stepChange puts the caret on the next change of the diff shown, or with
// back the one before, as JetBrains' F7 and ⇧F7 do. Past the last change,
// it first says so, and pressed again goes on to the next file, opened
// at its first change, or going back the previous file, at its last.
func (a *app) stepChange(back bool) {
	dir := 1
	if back {
		dir = -1
	}
	if d := a.detail.diff; d != nil && len(d.Hunks()) > 0 {
		n := len(d.Hunks())
		i, inside := d.CurrentHunk() // else the last change before the caret
		switch {
		case !back:
			i++
		case inside:
			i--
		}
		if i >= 0 && i < n {
			a.detail.edge = 0
			d.GoToHunk(i)
			return
		}
		if a.armed() != dir {
			d.GoToHunk(min(max(i, 0), n-1))
			if a.fileStep(dir) != nil { // nothing to say at the last file
				a.detail.edge, a.detail.edgeAt = dir, d.Cursor()
			}
			return
		}
	}
	if step := a.fileStep(dir); step != nil {
		step()
	}
}

// armed tells which way the last step stopped at the end of the diff
// shown, saying the next goes on to another file: 1 past its last change,
// -1 before its first, or 0 once the caret has moved since.
func (a *app) armed() int {
	d := &a.detail
	if d.edge == 0 || d.diff == nil || d.diff.Cursor() != d.edgeAt {
		return 0
	}
	return d.edge
}

// fileStep shows the file after the one shown, or before it with dir -1,
// opening it at its first change, or its last going back: of the commit's
// files, or along the staged changes and then the others; nil when there
// is none that way.
func (a *app) fileStep(dir int) func() {
	d := &a.detail
	switch {
	case d.commit != nil:
		i := d.fileSel + dir
		if d.fileSel < 0 || i < 0 || i >= len(d.files) {
			return nil
		}
		return func() {
			a.showFile(i)
			if diff := a.detail.diff; diff != nil {
				diff.GoToHunk(enteredHunk(len(diff.Hunks()), dir))
			}
		}
	case d.change != nil:
		at := indexOf(a.staged, d.change.Path)
		if !d.staged {
			if at = indexOf(a.unstaged, d.change.Path); at >= 0 {
				at += len(a.staged)
			}
		}
		k := at + dir
		if at < 0 || k < 0 || k >= len(a.staged)+len(a.unstaged) {
			return nil
		}
		staged, i := k < len(a.staged), k
		if !staged {
			i -= len(a.staged)
		}
		return func() {
			path := a.changes(staged)[i].Path
			a.chooseChange(i, staged)
			// Its diff opens there once loaded; not when an edit not
			// saved was asked about first.
			if ch := a.detail.change; ch != nil && ch.Path == path && a.detail.staged == staged {
				a.detail.enter = dir
			}
		}
	}
	return nil
}

// enteredHunk is the change a diff of n opens at, entered going dir: the
// first going on, the last going back.
func enteredHunk(n, dir int) int {
	if dir < 0 {
		return n - 1
	}
	return 0
}
