package main

import (
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	loc "mygit/locales"
)

// overview is a strip beside a diff with a mark for each of its changes
// where it is in the file, as VS Code's overview ruler, over a minimap of
// the file's lines, as CodeGlance: a click goes to the change, and
// pointing at it shows the lines there. Its left edge drags to make it
// wider or narrower. area is the diff pane the strip sits in, which the
// bubble keeps within; number is the file's line (from 1) that a line of
// the new side (from 0) is, as a patch's lines are not all the file's.
func overview(c *ui.Context, d *editor.DiffState, area ui.Element, number func(int) int) {
	hunks := d.Hunks() // none still keeps the strip, lest the diff widen as the last goes
	t := c.Theme()
	total := max(d.New.LineCount(), 1)
	col := ui.Column(c).Width(overviewWidth()).AlignSelf(ui.Stretch).Padding(2).BorderWidth(0, 0, 0, 1).BorderColor(t.Border).Children(func() {
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
				Label(loc.ChangeOf(i+1, len(hunks))).Cursor(ui.CursorPointer)
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
	})
	col.Draw(minimap(t, d, hunks))
	if x, y, over := col.PointerPosition(); over && !overviewResizing {
		minimapPreview(c, t, d, hunks, col, area, number, x, y)
	}
}

// The preview bubble's shape, in DIPs: the lines it shows, the least
// width it takes, the height of a line and its padding.
const (
	previewRows = 15
	previewMinW = 160
	previewLine = 20
	previewPad  = 6
)

// minimapPreview shows the lines of the new side around the pointer's
// place in the strip, changed or not, as CodeGlance's bubble beside its
// minimap. The bubble is as wide as the diff pane, so lines are not cut
// short the way they were at a fixed width, in the diff's type and its
// colors for code; the changed lines are tinted as in the diff.
func minimapPreview(c *ui.Context, t *ui.Theme, d *editor.DiffState, hunks []editor.DiffHunk, strip, area ui.Element, number func(int) int, x, y float32) {
	b, a := strip.Bounds(), area.Bounds()
	n := d.New.LineCount()
	if n == 0 || b.H < previewLine+2*previewPad || x < 7 { // the left edge drags the strip wider
		return
	}
	left := max(8, a.X+8)
	width := b.X - 8 - left
	if width < previewMinW { // a pane too narrow for the bubble
		return
	}
	line := min(n-1, max(0, int((y-1)/(b.H-2)*float32(n))))
	from := min(max(0, line-previewRows/2), max(0, n-previewRows))
	height := float32(min(previewRows, n)*previewLine + 2*previewPad)
	top := min(max(b.Y+y-height/2, b.Y), b.Y+b.H-height)
	at := 0 // the first change from the line on
	ct := editor.DefaultTheme(t)
	hl := previewHighlighter(d.New)
	if hl != nil {
		hl.Highlight(from+previewRows, 50*time.Millisecond)
	}
	var toks []editor.Token
	ui.Overlay(c, func() {
		ui.Box(c).Absolute().Left(left).Top(top).Width(width).Padding(previewPad).
			Radius(8).Background(t.Surface).Border(1, t.Border).
			Shadow(0, 6, 20, 0, ui.RGB(0, 0, 0).Alpha(0.3)).Children(func() {
			for i := from; i < min(n, from+previewRows); i++ {
				for at < len(hunks) && hunks[at].NewEnd <= i {
					at++
				}
				numColor, tint := t.TextMuted, ui.Color{}
				if at < len(hunks) && hunks[at].NewStart <= i {
					code := byte('M')
					switch {
					case hunks[at].OldEnd == hunks[at].OldStart:
						code = 'A'
					case hunks[at].NewEnd == hunks[at].NewStart:
						code = 'D'
					}
					numColor = changeColor(t, code, false)
					tint = numColor.Alpha(0.15)
				}
				row := ui.Row(c).Key(i).Height(previewLine).Padding(0, 4).Gap(8).AlignItems(ui.Center)
				if i == line {
					row.Background(t.SurfaceHover)
				} else if tint != (ui.Color{}) {
					row.Background(tint)
				}
				text := d.New.Line(i)
				if hl != nil {
					toks, _ = hl.Tokens(i, text, toks[:0])
				}
				row.Children(func() {
					ui.Text(c, strconv.Itoa(number(i))).Font("monospace").FontSize(11).TextColor(numColor).Width(36).TextAlign(ui.End)
					ui.RichText(c, codeSpans(ct, text, toks)...).Font("monospace").FontSize(13).TextColor(ct.Text).SingleLine()
				})
			}
		})
	})
}

// previewSyntax is the highlighter of the lines the minimap's bubble
// shows: that of the buffer's language, as the diff highlights it, for
// the version of its text it was made for.
var previewSyntax struct {
	buf     *editor.Buffer
	version uint64
	hl      editor.Highlighter
}

// previewHighlighter returns a highlighter of b's text as it is, nil for
// plain text.
func previewHighlighter(b *editor.Buffer) editor.Highlighter {
	l := b.Language()
	if l == nil || l.Tokenizer == nil && l.Highlighter == nil {
		return nil
	}
	s, p := b.Snapshot(), &previewSyntax
	if p.buf != b || p.version != s.Version() || p.hl == nil {
		p.buf, p.version = b, s.Version()
		if l.Highlighter != nil {
			p.hl = l.Highlighter(s, func() {})
		} else {
			p.hl = editor.NewTokenHighlighter(l.Tokenizer, s)
		}
	}
	return p.hl
}

// codeSpans are the spans of a line of code colored by its tokens, as
// the theme colors them, a tab as four spaces.
func codeSpans(ct *editor.Theme, line string, toks []editor.Token) []ui.Span {
	spans := make([]ui.Span, 0, len(toks)+1)
	add := func(text string, k editor.TokenKind) {
		if text != "" {
			st := ct.Tokens[k]
			sp := ui.Span{Text: strings.ReplaceAll(text, "\t", "    "), Color: st.Color, Italic: st.Italic}
			if st.Bold {
				sp.Weight = 700
			}
			spans = append(spans, sp)
		}
	}
	at := 0
	for _, tk := range toks {
		end := min(tk.End, len(line))
		if end > at {
			add(line[at:end], tk.Kind)
			at = end
		}
	}
	add(line[at:], editor.TokenText)
	return spans
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
