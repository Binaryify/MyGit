package ui

import (
	"slices"
	"time"
)

// wheelAnimation is how the notches of a mouse wheel scroll: along a short
// ease out, as browsers scroll them, rather than the whole notch at once.
var wheelAnimation = Animation{Duration: 150 * time.Millisecond, Ease: EaseOut}

// wheelGlide is the notches of a wheel on their way through a scroll
// container: x and y go from 0 to as far as all of them scroll, and done
// is how far the content has scrolled with them so far. It moves the
// content by what it went since the last frame, so that what moves the
// content meanwhile, as a List measuring its rows, does not throw it off.
type wheelGlide struct {
	x, y         channel
	doneX, doneY float64
}

// ahead returns how far the notches of the glide have yet to scroll s,
// as scrollBy takes them.
func (g *wheelGlide) ahead() (dx, dy float64) {
	if g == nil {
		return 0, 0
	}
	return g.x.target - g.doneX, g.y.target - g.doneY
}

// wheelMoves reports whether a notch scrolling dx, dy moves container s
// further than the notches on their way there already do.
func wheelMoves(s *state, dx, dy float32) bool {
	ax, ay := s.wheel.ahead()
	if s.rtl {
		dx, ax = -dx, -ax
	}
	mx, my := max(0, s.contentW-float64(s.w)), max(0, s.contentH-float64(s.h))
	if dy != 0 && s.flags&flagScrollY != 0 {
		at := max(0, min(s.scrollY+ay, my))
		if max(0, min(at+float64(dy), my)) != at {
			return true
		}
	}
	if dx != 0 && s.flags&flagScrollX != 0 {
		at := max(0, min(s.scrollX+ax, mx))
		if max(0, min(at+float64(dx), mx)) != at {
			return true
		}
	}
	return false
}

// wheelScroll scrolls container s by a notch of dx, dy smoothly, on top
// of the notches on their way there.
func (rt *engine) wheelScroll(s *state, dx, dy float32) {
	now := rt.now()
	g := s.wheel
	if g == nil {
		g = &wheelGlide{x: channel{at: now}, y: channel{at: now}}
		s.wheel = g
		if !slices.Contains(rt.wheeling, s.id) {
			rt.wheeling = append(rt.wheeling, s.id)
		}
	}
	if s.track != nil {
		s.track.glide = nil // the user takes over from ScrollTo
	}
	if l := s.list; l != nil {
		l.scrolled = false // and from the list's
	}
	g.x.advance(now)
	g.y.advance(now)
	if dx != 0 {
		g.x.retarget(g.x.target+float64(dx), wheelAnimation)
	}
	if dy != 0 {
		g.y.retarget(g.y.target+float64(dy), wheelAnimation)
	}
	rt.requestFrame()
}

// glideWheels moves the containers that notches of a wheel scroll to the
// frame's time.
func (rt *engine) glideWheels(now time.Time) {
	kept := rt.wheeling[:0]
	for _, id := range rt.wheeling {
		s := rt.states[id]
		if s == nil || s.wheel == nil {
			continue
		}
		if t := s.track; t != nil && (t.X != float32(s.scrollX) || t.Y != float32(s.scrollY) || t.glide != nil) ||
			s.list != nil && s.list.scrolled {
			// The app set the offset, or a ScrollTo, of its ScrollState
			// or its ListState: it takes over.
			s.wheel = nil
			continue
		}
		g := s.wheel
		g.x.advance(now)
		g.y.advance(now)
		dx, dy := g.x.value-g.doneX, g.y.value-g.doneY
		g.doneX, g.doneY = g.x.value, g.y.value
		moved := scrollBy(s, float32(dx), float32(dy))
		if !moved && (dx != 0 || dy != 0) || !g.x.moving && !g.y.moving {
			// At the end of the content, or there.
			s.wheel = nil
			continue
		}
		rt.animating = true
		kept = append(kept, id)
	}
	rt.wheeling = kept
}
