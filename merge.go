package main

import (
	"context"
	"math"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mygit/internal/conflict"
	"mygit/internal/git"
	loc "mygit/locales"
)

// The panes of the merge window, left to right.
const (
	paneOurs = iota
	paneResult
	paneTheirs
)

// paneOf is the pane of a side.
var paneOf = [2]int{conflict.Ours: paneOurs, conflict.Theirs: paneTheirs}

// merger resolves a file in conflict, as IntelliJ's merge window: our
// version on the left, theirs on the right, and between them the result,
// which starts as the base with the changes of one side alone taken in.
// The panes scroll together, the chunks each side changed are colored and
// joined to the result's, and each side's change to a conflict is taken
// with ≫ or ≪ or dismissed with ✕, or the result edited by hand.
type merger struct {
	path           string
	model          *conflict.Merge
	panes          [3]*editor.State
	focus          ui.Handle // the result editor, to give the keyboard back to after a button
	scroll         [3]ui.ScrollState
	shown          [3]float32 // the scroll each pane was given, to tell the one scrolled
	viewH          float32    // the panes' height
	lh             float32    // the editors' line height
	version        uint64     // the result's, that the model has
	caret          int        // the result's, to scroll it into view as it moves
	cur            int        // the chunk shown
	opening        bool       // to go to the first chunk once the panes are laid out
	note           string     // what the magic wand did
	split1, split2 float32
}

type mergeTexts struct{ base, ours, theirs string }

// showConflict opens the merge of a file in conflict.
func (a *app) showConflict(c git.Change) {
	a.detail = detail{change: &c, gen: a.detail.gen + 1, loading: true}
	gen := a.detail.gen
	work(a, func(ctx context.Context) (mergeTexts, error) {
		return mergeTexts{a.repo.ShowStage(ctx, c.Path, git.BaseStage), a.repo.ShowStage(ctx, c.Path, git.OursStage),
			a.repo.ShowStage(ctx, c.Path, git.TheirsStage)}, nil
	}, func(t mergeTexts, err error) {
		if gen != a.detail.gen {
			return
		}
		a.detail.loading = false
		if err != nil {
			a.errMsg = err.Error()
			return
		}
		a.detail.merge = newMerger(c.Path, t)
	})
}

func newMerger(path string, t mergeTexts) *merger {
	lf := func(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }
	m := &merger{path: path, model: conflict.New(lf(t.base), lf(t.ours), lf(t.theirs)), lh: 20, split1: 30, split2: 57}
	lang := editor.LanguageFor(path)
	for i, text := range []string{t.ours, m.model.Result(), t.theirs} {
		b := editor.NewBuffer(text)
		if lang != nil {
			b.SetLanguage(lang)
		}
		m.panes[i] = &editor.State{Buffer: b, ReadOnly: i != paneResult, NoActiveLine: i != paneResult, FitContent: true, NoFolding: true}
	}
	if strings.Contains(t.ours, "\r\n") { // the result's lines end as the file's did
		m.panes[paneResult].Buffer.SetLineEnding("\r\n")
	}
	m.version = m.panes[paneResult].Buffer.Version()
	m.cur = m.next(-1, 1)
	m.opening = true
	return m
}

// sync takes the result's edits by hand into the model.
func (m *merger) sync() {
	b := m.panes[paneResult].Buffer
	if v := b.Version(); v != m.version {
		m.version = v
		m.model.Sync(b.String())
	}
}

// edit makes an edit of the model's in the result.
func (m *merger) edit(e conflict.Edit) {
	b := m.panes[paneResult].Buffer
	b.Edit(editor.Edit{Start: e.Start, End: e.End, Text: e.Text})
	m.version = b.Version()
	m.caret = m.panes[paneResult].Cursor() // moved by the edit, not to scroll to
}

// rangeIn returns the lines of a chunk in a pane.
func rangeIn(c *conflict.Chunk, pane int) conflict.Range {
	switch pane {
	case paneOurs:
		return c.Sides[conflict.Ours]
	case paneTheirs:
		return c.Sides[conflict.Theirs]
	}
	return c.Result
}

