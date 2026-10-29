package mygo

import (
	"math"

	"github.com/egoist/mygo/internal/platform"
)

// fitContent is what a window that takes the size of its page's content
// (FlyoutOptions.FitContent, WindowOptions.FitContent) keeps: the limits
// of that size, the zoom its page's scripts were made at, the size it
// last took, and the content's size its page last told, which another
// zoom makes another size. Main thread only.
type fitContent struct {
	min, max platform.Size // DIPs, 0 for none
	zoom     float64
	size     platform.Size
	content  contentSize
	told     bool // the page told content
}

// contentSize is the size of a page's content as its bridge tells it, in
// CSS pixels: its root element's, and the room of the scrollbars the page
// shows. A vertical scrollbar is shown while the window is shorter than
// the content, as it is for a moment as the content grows, before the
// window does: its room counts only where the window can't take the
// content's height. A horizontal one the bridge tells only when the
// content is wider than the root, as at the maximum width.
type contentSize struct {
	Width, Height       float64
	Scrollbar           float64 // the vertical scrollbar's width
	HorizontalScrollbar float64 // the horizontal scrollbar's height
}

// maxFitDIPs bounds the sizes a page tells, so that no page makes an int
// overflow: no display is that large.
const maxFitDIPs = 1 << 16

// fit returns the size in DIPs, within the limits and below maxHeight
// DIPs if it is not 0, of content at zoom, and false for a size no layout
// gives (negative, NaN). Fractions of a pixel take one, so that the
// content fits, but for the error of rounding a layout's fractional
// sizes. A content taller than the height it gets takes the room of its
// vertical scrollbar.
func (f *fitContent) fit(c contentSize, zoom float64, maxHeight int) (platform.Size, bool) {
	if !(c.Width >= 0 && c.Height >= 0) {
		return platform.Size{}, false
	}
	if zoom <= 0 {
		zoom = 1
	}
	dips := func(css float64) int {
		return max(int(math.Ceil(min(css*zoom, maxFitDIPs)-1.0/128)), 1)
	}
	within := func(v, lo, hi int) int {
		v = max(v, lo)
		if hi > 0 {
			v = min(v, hi)
		}
		return v
	}
	maxH := f.max.Height
	if maxHeight > 0 && (maxH == 0 || maxHeight < maxH) {
		maxH = maxHeight
	}
	room := func(v float64) float64 { // NaN and below 0 are none
		if v > 0 {
			return v
		}
		return 0
	}
	width, height := c.Width, c.Height+room(c.HorizontalScrollbar)
	if dips(height) > within(dips(height), f.min.Height, maxH) {
		width += room(c.Scrollbar)
	}
	return platform.Size{
		Width:  within(dips(width), f.min.Width, f.max.Width),
		Height: within(dips(height), f.min.Height, maxH),
	}, true
}

// fitContentSize sizes a window that takes its page's size to the size of
// its content, as its bridge told it, unless that changes nothing. A
// window, as opposed to a flyout, which the platform keeps in the work
// area, keeps in the work area of its display, and changes only in the
// dimension its content changed in: a size the user gave it holds until
// its content changes again. It does not change while maximized or in
// full screen. Main thread only.
func (w *Window) fitContentSize(c contentSize) {
	f, n := w.fit, w.native
	if f == nil || n == nil {
		return
	}
	if w.flyout != nil {
		s, ok := f.fit(c, n.Zoom(), 0)
		if !ok {
			return
		}
		f.content, f.told = c, true
		if s == f.size {
			return
		}
		f.size = s
		if s != w.flyoutSize {
			w.setFlyoutSize(s.Width, s.Height)
		}
		return
	}
	bounds, content := n.Bounds(), n.ContentBounds()
	maxW, maxH := 0, 0
	if wa := platform.WorkAreaFor(bounds, backend().Screen().Displays()); wa.Width > 0 && wa.Height > 0 {
		maxW = max(wa.Width-(bounds.Width-content.Width), 1)
		maxH = max(wa.Height-(bounds.Height-content.Height), 1)
	}
	s, ok := f.fit(c, n.Zoom(), maxH)
	if !ok {
		return
	}
	f.content, f.told = c, true
	if maxW > 0 {
		s.Width = min(s.Width, maxW)
	}
	if s == f.size || n.IsMaximized() || n.IsFullScreen() {
		return
	}
	b := content
	if s.Width != f.size.Width {
		b.Width = s.Width
	}
	if s.Height != f.size.Height {
		b.Height = s.Height
	}
	f.size = s
	if b != content {
		n.SetContentBounds(b)
	}
}
