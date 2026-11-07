package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	loc "mygit/locales"
)

// The box a minimap draws on the rows in view: where they are in the file,
// as tall as their share of it, within the strip.
func TestMinimapViewportBox(t *testing.T) {
	r := ui.Rect{X: 10, Y: 100, W: 20, H: 200}
	for _, tt := range []struct {
		scroll, viewport, total float64
		want                    ui.Rect
	}{
		// At the top: from there, a quarter of the rows.
		{0, 250, 1000, ui.Rect{X: 10, Y: 100, W: 20, H: 50}},
		// Half way down, the quarter in the middle.
		{500, 250, 1000, ui.Rect{X: 10, Y: 200, W: 20, H: 50}},
		// At the end it stops at the bottom.
		{750, 250, 1000, ui.Rect{X: 10, Y: 250, W: 20, H: 50}},
		// A view of a few lines of a long file still shows.
		{0, 5, 100000, ui.Rect{X: 10, Y: 100, W: 20, H: 3}},
		// Everything in view is the whole strip.
		{0, 1000, 1000, ui.Rect{X: 10, Y: 100, W: 20, H: 200}},
		// Nothing to scale.
		{0, 0, 0, ui.Rect{}},
	} {
		if got := viewportBox(r, tt.scroll, tt.viewport, tt.total); got != tt.want {
			t.Errorf("box of %v/%v of %v: %+v, want %+v", tt.scroll, tt.viewport, tt.total, got, tt.want)
		}
	}
}

// A hunk that ends in a block comment leaves the next hunk code: the lines
// between them, where the comment ends, are not in the patch. The bubble
// reads a patch's lines alone, so it starts each hunk afresh, as the diff's
// own highlighter does.
func TestPatchHighlightsEachHunk(t *testing.T) {
	files, err := editor.ParsePatch(`diff --git a/a.cpp b/a.cpp
--- a/a.cpp
+++ b/a.cpp
@@ -1,4 +1,4 @@
 int a = 1;
-int b = 2;
+int b = 3;
 /**
  * Counts.
@@ -20,2 +20,2 @@
-for (int i = 0; i < n; i++) {
+for (size_t i = 0; i < n; i++) {
     ++count;
`)
	if err != nil || len(files) != 1 {
		t.Fatal(files, err)
	}
	d := patchDiff(files[0])
	starts := patchStarts(files[0])
	if len(starts) != 2 || starts[0] != 0 || starts[1] != 4 {
		t.Fatalf("starts %v of %d lines", starts, d.New.LineCount())
	}
	hl := previewHighlighter(d.New, starts)
	if hl == nil || !hl.Highlight(d.New.LineCount()-1, 1e9) {
		t.Fatal("not highlighted")
	}
	// The second hunk's line is code, not the comment the first ends in.
	line := d.New.Line(4) // "for (size_t ..."
	toks, ok := hl.Tokens(4, line, nil)
	if !ok || len(toks) == 0 {
		t.Fatalf("line %q: tokens %+v", line, toks)
	}
	for _, tk := range toks {
		if tk.Kind == editor.TokenComment {
			t.Fatalf("line %q: tokens %+v", line, toks)
		}
	}
	// Within a hunk the comment goes on.
	if toks, _ := hl.Tokens(3, d.New.Line(3), nil); len(toks) == 0 || toks[len(toks)-1].Kind != editor.TokenComment {
		t.Fatalf("line %q: tokens %+v", d.New.Line(3), toks)
	}
}

// The minimap's box follows the diff as it scrolls: it shows where the view
// is in the file, not only where the changes are, as mygo 0.8 gives the
// diff's scroll (ScrollY, ViewportHeight, LineTop).
func TestScenarioScrollTheDiffMovesTheMinimapBox(t *testing.T) {
	var b strings.Builder
	for i := range 400 {
		if i >= 100 && i <= 110 || i >= 300 && i <= 310 {
			b.WriteString("changed ")
		}
		b.WriteString("line " + strings.Repeat("x", 20) + "\n")
	}
	tt := newTester(t)
	tt.commitFiles(t, "Long file", map[string]string{"f.txt": strings.Repeat("line\n", 400)})
	tt.write(t, map[string]string{"f.txt": b.String()})
	tt.click(t, "f.txt")
	d := tt.a.detail.diff
	if d == nil || diffHeight(d) <= d.ViewportHeight() {
		t.Fatalf("diff %v high of %v in view", diffHeight(d), d.ViewportHeight())
	}
	// The strip's box, at the size of the strip it is drawn in.
	strip := ui.Rect{X: 0, Y: 0, W: 12, H: 300}
	box := func() ui.Rect {
		return viewportBox(minimapInner(strip), d.ScrollY(), d.ViewportHeight(), diffHeight(d))
	}
	before := box()
	if !tt.HasText(loc.ChangeOf(1, len(d.Hunks()))) {
		t.Fatalf("no change marks; texts %q", tt.Texts())
	}
	// The wheel over the diff goes on down the file.
	scrolled := d.ScrollY()
	tt.Scroll(1050, 500, 0, 300)
	tt.Frame()
	if d.ScrollY() <= scrolled {
		t.Fatalf("scrolled to %v, from %v", d.ScrollY(), scrolled)
	}
	if after := box(); after.Y <= before.Y {
		t.Fatalf("box at %+v after scrolling to %v, from %+v", after, d.ScrollY(), before)
	}
}

// The minimap's bubble colors a line by its tokens, keeping its text.
func TestPreviewSpans(t *testing.T) {
	d := textDiff("a.cpp", "", "\tfor (int i = 0; i < n; i++) { // go\n")
	hl := previewHighlighter(d.New, nil)
	if hl == nil || !hl.Highlight(0, 1e9) {
		t.Fatal("no highlighter")
	}
	line := d.New.Line(0)
	toks, _ := hl.Tokens(0, line, nil)
	ct := editor.DefaultTheme(ui.DarkTheme())
	spans := codeSpans(ct, line, toks)
	text := ""
	for _, s := range spans {
		text += s.Text
	}
	if text != "    for (int i = 0; i < n; i++) { // go" {
		t.Fatalf("text %q", text)
	}
	if spans[1].Text != "for" || spans[1].Color != ct.Tokens[editor.TokenKeyword].Color {
		t.Fatalf("spans %+v", spans[:2])
	}
}
