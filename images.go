package main

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/egoist/mygo/ui"

	"mygit/internal/git"
	loc "mygit/locales"
)

// isBinary tells whether a file's text is binary, as git tells it: a NUL
// among its first 8000 bytes.
func isBinary(text string) bool {
	return strings.IndexByte(text[:min(len(text), 8000)], 0) >= 0
}

// isImage tells whether a file is a picture the diff can show as one.
func isImage(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
		return true
	}
	return false
}

// picture is a side of an image's change: the picture and how big its
// file is, or nil where the side has none, as an image added.
type picture struct {
	bitmap *ui.Bitmap
	bytes  int
}

// images are a changed picture's sides, before and after, and how they
// are zoomed and panned.
type images struct {
	old, new *picture
	err      string
	zoom     imageZoom
}

// imageZoom is how far the pictures are zoomed past fitting their pane, 0
// as 1, and panned from its middle, in DIPs (#50).
type imageZoom struct {
	scale, x, y float32
	canvas      ui.Handle
}

// The least and the most a picture zooms past fitting its pane, and a
// wheel's notch or a button's step.
const (
	zoomMin  = 0.1
	zoomMax  = 32
	zoomStep = 1.25
)

func (z *imageZoom) factor() float32 {
	if z.scale == 0 {
		return 1
	}
	return z.scale
}

// by zooms by k, keeping the point at x, y of a canvas w by h where it is,
// as a map zooms around the pointer.
func (z *imageZoom) by(k, x, y, w, h float32) {
	next := min(max(z.factor()*k, zoomMin), zoomMax)
	k = next / z.factor()
	// The point, from the middle of the content, moves out k times.
	px, py := x-w/2, y-h/2
	z.x, z.y = px-(px-z.x)*k, py-(py-z.y)*k
	z.scale = next
}

// fit shows the pictures whole in their pane again.
func (z *imageZoom) fit() { z.scale, z.x, z.y = 0, 0, 0 }

// side reads a picture: from a revision ("" the index's), or the working
// tree's file when worktree.
type side struct {
	rev, path string
	worktree  bool
}

func (s *side) read(ctx context.Context, repo *git.Repo) (*picture, error) {
	if s == nil {
		return nil, nil
	}
	var data []byte
	var err error
	if s.worktree {
		data, err = repo.ReadBytes(ctx, s.path)
	} else {
		data, err = repo.Blob(ctx, s.rev, s.path)
	}
	if err != nil {
		return nil, err
	}
	b, err := ui.DecodeBitmap(data)
	if err != nil {
		return nil, err
	}
	return &picture{b, len(data)}, nil
}

// loadImages reads a picture's two sides for the diff shown.
func (a *app) loadImages(old, new *side) {
	gen := a.detail.gen
	if a.detail.images == nil { // else the sides shown stay until they're read again
		a.detail.images = &images{}
	}
	work(a, func(ctx context.Context) (images, error) {
		o, err := old.read(ctx, a.repo)
		if err != nil {
			return images{}, err
		}
		n, err := new.read(ctx, a.repo)
		return images{old: o, new: n}, err
	}, func(im images, err error) {
		if gen != a.detail.gen || a.detail.images == nil {
			return
		}
		if err != nil {
			im.err = err.Error()
		}
		im.zoom = a.detail.images.zoom // read again as the file changed, it stays where it was looked at
		*a.detail.images = im
	})
}

// commitSides are where a file of the commit shown was before and after:
// its parent's (the one chosen, of a merge) and its own, or a comparison's
// two revisions, the working tree when the second is empty.
func (a *app) commitSides(oldName, newName string) (old, new *side) {
	c := a.detail.commit
	from, to := "", c.Hash
	switch {
	case comparing(c):
		from, to, _ = strings.Cut(c.Hash, "..")
	case len(c.Parents) > 0:
		from = c.Parents[min(a.detail.parent, len(c.Parents)-1)]
	default:
		from = c.Hash + "^" // a stash's, or none for a root commit
	}
	if oldName != "" {
		old = &side{rev: from, path: oldName}
	}
	if newName != "" {
		new = &side{rev: to, path: newName, worktree: to == ""}
	}
	return old, new
}

// changeSides are where a change's picture was and is: HEAD's and the
// index's when staged, else the index's and the working tree's.
func changeSides(c git.Change, staged bool) (old, new *side) {
	oldPath := c.Path
	if c.OldPath != "" {
		oldPath = c.OldPath
	}
	code := c.Work
	if staged {
		code = c.Staged
	}
	if prefs.OneList { // against HEAD: added has no old side, deleted no new
		switch {
		case c.Staged == 'A' || c.Work == '?':
			code = 'A'
		case c.Staged == 'D' || c.Work == 'D':
			code = 'D'
		case c.Staged != 0:
			code = c.Staged
		}
	}
	if staged {
		old, new = &side{rev: "HEAD", path: oldPath}, &side{rev: "", path: c.Path}
	} else if prefs.OneList { // against HEAD, the working tree as it is now
		old, new = &side{rev: "HEAD", path: oldPath}, &side{path: c.Path, worktree: true}
	} else {
		old, new = &side{rev: "", path: c.Path}, &side{path: c.Path, worktree: true}
	}
	switch code {
	case 'A', '?':
		old = nil
	case 'D':
		new = nil
	}
	return old, new
}

