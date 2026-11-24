package main

import (
	"context"
	"sort"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mygit/internal/git"
	loc "mygit/locales"
)

// annotations say who last changed each line of the sides of the diff
// shown, in a column beside them, as IDEA's "Annotate with Git Blame" in
// its diff (#51): a click on a line shows the commit that made it.
type annotations struct {
	diff  *editor.DiffState  // the diff they are of
	raw   string             // and the change's patch, which a file changed on disk changes
	sides [2][]git.BlameLine // the old side's lines, then the new's; nil for none
	has   [2]bool            // the sides with a file to blame, which show a column
}

// annotateWidth is the width of an annotations column, in DIPs.
const annotateWidth = 150

// annotateToggle shows the annotations beside the diffs, or hides them,
// kept for the diffs opened next.
func (a *app) annotateToggle(c *ui.Context) {
	if !a.diffAlone() {
		return
	}
	if iconToggle(c, "history", loc.Annotate(), loc.Annotate()+"\n"+loc.AnnotateTip(), &prefs.Annotate) {
		prefs.save()
	}
}

// annotate blames the sides of the diff shown, once for each diff and
// patch, while the annotations are on.
func (a *app) annotate() {
	d := &a.detail
	if !prefs.Annotate || d.diff == nil {
		return
	}
	if n := d.notes; n != nil && n.diff == d.diff && n.raw == d.raw {
		return
	}
	srcs := a.blameSources()
	n := &annotations{diff: d.diff, raw: d.raw, has: [2]bool{srcs[0].rev != "", srcs[1].rev != ""}}
	d.notes = n
	work(a, func(ctx context.Context) ([2][]git.BlameLine, error) {
		var sides [2][]git.BlameLine
		for i, s := range srcs {
			if s.rev == "" {
				continue
			}
			sides[i], _ = a.repo.Blame(ctx, s.rev, s.path) // a side git can't blame shows none
		}
		return sides, nil
	}, func(sides [2][]git.BlameLine, _ error) { n.sides = sides })
}

// blameSource is a side of a diff to blame: a revision, or "WORKTREE" or
// "INDEX", as Blame takes them, and the file's path there; no rev for a
// side with no file.
type blameSource struct{ rev, path string }

// blameSources are where the sides of the diff shown come from.
func (a *app) blameSources() [2]blameSource {
	d := &a.detail
	var sides [2]textSide
	var paths [2]string
	switch {
	case d.commit != nil && d.fileSel >= 0 && d.fileSel < len(d.files):
		f := d.files[d.fileSel]
		sides, paths = d.sides, [2]string{f.OldName, f.NewName}
	case d.change != nil:
		c := d.change
		sides[0], sides[1] = changeTexts(d.staged)
		paths = [2]string{c.Path, c.Path}
		if c.OldPath != "" {
			paths[0] = c.OldPath
		}
		if c.Work == '?' && !d.staged { // untracked: no old side
			sides[0] = textSide{}
		}
	}
	var srcs [2]blameSource
	for i, s := range sides {
		rev := ""
		switch s.kind {
		case sideRev:
			rev = s.rev
		case sideIndex:
			rev = "INDEX"
		case sideDisk:
			rev = "WORKTREE"
		}
		if paths[i] != "" {
			srcs[i] = blameSource{rev, paths[i]}
		}
	}
	return srcs
}

// annotatedRow is a line of a side of a diff in view, top DIPs below the
// top of the viewer.
type annotatedRow struct {
	side editor.DiffSide
	line int
	top  float64
}