// next returns the chunk left to resolve after i going dir, wrapping
// around, or -1 for none.
func (m *merger) next(i, dir int) int {
	n := len(m.model.Chunks)
	for k := 1; k <= n; k++ {
		j := ((i+dir*k)%n + n) % n
		if !m.model.Chunks[j].Resolved() {
			return j
		}
	}
	return -1
}

// step shows the next chunk left to resolve, or with back the one before.
func (m *merger) step(back bool) {
	dir := 1
	if back {
		dir = -1
	}
	m.goTo(m.next(m.cur, dir))
}

// goTo shows chunk i a third down the panes, each pane's lines of it at
// the same height.
func (m *merger) goTo(i int) {
	if i < 0 {
		return
	}
	m.cur = i
	ch := &m.model.Chunks[i]
	for p := range m.scroll {
		y := float32(rangeIn(ch, p).Start)*m.lh - m.viewH/3
		if p != paneResult {
			y = min(y, m.scroll[p].MaxY)
		}
		m.scroll[p].Y = max(0, y)
		m.shown[p] = m.scroll[p].Y // not scrolled by the user, to follow
	}
	r := ch.Result
	ed := m.panes[paneResult]
	off := ed.Buffer.Offset(editor.Pos{Line: r.Start})
	ed.Select(off, off)
	m.caret = off
}

// apply takes a side's change to chunk i into the result.
func (m *merger) apply(i int, s conflict.Side) {
	m.sync()
	m.edit(m.model.Apply(i, s))
	m.cur = i
	m.focus.Focus() // for ⌘Z to undo it
}

// ignore dismisses a side's change to chunk i.
func (m *merger) ignore(i int, s conflict.Side) {
	m.model.Ignore(i, s)
	m.cur = i
}

// take resolves the chunk shown with a side's change, or both's.
func (m *merger) take(sides ...conflict.Side) {
	i := m.cur
	if i < 0 || i >= len(m.model.Chunks) || m.model.Chunks[i].Resolved() {
		return
	}
	for _, s := range sides {
		m.apply(i, s)
	}
	for s := range 2 {
		m.ignore(i, conflict.Side(s))
	}
	m.goTo(m.next(i, 1))
}

// resolveSimple resolves the conflicts whose sides changed different
// words, as IntelliJ's magic wand.
func (m *merger) resolveSimple() {
	m.sync()
	n := 0
	for i := range m.model.Chunks {
		if e, ok := m.model.ResolveSimple(i); ok {
			m.edit(e)
			n++
		}
	}
	m.note = loc.ResolvedSimple(n)
	if n > 0 {
		m.goTo(m.next(m.cur, 1))
	}
}

// followScroll scrolls the panes with the one scrolled, by the line in
// their middle, and the result to its caret as it moves.
func (m *merger) followScroll() {
	if m.opening && m.viewH > 0 { // scrolled before, it would land anywhere
		m.opening = false
		m.goTo(m.cur)
	}
	if caret := m.panes[paneResult].Cursor(); caret != m.caret {
		// From where the result was: focusing the editor scrolls it whole
		// into view, which isn't where its caret is.
		m.caret = caret
		top := float32(m.panes[paneResult].Buffer.Pos(caret).Line) * m.lh
		s := &m.scroll[paneResult]
		s.Y = m.shown[paneResult]
		if top < s.Y {
			s.Y = top
		} else if top+2*m.lh > s.Y+m.viewH {
			s.Y = top + 2*m.lh - m.viewH
		}
	}
	lead := -1
	for p := range m.scroll {
		if m.scroll[p].Y != m.shown[p] {
			lead = p
			break
		}
	}
	if lead >= 0 && m.lh > 0 {
		mid := float64((m.scroll[lead].Y + m.viewH/2) / m.lh)
		for p := range m.scroll {
			if p != lead {
				y := float32(m.mapLine(mid, lead, p))*m.lh - m.viewH/2
				m.scroll[p].Y = max(0, min(y, m.scroll[p].MaxY))
			}
		}
	}
	for p := range m.scroll {
		m.shown[p] = m.scroll[p].Y
	}
}