// imageView shows a picture's change: its sides side by side, fit in the
// pane until zoomed with the wheel, a pinch or the buttons, and dragged or
// scrolled about with a touchpad (#50); a double click fits them again.
func (a *app) imageView(c *ui.Context, im *images) {
	t := c.Theme()
	switch {
	case im.err != "":
		ui.Column(c).Fill().Center().Padding(16).Children(func() {
			ui.Text(c, loc.ImageError(im.err)).TextColor(t.TextMuted).Wrap()
		})
		return
	case im.old == nil && im.new == nil:
		ui.Column(c).Fill().Center().Children(func() { ui.Spinner(c).Size(18, 18) })
		return
	}
	z := &im.zoom
	box := z.canvas.Bounds(c)
	scale := fitScale(im, box.W, box.H) * z.factor()
	canvas := ui.Column(c).Bind(&z.canvas).Fill().Center().Clip().Cursor(ui.CursorGrab).
		HandleInput(func(ev ui.InputEvent) bool {
			r := z.canvas.Bounds(c)
			switch {
			case ev.Kind == ui.InputZoom:
				z.by(ev.Scale, ev.X, ev.Y, r.W, r.H)
			case ev.Kind == ui.InputScroll && ev.Precise: // a touchpad's two fingers pan
				z.x, z.y = z.x-ev.DX, z.y-ev.DY
			case ev.Kind == ui.InputScroll: // a wheel's notch zooms
				k := float32(zoomStep)
				if ev.DY > 0 {
					k = 1 / k
				}
				z.by(k, ev.X, ev.Y, r.W, r.H)
			default:
				return false
			}
			return true
		})
	if dx, dy, held := canvas.Dragged(); held {
		z.x, z.y = z.x+dx, z.y+dy
	}
	if canvas.DoubleClicked() {
		z.fit()
	}
	canvas.Children(func() {
		ui.Row(c).Gap(imageGap).AlignItems(ui.End).Translate(z.x, z.y).Shrink(0).Children(func() {
			if im.old != nil {
				pictureView(c, loc.ImageBefore(), im.old, t.Danger, scale)
			}
			if im.new != nil {
				pictureView(c, loc.ImageAfter(), im.new, t.Success, scale)
			}
		})
		zoomBar(c, z, box, scale)
	})
}

// zoomBar is the canvas's buttons over its corner: out, the scale the
// pictures show at, of their pixels, which fits them again, and in.
func zoomBar(c *ui.Context, z *imageZoom, box ui.Rect, scale float32) {
	t := c.Theme()
	ui.Row(c).Absolute().Top(8).Right(8).Gap(2).Padding(2).Radius(6).AlignItems(ui.Center).
		Background(t.Surface).BorderWidth(1).BorderColor(t.Border).Children(func() {
		if iconButton(c, "minus", loc.ZoomOut()) {
			z.by(1/zoomStep, box.W/2, box.H/2, box.W, box.H)
		}
		label := fmt.Sprintf("%d%%", int(scale*100+0.5))
		b := ui.ButtonBase(c.Key("fit")).Height(22).Padding(0, 6).Radius(5).Center().
			Tooltip(loc.ZoomFit()).Label(loc.ZoomFit()).Cursor(ui.CursorPointer)
		if b.Hovered() {
			b.Background(t.Text.Alpha(0.12))
		}
		b.Children(func() { ui.Text(c, label).FontSize(11).Font("monospace") })
		if b.Clicked() {
			z.fit()
		}
		if iconButton(c, "plus", loc.ZoomIn()) {
			z.by(zoomStep, box.W/2, box.H/2, box.W, box.H)
		}
	})
}

// The room around the pictures shown: between the two, the frame of each
// (its padding and border, both sides), the label above and the size
// below with their gaps, and the canvas's margin.
const (
	imageGap    = 24
	imageFrame  = 2 * (6 + 1)
	imageText   = 2 * (18 + 8)
	imageMargin = 32
)

// fitScale is how much the pictures shrink to fit a canvas w by h, at
// most their own size.
func fitScale(im *images, w, h float32) float32 {
	var sumW, maxH float32
	n := 0
	for _, p := range []*picture{im.old, im.new} {
		if p != nil {
			pw, ph := p.bitmap.Size()
			sumW += float32(pw)
			maxH = max(maxH, float32(ph))
			n++
		}
	}
	if w <= 0 || h <= 0 { // not laid out yet: as the strip showed them, 420 by 360
		w, h = float32(n)*(420+imageFrame)+imageGap+imageMargin, 360+imageFrame+imageText+imageMargin
	}
	roomW := w - imageMargin - float32(n)*imageFrame - float32(n-1)*imageGap
	roomH := h - imageMargin - imageFrame - imageText
	if sumW <= 0 || maxH <= 0 || roomW <= 0 || roomH <= 0 {
		return 1
	}
	return min(roomW/sumW, roomH/maxH, 1)
}

// pictureView is a side of a picture's change: its label, the picture in
// a frame of the side's color, scaled, and its size.
func pictureView(c *ui.Context, label string, p *picture, color ui.Color, scale float32) {
	t := c.Theme()
	w, h := p.bitmap.Size()
	ui.Column(c).Gap(8).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Text(c, label).Bold().TextColor(color)
		ui.Box(c).Padding(6).Radius(8).BorderWidth(1).BorderColor(color.Alpha(0.5)).Background(t.Surface).Shrink(0).Children(func() {
			ui.Image(c, p.bitmap).Size(float32(w)*scale, float32(h)*scale)
		})
		ui.Text(c, fmt.Sprintf("%d × %d · %s", w, h, byteSize(p.bytes))).FontSize(11).TextColor(t.TextMuted)
	})
}

// byteSize says a file's size as Finder does.
func byteSize(n int) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d B", n)
	case n < 1000*1000:
		return fmt.Sprintf("%.1f KB", float64(n)/1000)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/1000/1000)
}
