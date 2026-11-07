package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

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