// mapLine returns the place in pane to of the place line in pane from,
// lines as far through a chunk in one as in the other.
func (m *merger) mapLine(line float64, from, to int) float64 {
	a0, b0 := 0.0, 0.0
	point := func(a, b int) (float64, bool) {
		fa, fb := float64(a), float64(b)
		if line <= fa {
			if fa == a0 {
				return fb, true
			}
			return b0 + (line-a0)/(fa-a0)*(fb-b0), true
		}
		a0, b0 = fa, fb
		return 0, false
	}
	for i := range m.model.Chunks {
		c := &m.model.Chunks[i]
		ra, rb := rangeIn(c, from), rangeIn(c, to)
		if y, ok := point(ra.Start, rb.Start); ok {
			return y
		}
		if y, ok := point(ra.End, rb.End); ok {
			return y
		}
	}
	return b0 + line - a0
}

// chunkColor is the color of a chunk, as IntelliJ's: red for a conflict,
// else green for lines added, gray for lines removed, and blue for lines
// changed by the side that changed it.
func chunkColor(t *ui.Theme, ct *editor.Theme, c *conflict.Chunk) ui.Color {
	if c.Conflict {
		return t.Danger
	}
	s := conflict.Ours
	if c.State[s] == conflict.Unchanged {
		s = conflict.Theirs
	}
	switch {
	case c.Base.Len() == 0:
		return ct.Added
	case c.Sides[s].Len() == 0:
		return t.TextMuted
	}
	return ct.Modified
}

// shows reports whether a pane marks a chunk: the result marks them all,
// a side those it changed.
func shows(c *conflict.Chunk, pane int) bool {
	switch pane {
	case paneOurs:
		return c.State[conflict.Ours] != conflict.Unchanged
	case paneTheirs:
		return c.State[conflict.Theirs] != conflict.Unchanged
	}
	return true
}

// alpha is how strongly a chunk shows: the one shown most, those left to
// resolve more than those resolved.
func (m *merger) alpha(i int) float32 {
	switch {
	case m.model.Chunks[i].Resolved():
		return 0.07
	case i == m.cur:
		return 0.32
	}
	return 0.18
}

