package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
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
