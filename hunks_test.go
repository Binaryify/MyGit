package main

import (
	"testing"

	"github.com/egoist/mygo/ui/editor"
)

// A hunk that ends in a block comment leaves the next hunk code: the
// lines between them, where the comment ends, are not in the patch.
func TestHunkAfterACommentIsCode(t *testing.T) {
	files, err := editor.ParsePatch(`diff --git a/a.cpp b/a.cpp
--- a/a.cpp
+++ b/a.cpp
@@ -1,3 +1,3 @@
-int a = 1;
+int a = 2;
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
	lang := d.New.Language()
	if lang == nil || lang.Highlighter == nil {
		t.Fatal("no hunk highlighter for C++")
	}
	h := lang.Highlighter(d.New.Snapshot(), func() {})
	n := d.New.LineCount()
	if !h.Highlight(n-1, 1e9) {
		t.Fatal("not highlighted")
	}
	line := d.New.Line(3) // "for (size_t ..."
	toks, ok := h.Tokens(3, line, nil)
	if !ok || len(toks) == 0 || toks[0].Kind != editor.TokenKeyword {
		t.Fatalf("line %q: tokens %+v", line, toks)
	}
	// Within a hunk the comment goes on.
	if toks, _ := h.Tokens(2, d.New.Line(2), nil); len(toks) == 0 || toks[len(toks)-1].Kind != editor.TokenComment {
		t.Fatalf("line %q: tokens %+v", d.New.Line(2), toks)
	}
	// mygo's own highlighter, one hunk going on into the next, makes it comment.
	plain := editor.NewTokenHighlighter(lang.Tokenizer, d.New.Snapshot())
	plain.Highlight(n-1, 1e9)
	if toks, _ := plain.Tokens(3, line, nil); toks[0].Kind != editor.TokenComment {
		t.Skip("mygo now starts each hunk afresh: hunkHighlighter may go")
	}
}