func (a *app) mergeView(c *ui.Context, m *merger) {
	t := c.Theme()
	ct := editor.DefaultTheme(t)
	m.sync()
	m.followScroll()
	if m.opening {
		c.Invalidate() // a frame more, laid out, to go to the first chunk
	}
	left, right := loc.MergeOurs(), loc.MergeTheirs()
	if a.op == git.Rebasing { // rebasing, HEAD is the branch rebased onto
		left, right = loc.MergeUpstream(), loc.MergeYours()
	}
	for p, st := range m.panes {
		st.LineBackgrounds = st.LineBackgrounds[:0]
		for i := range m.model.Chunks {
			ch := &m.model.Chunks[i]
			if r := rangeIn(ch, p); shows(ch, p) && r.Len() > 0 {
				st.LineBackgrounds = append(st.LineBackgrounds, editor.LineBackground{First: r.Start, Last: r.End - 1,
					Color: chunkColor(t, ct, ch).Alpha(m.alpha(i))})
			}
		}
	}
	unresolved := m.model.Unresolved()
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Padding(6, 12).Gap(6).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
			ui.Text(c, m.path).Bold().SingleLine().Shrink(0)
			if unresolved > 0 {
				n := 0
				for i := range min(m.cur+1, len(m.model.Chunks)) {
					if !m.model.Chunks[i].Resolved() {
						n++
					}
				}
				ui.Text(c, loc.ConflictOf(max(n, 1), unresolved)).TextColor(t.TextMuted).SingleLine().Shrink(0)
				if iconButton(c, "up", loc.PrevConflict()+" "+shiftKey("F7")) {
					m.step(true)
				}
				if iconButton(c, "chevron-down", loc.NextConflict()+" F7") {
					m.step(false)
				}
				if ui.Button(c, loc.TakeLeft()).Clicked() {
					m.take(conflict.Ours)
				}
				if ui.Button(c, loc.TakeRight()).Clicked() {
					m.take(conflict.Theirs)
				}
				if ui.Button(c, loc.TakeBoth()).Clicked() {
					m.take(conflict.Ours, conflict.Theirs)
				}
				if ui.Button(c, "✨").Label(loc.ResolveSimple()).Tooltip(loc.ResolveSimple() + ": " + loc.ResolveSimpleHint()).Clicked() {
					m.resolveSimple()
				}
			} else {
				ui.Text(c, loc.NoConflictsLeft()).TextColor(t.TextMuted)
			}
			if m.note != "" {
				ui.Text(c, m.note).TextColor(t.TextMuted).SingleLine()
			}
			ui.Box(c).Grow(1)
			iconMenu(c, "more", loc.WholeFile(), func(menu *ui.Menu) {
				if menu.Item(loc.TakeAllLeft(left)).Chosen() {
					a.takeSide(m.path, git.OursStage)
				}
				if menu.Item(loc.TakeAllRight(right)).Chosen() {
					a.takeSide(m.path, git.TheirsStage)
				}
			})
			if ui.PrimaryButton(c, loc.SaveResolved()).Disabled(a.busy != "").Clicked() {
				a.saveResolution(m)
			}
		})
		title := func(s string) { ui.Text(c, s).TextColor(t.TextMuted).Padding(4, 10).SingleLine() }
		side := func(name string, p int) {
			ui.Column(c).Fill().Children(func() {
				title(name)
				m.paneView(c, p)
			})
		}
		ui.SplitPercent(c.Key("merge1"), &m.split1, func() { side(left, paneOurs) }, func() {
			ui.SplitPercent(c.Key("merge2"), &m.split2, func() {
				ui.Column(c).Fill().Children(func() {
					title(loc.MergeResult())
					ui.Row(c).Grow(1).FillWidth().Children(func() {
						m.gutter(c, conflict.Ours)
						m.paneView(c, paneResult)
						m.gutter(c, conflict.Theirs)
					})
				})
			}, func() { side(right, paneTheirs) }).Fill()
		}).Grow(1)
	})
}

// paneView is a pane's editor, as high as its lines, in a scroll view of
// the merger's, with a line where it has a chunk of no lines.
func (m *merger) paneView(c *ui.Context, p int) {
	t := c.Theme()
	ct := editor.DefaultTheme(t)
	s := ui.Scroll(c.Key(p)).TrackScroll(&m.scroll[p]).Grow(1)
	if p == paneResult { // in a row between the gutters
		s.FillHeight()
		m.viewH = s.Bounds().H
	} else {
		s.FillWidth()
	}
	s.DrawOver(func(pt *ui.Painter, r ui.Rect) {
		// The editors' line height, as they round it to the pixels.
		scale := pt.Scale()
		m.lh = float32(math.Round(float64(13*1.5*scale))) / scale
		for i := range m.model.Chunks {
			ch := &m.model.Chunks[i]
			if rg := rangeIn(ch, p); shows(ch, p) && rg.Len() == 0 {
				y := r.Y + float32(rg.Start)*m.lh - m.scroll[p].Y
				pt.Line(r.X, y, r.X+r.W, y, 1.5, chunkColor(t, ct, ch).Alpha(min(1, 3*m.alpha(i))))
			}
		}
	}).Children(func() {
		ed := editor.View(c.Key("editor"), m.panes[p]).FillWidth()
		if p == paneResult {
			ed.Bind(&m.focus)
		}
	})
}