// annotationColumn is the annotations beside the diff: one before a
// unified diff, for its lines and those it deleted, or one beside each
// side of a split diff with a file, the old side's before it and the new
// side's after it. before tells which.
func (a *app) annotationColumn(c *ui.Context, d *editor.DiffState, before bool) {
	n := a.detail.notes
	if !prefs.Annotate || n == nil || n.diff != d {
		return
	}
	sides := []editor.DiffSide{editor.DiffNew, editor.DiffOld} // unified: all of them
	switch {
	case d.Layout == editor.DiffSplit && before:
		sides = []editor.DiffSide{editor.DiffOld}
	case d.Layout == editor.DiffSplit:
		sides = []editor.DiffSide{editor.DiffNew}
	case !before:
		return
	}
	if len(sides) == 1 && !n.has[sides[0]] {
		return
	}
	p := a.diffPatch()
	blamed := func(r annotatedRow) (git.BlameLine, bool) {
		lines := n.sides[r.side]
		if r.side == editor.DiffNew && d.New.Modified() {
			return git.BlameLine{}, false // its lines have moved since
		}
		i := patchLine(p, r.side, r.line) - 1
		if i < 0 || i >= len(lines) {
			return git.BlameLine{}, false
		}
		return lines[i], true
	}
	t := c.Theme()
	col := ui.Box(c.Key(map[bool]string{true: "annotations before", false: "annotations after"}[before])).
		Width(annotateWidth).AlignSelf(ui.Stretch).Shrink(0).Clip().Label(loc.AnnotateTip()).
		BorderColor(t.Border).HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind != ui.InputScroll {
			return false
		}
		d.SetScrollY(max(d.ScrollY()+float64(ev.DY), 0)) // the diff scrolls under the column too
		return true
	})
	if before {
		col.BorderWidth(0, 1, 0, 0)
	} else {
		col.BorderWidth(0, 0, 0, 1)
	}
	// The line under the pointer: its commit as a tooltip, and a click
	// shows the commit.
	hovered := annotatedRow{line: -1}
	if _, y, over := col.PointerPosition(); over {
		rows, lh := annotatedRows(d, sides)
		for _, r := range rows {
			if float64(y) >= r.top && float64(y) < r.top+lh {
				hovered = r
				break
			}
		}
	}
	if bl, ok := blamed(hovered); ok && hovered.line >= 0 && !bl.NotCommitted() {
		col.Cursor(ui.CursorPointer).Tooltip(bl.Hash[:8] + " " + bl.Summary + "\n" + bl.Author + " · " + bl.Time.Format("2006-01-02 15:04"))
		col.OnClick(func() { a.revealCommit(bl.Hash) }) // after the build, which shows the diff it replaces
	}
	col.Draw(func(pt *ui.Painter, r ui.Rect) {
		rows, lh := annotatedRows(d, sides)
		for _, row := range rows {
			bl, ok := blamed(row)
			if !ok {
				continue
			}
			y := r.Y + float32(row.top)
			if row == hovered {
				pt.Fill(ui.Rect{X: r.X, Y: y, W: r.W, H: float32(lh)}, t.Text.Alpha(0.08), 0)
			}
			label, color := loc.NotCommitted(), t.TextMuted
			if !bl.NotCommitted() {
				label, color = bl.Time.Format("2006-01-02")+"  "+bl.Author, t.Text
				pt.Fill(ui.Rect{X: r.X, Y: y, W: 3, H: float32(lh)}, laneColor(int(bl.Hash[0])+int(bl.Hash[1])), 0)
			}
			pt.Text(r.X+8, y+max(float32(lh)-15, 0)/2, label, 11, color)
		}
	})
}

// annotatedRows are the lines of sides of a diff in view, as the last
// frame laid them out, and the height of a line, which a wrapped line
// spans more than once. A unified diff's old side counts the lines it
// deleted only: the others show on the new side.
func annotatedRows(d *editor.DiffState, sides []editor.DiffSide) ([]annotatedRow, float64) {
	lo := d.ScrollY()
	hi := lo + d.ViewportHeight()
	var rows []annotatedRow
	add := func(side editor.DiffSide, from, to int) {
		// The first line from from whose bottom is in view.
		i := from + sort.Search(to-from, func(k int) bool { return d.LineTop(side, from+k+1) > lo })
		for ; i < to; i++ {
			top := d.LineTop(side, i)
			if top >= hi {
				break
			}
			// Lines folded into a separator take no room, at its top.
			if d.LineTop(side, i+1) == top || i > from && d.LineTop(side, i-1) == top {
				continue
			}
			rows = append(rows, annotatedRow{side, i, top - lo})
		}
	}
	for _, side := range sides {
		switch {
		case side == editor.DiffOld && d.Layout == editor.DiffUnified:
			for _, h := range d.Hunks() {
				add(side, h.OldStart, h.OldEnd)
			}
		case side == editor.DiffOld:
			add(side, 0, d.Old.LineCount())
		default:
			add(side, 0, d.New.LineCount())
		}
	}
	// A line's height is the least distance between two: a wrapped line,
	// or one with a separator below, is further from the next.
	lh := 0.0
	for _, side := range sides {
		for i := 1; i < len(rows); i++ {
			if rows[i].side == side && rows[i-1].side == side && rows[i].line == rows[i-1].line+1 {
				if h := rows[i].top - rows[i-1].top; h > 0 && (lh == 0 || h < lh) {
					lh = h
				}
			}
		}
	}
	if lh == 0 {
		lh = 20
	}
	return rows, lh
}
