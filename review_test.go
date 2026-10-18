package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func TestCommentOnACommit(t *testing.T) {
	tt := newTester(t)
	tt.commitFiles(t, "Grow main", map[string]string{"main.go": "package main\n\nfunc main() {}\n\nfunc run() {\n\tprintln(1)\n}\n"})
	tt.click(t, "Grow main")
	a := tt.a
	for i, f := range a.detail.files {
		if fileName(f) == "main.go" {
			a.showFile(i)
		}
	}
	tt.Frame()
	// The diff's lines are the patch's: its line 0 is the file's first
	// line the hunk shows, and the comment numbers the file's.
	d := a.detail.diff
	i, ok := a.toDiff(editor.DiffNew, 6)
	if !ok {
		t.Fatal("line 6 not in the diff")
	}
	d.OnGutterButton(editor.DiffLineRange{Side: editor.DiffNew, Start: i, End: i})
	tt.Frame()
	if !a.composer.open || a.composer.draft.Start != 6 || a.composer.draft.Code[0] != "\tprintln(1)" {
		t.Fatalf("composer %+v", a.composer)
	}
	tt.Type("Print something useful")
	tt.Key(ui.Cmd, ui.KeyEnter)
	tt.Frame()
	if a.composer.open || len(a.comments) != 1 || a.comments[0].Body != "Print something useful" {
		t.Fatalf("comments %+v", a.comments)
	}
	if len(d.Annotations) != 1 || d.Annotations[0].Line != i || !tt.HasText("Print something useful") || !tt.HasText("1 条评论 ▾") {
		t.Fatalf("annotations %+v, texts %q", d.Annotations, tt.Texts())
	}
	md := commentsMarkdown(a.comments)
	for _, want := range []string{"# Address these Review Comments", "## 1. main.go:6 (commit " + a.detail.commit.Hash[:8] + ")", "```go\n\tprintln(1)\n```", "Print something useful"} {
		if !strings.Contains(md, want) {
			t.Errorf("no %q in\n%s", want, md)
		}
	}
	// Another file's diff doesn't show it.
	for i, f := range a.detail.files {
		if fileName(f) == "notes.txt" {
			a.showFile(i)
		}
	}
	tt.Frame()
	if len(a.detail.diff.Annotations) != 0 {
		t.Error("the comment shows on another file")
	}
}

func TestCommentRange(t *testing.T) {
	cm := comment{Path: "a/b.ts", Start: 3, End: 5, Side: editor.DiffOld, Code: []string{"x", "y", "z"}, Body: "Why remove these?"}
	md := commentsMarkdown([]comment{cm})
	if !strings.Contains(md, "## 1. a/b.ts:3-5 (removed lines)") || !strings.Contains(md, "```ts\nx\ny\nz\n```") {
		t.Errorf("markdown\n%s", md)
	}
}