// gutter joins a side's chunks to the result's, between their panes, with
// buttons to take or dismiss the side's changes left to choose.
func (m *merger) gutter(c *ui.Context, s conflict.Side) {
	t := c.Theme()
	ct := editor.DefaultTheme(t)
	p := paneOf[s]
	const w = 52
	ui.Box(c.Key(p)).Width(w).FillHeight().Clip().Draw(func(pt *ui.Painter, r ui.Rect) {
		for i := range m.model.Chunks {
			ch := &m.model.Chunks[i]
			if !shows(ch, p) {
				continue
			}
			side, res := rangeIn(ch, p), ch.Result
			y := func(line int, pane int) float32 { return r.Y + float32(line)*m.lh - m.scroll[pane].Y }
			// The side's edge is the left for ours, the right for theirs.
			x0, x1 := r.X, r.X+r.W
			if s == conflict.Theirs {
				x0, x1 = x1, x0
			}
			st, sb, rt, rb := y(side.Start, p), y(side.End, p), y(res.Start, paneResult), y(res.End, paneResult)
			mid := (x0 + x1) / 2
			var path ui.Path
			path.MoveTo(x0, st).CubeTo(mid, st, mid, rt, x1, rt).LineTo(x1, rb).CubeTo(mid, rb, mid, sb, x0, sb).Close()
			col := chunkColor(t, ct, ch)
			pt.FillPath(&path, col.Alpha(m.alpha(i)))
			var edge ui.Path
			edge.MoveTo(x0, st).CubeTo(mid, st, mid, rt, x1, rt)
			pt.StrokePath(&edge, 1, col.Alpha(min(1, 3*m.alpha(i))))
			edge.Reset().MoveTo(x0, sb).CubeTo(mid, sb, mid, rb, x1, rb)
			pt.StrokePath(&edge, 1, col.Alpha(min(1, 3*m.alpha(i))))
		}
	}).Children(func() {
		for i := range m.model.Chunks {
			ch := &m.model.Chunks[i]
			if ch.State[s] != conflict.Pending {
				continue
			}
			top := float32(rangeIn(ch, p).Start)*m.lh - m.scroll[p].Y
			if top < -m.lh || top > m.viewH {
				continue
			}
			apply, ignore := "»", "✕"
			if s == conflict.Theirs {
				apply = "«"
			}
			row := ui.Row(c.Key(i)).Absolute().Top(top + 1).Gap(1)
			if s == conflict.Ours { // by their side
				row.Left(1)
			} else {
				row.Right(1)
			}
			row.Children(func() {
				button := func(label, tip string) bool {
					return ui.ButtonBase(c.Key(tip)).Size(16, m.lh-2).Radius(4).Center().Tooltip(tip).Label(tip).
						Cursor(ui.CursorPointer).Background(t.Background).Children(func() {
						ui.Text(c, label).FontSize(12).TextColor(chunkColor(t, ct, ch))
					}).Clicked()
				}
				first, second := func() {
					if button(apply, loc.ApplyChange()) {
						m.apply(i, s)
					}
				}, func() {
					if button(ignore, loc.IgnoreChange()) {
						m.ignore(i, s)
					}
				}
				if s == conflict.Theirs { // ✕ outermost
					first, second = second, first
				}
				first()
				second()
			})
		}
	})
}

func (a *app) takeSide(path string, s git.Stage) {
	a.run(loc.OpResolve(), func(ctx context.Context) error { return a.repo.TakeSide(ctx, path, s) }, func() { a.detail = detail{} })
}

// saveResolution writes the result and marks the file resolved, asking
// first while conflicts are left in it.
func (a *app) saveResolution(m *merger) {
	save := func() {
		var b strings.Builder
		m.panes[paneResult].Buffer.WriteTo(&b) // with the file's line ends
		text := b.String()
		a.run(loc.OpResolve(), func(ctx context.Context) error { return a.repo.Resolve(ctx, m.path, text) }, func() { a.detail = detail{} })
	}
	m.sync()
	if n := m.model.Unresolved(); n > 0 {
		a.ask(loc.ConflictsLeftTitle(), loc.ConflictsLeftMessage(n), loc.SaveAnyway(), save)
		return
	}
	save()
}
