package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui/editor"

	"mygit/internal/conflict"
	"mygit/internal/git"
)

// conflictIn merges a branch into main that changed list.txt from base
// as theirs did, while main changed it as ours did, and opens its merge.
func conflictIn(t *testing.T, tt *tester, base, ours, theirs string) *merger {
	t.Helper()
	ctx := context.Background()
	r := tt.a.repo
	write := func(text string) {
		if err := os.WriteFile(filepath.Join(r.Dir, "list.txt"), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	commit := func(msg string) {
		for _, args := range [][]string{{"add", "list.txt"}, {"-c", "user.name=a", "-c", "user.email=a@a", "commit", "-q", "-m", msg}} {
			if _, err := r.Git(ctx, args...); err != nil {
				t.Fatal(err)
			}
		}
	}
	write(base)
	commit("base")
	if _, err := r.Git(ctx, "switch", "-q", "-c", "other"); err != nil {
		t.Fatal(err)
	}
	write(theirs)
	commit("theirs")
	if _, err := r.Git(ctx, "switch", "-q", "main"); err != nil {
		t.Fatal(err)
	}
	write(ours)
	commit("ours")
	tt.a.run("合并", func(ctx context.Context) error { return r.Merge(ctx, "other") }, nil)
	tt.settle()
	if tt.a.op != git.Merging {
		t.Fatalf("op %q, texts %q", tt.a.op, tt.Texts())
	}
	tt.click(t, "list.txt")
	m := tt.a.detail.merge
	if m == nil {
		t.Fatalf("no merge window: %q", tt.Texts())
	}
	return m
}

func numbered(n int, change map[int]string) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		if s, ok := change[i]; ok {
			b.WriteString(s)
			continue
		}
		fmt.Fprintf(&b, "line %d\n", i)
	}
	return b.String()
}

// The merge window takes the changes of one side alone, resolves
// conflicts of different words with the magic wand, takes or dismisses a
// side's change with the buttons by it, follows edits by hand and their
// undoing, and scrolls its panes together.
func TestMergeWindow(t *testing.T) {
	tt := newTester(t)
	base := numbered(200, map[int]string{100: "call(a, b)\n"})
	ours := numbered(200, map[int]string{2: "ours two\n", 100: "call(x, b)\n", 150: "ours only\n"})
	theirs := numbered(200, map[int]string{2: "theirs two\n", 100: "call(a, y)\n", 180: ""})
	m := conflictIn(t, tt, base, ours, theirs)
	result := m.panes[paneResult].Buffer
	want := numbered(200, map[int]string{100: "call(a, b)\n", 150: "ours only\n", 180: ""})
	if result.String() != want || m.model.Unresolved() != 2 || !tt.HasText("冲突 1 / 2") {
		t.Fatalf("result %q, chunks %+v, texts %q", result.String(), m.model.Chunks, tt.Texts())
	}
	// It opens at the first conflict, the second line, as near the top as
	// it goes.
	if m.opening || m.scroll[paneResult].Y != 0 {
		t.Fatalf("opening %v, scrolled to %v", m.opening, m.scroll[paneResult].Y)
	}

	tt.click(t, "自动解决简单冲突")
	want = numbered(200, map[int]string{100: "call(x, y)\n", 150: "ours only\n", 180: ""})
	if result.String() != want || m.model.Unresolved() != 1 || !tt.HasText("自动解决了 1 个冲突") {
		t.Fatalf("result %q, texts %q", result.String(), tt.Texts())
	}

	// Ours by its ≫, then theirs dismissed by its ✕.
	tt.click(t, "采用这处改动")
	want = numbered(200, map[int]string{2: "ours two\n", 100: "call(x, y)\n", 150: "ours only\n", 180: ""})
	if result.String() != want || m.model.Unresolved() != 1 {
		t.Fatalf("result %q, chunks %+v", result.String(), m.model.Chunks)
	}
	tt.click(t, "忽略这处改动")
	if result.String() != want || !tt.HasText("没有剩余冲突") {
		t.Fatalf("result %q, texts %q", result.String(), tt.Texts())
	}

	// Lines typed above move the chunks; undoing the side taken makes the
	// conflict one again.
	result.Edit(editor.Edit{Text: "top\n"})
	tt.Frame()
	if c := m.model.Chunks[0]; c.Result != (conflict.Range{Start: 2, End: 3}) || !c.Resolved() {
		t.Fatalf("after typing: %+v", c)
	}
	result.Edit(editor.Edit{End: len("top\n")})
	at := len("line 1\n")
	result.Edit(editor.Edit{Start: at, End: at + len("ours two\n"), Text: "line 2\n"})
	tt.Frame()
	if m.model.Unresolved() != 1 || !tt.HasText("冲突 1 / 1") {
		t.Fatalf("after undoing: %+v, texts %q", m.model.Chunks, tt.Texts())
	}

	// The sides scroll with the result, line for line.
	r, _ := tt.Find("结果")
	tt.Scroll(r.X+r.W/2, r.Y+r.H+60, 0, 1200)
	tt.Frame()
	tt.Frame()
	ry, oy, ty := m.scroll[paneResult].Y, m.scroll[paneOurs].Y, m.scroll[paneTheirs].Y
	if ry <= 0 || abs(oy-ry) > m.lh || abs(ty-ry) > m.lh {
		t.Fatalf("scrolls: ours %v, result %v, theirs %v (line %v)", oy, ry, ty, m.lh)
	}

	tt.click(t, "采用右侧")
	tt.click(t, "保存并标记为已解决")
	if b, _ := os.ReadFile(filepath.Join(tt.a.repo.Dir, "list.txt")); string(b) != numbered(200, map[int]string{2: "theirs two\n", 100: "call(x, y)\n", 150: "ours only\n", 180: ""}) {
		t.Fatalf("saved %q, error %q", b, tt.a.errMsg)
	}
}

