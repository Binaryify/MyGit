package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	loc "mygit/locales"
)

// previewLines is how many lines of each side a change's preview shows.
const previewLines = 8

// overview is a strip beside a diff with a mark for each of its changes
// where it is in the file, as VS Code's overview ruler, over a minimap of
// the file's lines, as CodeGlance: a click goes to the change, and
// pointing at it previews its lines. Its left edge drags to make it wider
// or narrower.
func overview(c *ui.Context, d *editor.DiffState) {
	hunks := d.Hunks() // none still keeps the strip, lest the diff widen as the last goes
	t := c.Theme()
	total := max(d.New.LineCount(), 1)
	ui.Column(c).Width(overviewWidth()).AlignSelf(ui.Stretch).Padding(2).BorderWidth(0, 0, 0, 1).BorderColor(t.Border).Children(func() {
		at := 0
		space := func(lines int) {
			if lines > 0 {
				ui.Box(c.Key(-at - 1)).Grow(float32(lines)).Basis(0)
			}
		}
		for i, h := range hunks {
			space(h.NewStart - at)
			n := max(h.NewEnd-h.NewStart, 1)
			code := byte('M')
			switch {
			case h.OldEnd == h.OldStart:
				code = 'A'
			case h.NewEnd == h.NewStart:
				code = 'D'
			}
			mark := ui.ButtonBase(c.Key(i)).Grow(float32(n)).Basis(0).MinHeight(4).Radius(2).
				Label(loc.ChangeOf(i+1, len(hunks))).Tooltip(hunkPreview(d, h)).Cursor(ui.CursorPointer)
			color := changeColor(t, code, false)
			if !mark.Hovered() {
				color = color.Alpha(0.7)
			}
			mark.Background(color)
			if mark.Clicked() {
				d.GoToHunk(i)
			}
			at = h.NewStart + n
		}
		space(total - at)
		overviewGrip(c) // last, over the marks, to take the pointer at its edge
	}).Draw(minimap(t, d, hunks))
}

// minimap paints the strip behind its marks as a minimap of the file, as
// CodeGlance does: each line of the new side as a bar as long as the line
// is, in the muted text color, the lines a change adds or makes in its
// own. Where lines are shorter than a pixel, one of them stands for the
// rest of its row.
//
// It says where the changes are, not where the view is: mygo's diff keeps
// its scroll to itself, so there is no viewport to draw yet.
func minimap(t *ui.Theme, d *editor.DiffState, hunks []editor.DiffHunk) func(p *ui.Painter, r ui.Rect) {
	return func(p *ui.Painter, r ui.Rect) {
		n := d.New.LineCount()
		r = ui.Rect{X: r.X + 3, Y: r.Y + 1, W: r.W - 6, H: r.H - 2}
		if n == 0 || r.W < 4 || r.H < 4 {
			return
		}
		step := max(1, (n+int(r.H)-1)/int(r.H))
		lines := make([]string, 0, int(r.H))
		longest := 1
		for i := 0; i < n; i += step {
			line := d.New.Line(i)
			lines = append(lines, line)
			longest = max(longest, lineLength(line))
		}
		at := 0 // the first change from the line on, its end at most
		for k, line := range lines {
			i := k * step
			for at < len(hunks) && hunks[at].NewEnd <= i {
				at++
			}
			color := t.TextMuted.Alpha(0.45)
			if at < len(hunks) && hunks[at].NewStart <= i {
				code := byte('M')
				switch {
				case hunks[at].OldEnd == hunks[at].OldStart:
					code = 'A'
				case hunks[at].NewEnd == hunks[at].NewStart:
					code = 'D'
				}
				color = changeColor(t, code, false).Alpha(0.8)
			}
			y := r.Y + float32(i)*r.H/float32(n)
			height := max(1, float32(step)*r.H/float32(n)*0.75)
			width := max(1, float32(lineLength(line))/float32(longest)*r.W)
			p.Fill(ui.Rect{X: r.X, Y: y, W: width, H: height}, color, 0)
		}
	}
}

// lineLength is how long a line shows, its indentation counted, a tab as
// four spaces.
func lineLength(line string) int {
	length := 0
	for _, ch := range line {
		if ch == '\t' {
			length += 4
		} else {
			length++
		}
	}
	return length
}

// overviewResizing is set while the pointer drags the overview strip's
// edge, to save its width once it lets go.
var overviewResizing bool

// overviewGrip is the strip's left edge, which the pointer drags to make
// the strip wider or narrower, as the edge of a column.
func overviewGrip(c *ui.Context) {
	t := c.Theme()
	grip := ui.Box(c).Absolute().Left(0).Top(0).Bottom(0).Width(5).
		Cursor(ui.CursorResizeEW).Tooltip(loc.OverviewWidthTip()).Label(loc.OverviewWidthTip())
	if dx, _, held := grip.Dragged(); held && dx != 0 {
		prefs.OverviewW = min(max(overviewWidth()-dx, 8), 64)
		overviewResizing = true
	}
	if overviewResizing && !grip.Dragging() {
		prefs.save()
		overviewResizing = false
	}
	grip.Draw(func(p *ui.Painter, r ui.Rect) {
		if grip.Hovered() || grip.Dragging() {
			p.Fill(ui.Rect{X: r.X + r.W - 1, Y: r.Y, W: 1, H: r.H}, t.Accent, 0)
		}
	})
}

// hunkPreview is a change's lines, as a patch shows them: the old ones
// with -, the new ones with +, a few of each.
func hunkPreview(d *editor.DiffState, h editor.DiffHunk) string {
	var b strings.Builder
	side := func(buf *editor.Buffer, from, to int, sign string) {
		for i := from; i < min(to, from+previewLines); i++ {
			b.WriteString(sign + buf.Line(i) + "\n")
		}
		if to-from > previewLines {
			b.WriteString(loc.AndMore(to-from-previewLines) + "\n")
		}
	}
	side(d.Old, h.OldStart, h.OldEnd, "- ")
	side(d.New, h.NewStart, h.NewEnd, "+ ")
	return strings.TrimSuffix(b.String(), "\n")
}
