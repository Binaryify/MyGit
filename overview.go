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
// where it is in the file, as VS Code's overview ruler: a click goes to
// the change, and pointing at it previews its lines.
func overview(c *ui.Context, d *editor.DiffState) {
	hunks := d.Hunks() // none still keeps the strip, lest the diff widen as the last goes
	t := c.Theme()
	total := max(d.New.LineCount(), 1)
	ui.Column(c).Width(14).AlignSelf(ui.Stretch).Padding(2).BorderWidth(0, 0, 0, 1).BorderColor(t.Border).Children(func() {
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
