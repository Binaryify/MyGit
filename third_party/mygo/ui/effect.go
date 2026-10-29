package ui

import (
	"math"

	"github.com/egoist/mygo/internal/scene"
)

// Effect draws an element from what it renders, as SwiftUI's colorEffect,
// distortionEffect and layerEffect do: package ui/shader's effects
// (shader.Effect), which change each pixel's color, move the pixels, or
// read the content anywhere. The element and its children render into a
// bitmap, which the effect then draws over the element's box, in its
// place. Hit testing, focus and accessibility do not change: only what
// shows.
type Effect interface {
	// EffectMargin returns how far beyond the element's box, in DIPs
	// across and down, the effect reads its content: the content renders
	// that much around the box, as shadows painted beyond it, and reads as
	// transparent further away.
	EffectMargin() Point
	// PaintEffect paints the element, its box in DIPs, from content, the
	// element and its children rendered with p's scale, the box and the
	// margin around it: inset is where the box's corner lies in content,
	// in its pixels.
	PaintEffect(p *Painter, box Rect, content *Bitmap, inset Point)
}

// Effect draws the element, with its children, through fx, an effect of
// package ui/shader, in place of drawing it as it is; nil draws it as it
// is:
//
//	ui.Text(c, "Wavy").Effect(wave.Effect(shader.Options{
//		Uniforms:        map[string]any{"Time": t},
//		MaxSampleOffset: ui.Point{Y: 8},
//	}))
//
// What the element renders, with its children, shadows and what its Draw
// callbacks paint, renders into a bitmap of the GPU, the size of its box
// and the effect's margin (Effect.EffectMargin) around it, every frame
// the element paints, which the effect then draws over the box and the
// margin, transformed, clipped and faded as the element would be. Web views
// inside the element do not show through an effect.
func (e *node) Effect(fx Effect) *node { e.effect = fx; return e }

// maxEffectLayer is the largest side of an effect's bitmap, in pixels,
// which every GPU renderer's textures allow.
const maxEffectLayer = 8192

// effectLayerStep is the multiple of pixels an effect's bitmap is made
// in, as Chromium makes the textures of its render passes: one made a
// little larger than the content holds it as the element's edges cross
// pixels on the way of an animation, or as its text grows, rather than a
// texture made for each size. A bitmap two steps larger than the content
// is made smaller again.
const effectLayerStep = 64

// layerSide is the side of an effect's bitmap holding n pixels: n rounded
// up to a step.
func layerSide(n int) int {
	return min((n+effectLayerStep-1)/effectLayerStep*effectLayerStep, maxEffectLayer)
}

// paintEffect paints e through its effect, or blurred (Blur), and reports
// whether it did: e's subtree rendered into its state's canvas, then the
// effect over its box, or the canvas blurred. It paints nothing of e
// outside p's clip.
func (p *Painter) paintEffect(e *node) bool {
	if e.effect == nil && e.blur <= 0 && e.cache == nil || e.effecting {
		return false
	}
	// A blur reaches 3 sigma: as far as it shows.
	m := Point{3 * e.blur, 3 * e.blur}
	if e.effect != nil {
		m = e.effect.EffectMargin()
	}
	m.X, m.Y = max(m.X, 0), max(m.Y, 0)
	box := Rect{e.x, e.y, e.w, e.h}
	from := Rect{box.X - m.X, box.Y - m.Y, box.W + 2*m.X, box.H + 2*m.Y}
	if e.cache != nil && e.effect == nil {
		// What the element paints, blurred as far as the blur shows.
		from = e.cache.area(e)
		from = Rect{from.X - m.X, from.Y - m.Y, from.W + 2*m.X, from.H + 2*m.Y}
	}
	if box.W <= 0 || box.H <= 0 || !p.visible(from, 4) {
		return true
	}
	s := p.scale
	// The content's pixels, from the box grown by the margin (or the
	// area a kept layer takes), on the pixel grid.
	x0 := float32(math.Floor(float64(from.X * s)))
	y0 := float32(math.Floor(float64(from.Y * s)))
	x1 := float32(math.Ceil(float64((from.X + from.W) * s)))
	y1 := float32(math.Ceil(float64((from.Y + from.H) * s)))
	w, h := int(x1-x0), int(y1-y0)
	if w <= 0 || h <= 0 || w > maxEffectLayer || h > maxEffectLayer {
		return false // too large to render apart: as it is
	}
	cv := e.st.effectLayer
	switch {
	case cv == nil:
		cv = NewCanvas(layerSide(w), layerSide(h))
		e.st.effectLayer = cv
	case cv.img.W < w || cv.img.H < h || cv.img.W-w >= 2*effectLayerStep || cv.img.H-h >= 2*effectLayerStep:
		cv.resize(layerSide(w), layerSide(h))
	}
	// Drawn anew whenever it shows, unless kept (Cache).
	cv.img.Transient = e.cache == nil
	dx, dy := -x0/s, -y0/s
	world := e.world
	render := func(cp *Painter) {
		e.effecting = true
		// The subtree in e's own space, moved to the bitmap's corner:
		// positions shift, and the transforms of those transformed lose
		// e's own, as a layer of e's content.
		shift(e, dx, dy)
		if world.Set {
			unworld(e, world, dx, dy)
		}
		cp.element(e)
		if world.Set {
			restoreVisual(e)
		}
		shift(e, -dx, -dy)
		e.effecting = false
	}
	if e.cache != nil {
		e.cache.draw(cv, p, s, w, h, render)
	} else {
		cv.drawPart(p, s, w, h, render)
	}
	if e.effect != nil {
		// The content is the bitmap's top left w×h pixels: the effect
		// draws over those, and reads it transparent beyond.
		e.effect.PaintEffect(p, box, cv.region(w, h), Point{box.X*s - x0, box.Y*s - y0})
		return true
	}
	// The canvas, blurred unless kept alone, in its pixels, where it was
	// drawn from.
	area := Rect{X: x0 / s, Y: y0 / s, W: float32(w) / s, H: float32(h) / s}
	rect := p.snap(area)
	if p.transform.Set {
		rect = scene.Rect{X: x0, Y: y0, W: float32(w), H: float32(h)}
	}
	p.add(scene.Op{Kind: scene.OpImage, Rect: rect, Image: cv.img, Src: scene.Rect{W: float32(w), H: float32(h)}, Opacity: p.opacity, Blur: e.blur * s})
	return true
}

// unworld takes off the transform w of e's subtree, laid out as it moved
// by (dx, dy): each one's maps its layout, moved, as w's inverse does
// what it mapped before, so that e itself draws untransformed.
func unworld(e *node, w scene.Affine, dx, dy float32) {
	inv, ok := w.Inverse()
	if !ok {
		return
	}
	pre, post := scene.Translation(dx, dy).Mul(inv), scene.Translation(-dx, -dy)
	var walk func(n *node)
	walk = func(n *node) {
		if n.world.Set {
			m := pre.Mul(n.world).Mul(post)
			if nearIdentity(m) {
				m = scene.Affine{}
			}
			n.world = m
		}
		for ch := n.first; ch != nil; ch = ch.next {
			walk(ch)
		}
	}
	walk(e)
}

// nearIdentity reports whether m is the identity within rounding, as
// undoing a transform leaves it.
func nearIdentity(m scene.Affine) bool {
	const eps = 1e-4
	near := func(a, b float32) bool { return abs32(a-b) < eps }
	return near(m.A, 1) && near(m.B, 0) && near(m.C, 0) && near(m.D, 1) && near(m.X, 0) && near(m.Y, 0) && near(m.G, 0) && near(m.H, 0)
}
