package main

import (
	"slices"
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
// pointing at it shows the lines there, and it marks the rows in view. Its
// left edge drags to make it wider or narrower. area is the diff pane the
// strip sits in, which the bubble keeps within; starts are the lines a
// patch's hunks start at (patchStarts), nil for a diff of whole texts;
// number is the file's line (from 1) that a line of the new side (from 0)
// is, as a patch's lines are not all the file's.
func overview(c *ui.Context, d *editor.DiffState, area ui.Element, starts []int, number func(int) int) {
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
		minimapPreview(c, t, d, hunks, starts, col, area, number, x, y)
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
func minimapPreview(c *ui.Context, t *ui.Theme, d *editor.DiffState, hunks []editor.DiffHunk, starts []int, strip, area ui.Element, number func(int) int, x, y float32) {
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
	hl := previewHighlighter(d.New, starts)
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
	starts  []int
	hl      editor.Highlighter
}

// previewHighlighter returns a highlighter of b's text as it is, nil for
// plain text. starts are the lines a patch's hunks start at, which each
// start afresh as the diff's own highlighter does.
func previewHighlighter(b *editor.Buffer, starts []int) editor.Highlighter {
	l := b.Language()
	if l == nil || l.Tokenizer == nil && l.Highlighter == nil {
		return nil
	}
	s, p := b.Snapshot(), &previewSyntax
	if p.buf != b || p.version != s.Version() || p.hl == nil || !slices.Equal(p.starts, starts) {
		p.buf, p.version, p.starts = b, s.Version(), slices.Clone(starts)
		switch {
		case l.Highlighter != nil:
			p.hl = l.Highlighter(s, func() {})
		case len(starts) > 0:
			p.hl = &hunkHighlighter{t: l.Tokenizer, text: s, starts: p.starts}
		default:
			p.hl = editor.NewTokenHighlighter(l.Tokenizer, s)
		}
	}
	return p.hl
}

// hunkHighlighter highlights a patch's lines as mygo's default highlighter
// does, a line at a time from the state the line before ends in, but each
// hunk from the start: the file's lines between two hunks are not there.
// mygo's own highlighter does as much for a diff, its restarts moving with
// the text's edits; this one is for the bubble, which reads the text alone.
type hunkHighlighter struct {
	t       editor.Tokenizer
	text    editor.Snapshot
	starts  []int    // the lines the hunks start at, in order
	states  []uint64 // the state line i starts in, for the lines known
	scratch []editor.Token
}

func (h *hunkHighlighter) Edit(_, new editor.Snapshot, _ []editor.Change) {
	h.text = new
	h.states = h.states[:0]
}

func (h *hunkHighlighter) Highlight(last int, budget time.Duration) bool {
	last = min(last, h.text.LineCount()-1)
	if len(h.states) == 0 {
		h.states = append(h.states, 0)
	}
	start := time.Now()
	for i := len(h.states) - 1; i < last; i++ {
		if i&63 == 63 && time.Since(start) > budget {
			return false
		}
		var end uint64
		h.scratch, end = h.t.Tokenize(h.text.Line(i), h.states[i], h.scratch[:0])
		if _, ok := slices.BinarySearch(h.starts, i+1); ok {
			end = 0
		}
		h.states = append(h.states, end)
	}
	return true
}

func (h *hunkHighlighter) Tokens(i int, text string, dst []editor.Token) ([]editor.Token, bool) {
	state, ok := uint64(0), i < len(h.states)
	if ok {
		state = h.states[i]
	}
	if len(text) > 10000 { // left plain, as mygo's highlighter leaves it
		return dst, ok
	}
	dst, _ = h.t.Tokenize(text, state, dst)
	return dst, ok
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
// own, over a box on the rows in view. Where lines are shorter than a
// pixel, one of them stands for the rest of its row.
func minimap(t *ui.Theme, d *editor.DiffState, hunks []editor.DiffHunk) func(p *ui.Painter, r ui.Rect) {
	return func(p *ui.Painter, r ui.Rect) {
		n := d.New.LineCount()
		r = minimapInner(r)
		if n == 0 || r.W < 4 || r.H < 4 {
			return
		}
		total := diffHeight(d)
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
			// The rows as the diff laid them out: a separator, or a line
			// wrapped onto two, is not the same height as the next.
			y := minimapY(r.Y, r.H, d.LineTop(editor.DiffNew, i), total)
			end := minimapY(r.Y, r.H, d.LineTop(editor.DiffNew, min(i+step, n)), total)
			height := max(1, (end-y)*0.75)
			width := max(1, float32(lineLength(line))/float32(longest)*r.W)
			p.Fill(ui.Rect{X: r.X, Y: y, W: width, H: height}, color, 0)
		}
		if box := viewportBox(r, d.ScrollY(), d.ViewportHeight(), total); box.H > 0 {
			p.Fill(box, t.Text.Alpha(0.1), 2)
			p.Stroke(box, t.TextMuted.Alpha(0.6), 2, 1)
		}
	}
}

// minimapInner is the part of a strip the minimap draws in, clear of its edge.
func minimapInner(r ui.Rect) ui.Rect {
	return ui.Rect{X: r.X + 3, Y: r.Y + 1, W: r.W - 6, H: r.H - 2}
}

// diffHeight is the height of the diff's rows, in DIPs, as its last frame
// laid them out: LineTop of the line after the new side's last counts the
// separators, the annotations and the room a split view leaves across from
// the other side, as a minimap scales them.
func diffHeight(d *editor.DiffState) float64 {
	n := d.New.LineCount()
	if h := d.LineTop(editor.DiffNew, n); h > 0 {
		return h
	}
	return float64(max(n, 1))
}

// minimapY is where a row above the first of a minimap's rows shows in it,
// the minimap h high from y, the diff's rows total DIPs.
func minimapY(y, h float32, above, total float64) float32 {
	if total <= 0 {
		return y
	}
	return y + float32(min(max(above/total, 0), 1))*h
}

// viewportBox is the box a minimap of a strip r draws over the rows in view,
// those from scroll to scroll+viewport of total DIPs: at least 3 high, so a
// few lines of a long file show, and within r.
func viewportBox(r ui.Rect, scroll, viewport, total float64) ui.Rect {
	if total <= 0 {
		return ui.Rect{}
	}
	top := minimapY(r.Y, r.H, scroll, total)
	bottom := minimapY(r.Y, r.H, scroll+viewport, total)
	if bottom-top < 3 {
		bottom = top + 3
	}
	if bottom > r.Y+r.H {
		top, bottom = r.Y+r.H-3, r.Y+r.H
	}
	if top < r.Y {
		top, bottom = r.Y, r.Y+3
	}
	return ui.Rect{X: r.X, Y: top, W: r.W, H: bottom - top}
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
