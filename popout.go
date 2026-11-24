package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	loc "mygit/locales"
)

// popout is a window of a diff's own, for the room the pane under the log
// does not give, as IntelliJ opens a diff in a window (#46). It shows a
// copy of the diff as it popped out, read only, so the main window goes
// on to other files and the two keep their own scroll.
type popout struct {
	title  string
	diff   *editor.DiffState
	starts []int             // the lines a patch's hunks start at, nil for whole files
	patch  *editor.FilePatch // numbers a patch's lines, nil for whole files
}

// diffAlone tells whether a diff shows by itself, to pop out or annotate:
// not a picture, a binary or folded file, or files one after another.
func (a *app) diffAlone() bool {
	d := &a.detail
	return d.diff != nil && d.images == nil && !d.folded && !d.binary && d.all == nil &&
		(d.commit == nil || !prefs.CommitAll || d.commitAll == nil)
}

// popOut copies the diff shown, nil when none shows by itself.
func (a *app) popOut() *popout {
	d := &a.detail
	if !a.diffAlone() {
		return nil
	}
	var f editor.FilePatch
	var title string
	switch {
	case d.commit != nil && d.fileSel >= 0 && d.fileSel < len(d.files):
		f = d.files[d.fileSel]
		title = fileName(f) + " · " + d.commit.Short()
	case d.change != nil && d.whole: // the file as it is, edits not saved too
		po := &popout{title: d.change.Path, diff: textDiff(d.change.Path, d.diff.Old.String(), d.diff.New.String())}
		po.diff.ExpandUnchanged = d.diff.ExpandUnchanged
		return po
	case d.change != nil && d.patch != nil:
		f = *d.patch
		title = d.change.Path
	default:
		return nil
	}
	po := &popout{title: title}
	if texts, ok := d.texts[fileName(f)]; ok && d.patchWhole {
		po.diff = fullDiff(f, texts[0], texts[1])
	}
	if po.diff == nil {
		po.diff, po.starts, po.patch = patchDiff(f), patchStarts(f), &f
	}
	po.diff.ExpandUnchanged = d.diff.ExpandUnchanged
	return po
}

// popButton opens the diff shown in a window of its own.
func (a *app) popButton(c *ui.Context) {
	if !a.diffAlone() {
		return
	}
	if iconButton(c, "maximize", loc.PopOutDiff()) {
		if po := a.popOut(); po != nil {
			po.open()
		}
	}
}

func (po *popout) open() {
	win := mygo.NewWindow(customTitleBar(mygo.WindowOptions{
		Title:     po.title + " — MyGit",
		Width:     1280,
		Height:    860,
		MinWidth:  480,
		MinHeight: 320,
		StateKey:  "diff",
		Content:   ui.View(po.view),
	}))
	dropMenuBar(win)
}

func (po *popout) view(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		titleBar(c).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
			ui.Text(c, po.title).Bold().SingleLine().Grow(1).Basis(0)
			wrapToggle(c, po.diff)
		})
		row := ui.Row(c).Fill()
		row.Children(func() {
			editor.DiffView(c.Key("diff"), po.diff).Grow(1).Basis(0).AlignSelf(ui.Stretch)
			overview(c, po.diff, row, po.starts, func(i int) int { return patchLine(po.patch, editor.DiffNew, i) })
		})
	})
}