// A file with \r\n line ends is saved with them.
func TestMergeLineEnds(t *testing.T) {
	tt := newTester(t)
	crlf := func(s string) string { return strings.ReplaceAll(s, "\n", "\r\n") }
	m := conflictIn(t, tt, crlf("a\nb\nc\n"), crlf("a\nours\nc\n"), crlf("a\ntheirs\nc\nd\n"))
	if got := m.panes[paneResult].Buffer.String(); got != "a\nb\nc\nd\n" {
		t.Fatalf("result %q", got)
	}
	tt.click(t, "采用左侧")
	tt.click(t, "保存并标记为已解决")
	if b, _ := os.ReadFile(filepath.Join(tt.a.repo.Dir, "list.txt")); string(b) != crlf("a\nours\nc\nd\n") {
		t.Fatalf("saved %q, error %q", b, tt.a.errMsg)
	}
}

func abs(x float32) float32 { return max(x, -x) }

// Undoing a side taken makes the conflict one again.
func TestMergeUndo(t *testing.T) {
	m := newMerger("a.txt", mergeTexts{base: "a\nb\nc\n", ours: "a\nours\nc\n", theirs: "a\ntheirs\nc\n", has: [4]bool{1: true, 2: true, 3: true}})
	m.apply(0, conflict.Ours)
	b := m.panes[paneResult].Buffer
	if b.String() != "a\nours\nc\n" || m.model.Unresolved() != 1 {
		t.Fatalf("result %q, %+v", b.String(), m.model.Chunks)
	}
	if !b.Undo() {
		t.Fatal("nothing to undo")
	}
	m.sync()
	if c := m.model.Chunks[0]; b.String() != "a\nb\nc\n" || c.State != [2]conflict.State{conflict.Pending, conflict.Pending} {
		t.Fatalf("result %q, %+v", b.String(), c)
	}
}

// ⌘Z takes back a change dismissed, while the result wasn't edited since.
func TestMergeUndoIgnore(t *testing.T) {
	m := newMerger("a.txt", mergeTexts{base: "a\nb\nc\n", ours: "a\nours\nc\n", theirs: "a\ntheirs\nc\n", has: [4]bool{1: true, 2: true, 3: true}})
	m.ignore(0, conflict.Theirs)
	if !m.undoIgnore() || m.model.Chunks[0].State[conflict.Theirs] != conflict.Pending || m.undoIgnore() {
		t.Fatalf("%+v", m.model.Chunks[0])
	}
	m.ignore(0, conflict.Theirs)
	m.panes[paneResult].Buffer.Edit(editor.Edit{Text: "x"})
	if m.undoIgnore() {
		t.Fatal("took back a dismissal under an edit")
	}
}

