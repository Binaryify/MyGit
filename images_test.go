package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	loc "mygit/locales"
)

// pngOf is a picture w by h of one color, as a PNG file's bytes.
func pngOf(t *testing.T, w, h int, c color.Color) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestImagePreview(t *testing.T) {
	tt := newTester(t)
	tt.commitFiles(t, "Add the logo", map[string]string{"logo.png": pngOf(t, 40, 30, color.White)})
	tt.click(t, "Add the logo")
	tt.settle()
	im := tt.a.detail.images
	if im == nil || im.err != "" || im.old != nil || im.new == nil {
		t.Fatalf("added picture: %+v", im)
	}
	if w, h := im.new.bitmap.Size(); w != 40 || h != 30 || !tt.HasText("之后") || !tt.HasText("40 × 30 · "+byteSize(im.new.bytes)) {
		t.Fatalf("size %dx%d, texts %q", w, h, tt.Texts())
	}

	// The working tree's change: the index's picture beside the file's.
	if err := os.WriteFile(filepath.Join(tt.a.repo.Dir, "logo.png"), []byte(pngOf(t, 80, 60, color.Black)), 0o644); err != nil {
		t.Fatal(err)
	}
	tt.a.refresh()
	tt.settle()
	ch, ok := find(tt.a.unstaged, "logo.png")
	if !ok {
		t.Fatal("no change of logo.png")
	}
	tt.a.showChange(ch, false)
	tt.settle()
	im = tt.a.detail.images
	if im == nil || im.old == nil || im.new == nil {
		t.Fatalf("changed picture: %+v", im)
	}
	ow, _ := im.old.bitmap.Size()
	nw, _ := im.new.bitmap.Size()
	if ow != 40 || nw != 80 || !tt.HasText("之前") || !tt.HasText("之后") {
		t.Fatalf("widths %d and %d, texts %q", ow, nw, tt.Texts())
	}
}

func TestByteSize(t *testing.T) {
	for n, want := range map[int]string{12: "12 B", 1500: "1.5 KB", 2_400_000: "2.4 MB"} {
		if got := byteSize(n); got != want {
			t.Errorf("byteSize(%d) = %q, want %q", n, got, want)
		}
	}
}

// A picture too big for its pane fits in it, zooms with the wheel, a pinch
// and the buttons, pans as it is dragged, and fits again (#50).
func TestImageZoomAndPan(t *testing.T) {
	tt := newTester(t)
	tt.commitFiles(t, "Add a shot", map[string]string{"shot.png": pngOf(t, 3000, 2000, color.White)})
	tt.click(t, "Add a shot")
	showFileNamed(t, tt, "shot.png")
	im := tt.a.detail.images
	if im == nil || im.new == nil {
		t.Fatalf("no picture: %+v", im)
	}
	tt.Frame()
	z := &im.zoom
	r, ok := tt.Find(loc.ZoomFit())
	if !ok {
		t.Fatalf("no zoom bar; texts %q", tt.Texts())
	}
	// Fit in the pane, it shows at a fraction of its pixels.
	if !slices.ContainsFunc(tt.Texts(), func(s string) bool {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "%"))
		return err == nil && strings.HasSuffix(s, "%") && n > 0 && n < 50
	}) {
		t.Fatalf("not fit; texts %q", tt.Texts())
	}
	tt.click(t, loc.ZoomIn())
	if z.factor() != zoomStep {
		t.Fatalf("zoomed in to %v", z.factor())
	}
	// The wheel zooms, around the pointer; a pinch too.
	p, _ := tt.Find(loc.ImageAfter())
	cx, cy := p.X+p.W/2, p.Y+200
	tt.Wheel(cx, cy, 0, -40)
	tt.Frame()
	if z.factor() != zoomStep*zoomStep {
		t.Fatalf("wheel: %+v", *z)
	}
	tt.Pinch(cx+100, cy, 2)
	tt.Frame()
	if z.factor() != zoomStep*zoomStep*2 {
		t.Fatalf("pinch: %+v", *z)
	}
	// Dragged, the pictures move with the pointer.
	x0, y0 := z.x, z.y
	tt.Press(cx, cy)
	tt.Move(cx+30, cy+20)
	tt.Frame()
	tt.Release(cx+30, cy+20)
	tt.Frame()
	if z.x != x0+30 || z.y != y0+20 {
		t.Fatalf("dragged from %v,%v to %v,%v", x0, y0, z.x, z.y)
	}
	tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()
	if z.factor() != 1 || z.x != 0 || z.y != 0 {
		t.Fatalf("not fit again: %+v", *z)
	}
}

func TestImageZoomKeepsThePoint(t *testing.T) {
	var z imageZoom
	z.by(2, 150, 50, 200, 100) // 50 right of the middle
	if z.x != -50 || z.y != 0 || z.scale != 2 {
		t.Fatalf("%+v", z)
	}
	z.by(1000, 0, 0, 200, 100)
	if z.scale != zoomMax {
		t.Fatalf("past the most: %v", z.scale)
	}
}