// A file whose conflicts were edited by hand keeps the edits: the chunks
// are what the text has.
func TestMergeKeepsEdits(t *testing.T) {
	m := newMerger("a.txt", mergeTexts{base: "a\nb\nc\nd\n", ours: "a\nB\nc\nd\n", theirs: "a\nβ\nc\nD\n",
		work: "a\nB\nc\nD\n", has: [4]bool{1: true, 2: true, 3: true}})
	if got := m.panes[paneResult].Buffer.String(); got != "a\nB\nc\nD\n" || m.model.Unresolved() != 0 || m.note == "" {
		t.Fatalf("result %q, %+v", got, m.model.Chunks)
	}
	m.remerge(false)
	if got := m.panes[paneResult].Buffer.String(); got != "a\nb\nc\nD\n" || m.model.Unresolved() != 1 {
		t.Fatalf("merged again: %q, %+v", got, m.model.Chunks)
	}
	// Markers left, it is merged afresh.
	m = newMerger("a.txt", mergeTexts{base: "a\nb\n", ours: "a\nB\n", theirs: "a\nβ\n", work: "a\n<<<<<<< HEAD\nB\n=======\nβ\n>>>>>>> x\n",
		has: [4]bool{1: true, 2: true, 3: true}})
	if got := m.panes[paneResult].Buffer.String(); got != "a\nb\n" || m.note != "" {
		t.Fatalf("with markers: %q", got)
	}
}

// Saving a file opens the next in conflict; one a side deleted is kept or
// deleted whole.
func TestMergeNextAndDeleted(t *testing.T) {
	tt := newTester(t)
	ctx := context.Background()
	r := tt.a.repo
	sh := func(args ...string) {
		t.Helper()
		if _, err := r.Git(ctx, append([]string{"-c", "user.name=a", "-c", "user.email=a@a"}, args...)...); err != nil {
			t.Fatal(err)
		}
	}
	write := func(name, text string) {
		if err := os.WriteFile(filepath.Join(r.Dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.txt", "a\n")
	write("b.txt", "b\n")
	sh("add", ".")
	sh("commit", "-q", "-m", "base")
	sh("switch", "-q", "-c", "other")
	write("a.txt", "theirs a\n")
	write("b.txt", "theirs b\n")
	sh("commit", "-q", "-am", "theirs")
	sh("switch", "-q", "main")
	write("a.txt", "ours a\n")
	sh("rm", "-q", "b.txt")
	sh("commit", "-q", "-am", "ours")
	tt.a.run("合并", func(ctx context.Context) error { return r.Merge(ctx, "other") }, nil)
	tt.settle()
	tt.click(t, "a.txt")
	tt.click(t, "采用左侧")
	tt.click(t, "保存并标记为已解决")
	m := tt.a.detail.merge
	if m == nil || m.path != "b.txt" || !tt.HasText("我们的（HEAD）删除了这个文件，他们的修改了它。") {
		t.Fatalf("next: %+v, texts %q", m, tt.Texts())
	}
	tt.click(t, "保留文件（他们的）")
	if b, _ := os.ReadFile(filepath.Join(r.Dir, "b.txt")); string(b) != "theirs b\n" || tt.a.errMsg != "" {
		t.Fatalf("kept %q, error %q", b, tt.a.errMsg)
	}
	if out, _ := r.Git(ctx, "diff", "--name-only", "--diff-filter=U"); out != "" {
		t.Fatalf("left in conflict: %q", out)
	}
}

// The base shows against a side, and back.
func TestMergeCompareBase(t *testing.T) {
	tt := newTester(t)
	m := conflictIn(t, tt, "a\nb\n", "a\nours\n", "a\ntheirs\n")
	m.compareBase(paneOurs, "比较 base 与左侧")
	tt.Frame()
	if !tt.HasText("返回合并") || m.compare.Old.String() != "a\nb\n" || m.compare.New.String() != "a\nours\n" {
		t.Fatalf("texts %q", tt.Texts())
	}
	tt.click(t, "返回合并")
	if m.compare != nil || !tt.HasText("采用左侧") {
		t.Fatalf("texts %q", tt.Texts())
	}
}

// The Edit menu's Undo, as ⌘Z comes on macOS, takes back a dismissal.
func TestMergeUndoIgnoreMenu(t *testing.T) {
	tt := newTester(t)
	m := conflictIn(t, tt, "a\nb\n", "a\nours\n", "a\ntheirs\n")
	tt.click(t, "忽略这处改动")
	if m.model.Chunks[0].State[conflict.Ours] != conflict.Ignored {
		t.Fatalf("%+v", m.model.Chunks[0])
	}
	tt.Command("undo")
	tt.Frame()
	if c := m.model.Chunks[0]; c.State != [2]conflict.State{conflict.Pending, conflict.Pending} || m.panes[paneResult].Buffer.String() != "a\nb\n" {
		t.Fatalf("%+v %q", c, m.panes[paneResult].Buffer.String())
	}
}
