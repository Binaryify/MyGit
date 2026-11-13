package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestTreeRows(t *testing.T) {
	paths := []string{"z.txt", "src/ui/view.go", "src/main.go", "docs/a/b/guide.md", "src/ui/app.go", "a.txt"}
	items := []int{0, 1, 2, 3, 4, 5}
	show := func(rows []treeRow) []string {
		var out []string
		for _, r := range rows {
			out = append(out, fmt.Sprintf("%d %s %d", r.depth, r.name, r.item))
		}
		return out
	}
	got := show(treeRows(items, func(i int) string { return paths[i] }, func(string) bool { return false }))
	want := []string{
		"0 docs/a/b -1", // a folder with one folder alone shows as one row
		"1 guide.md 3",
		"0 src -1",
		"1 ui -1", // folders first, then files
		"2 app.go 4",
		"2 view.go 1",
		"1 main.go 2",
		"0 a.txt 5",
		"0 z.txt 0",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("rows\n%q\nwant\n%q", got, want)
	}
	rows := treeRows(items, func(i int) string { return paths[i] }, func(dir string) bool { return dir == "src/ui" })
	if got := show(rows); !slices.Equal(got, slices.Delete(slices.Clone(want), 4, 6)) {
		t.Fatalf("folded: %q", got)
	}
	if treeRowOf(rows, 2) != 4 || treeRowOf(rows, 1) != -1 || treeRowOf(rows, -1) != -1 {
		t.Fatalf("rows of items: %d %d", treeRowOf(rows, 2), treeRowOf(rows, 1))
	}
}

func TestTreeView(t *testing.T) {
	defer func(on bool) { prefs.TreeView = on }(prefs.TreeView)
	prefs.TreeView = false
	tt := newTester(t)
	for _, name := range []string{"src/ui/view.go", "src/main.go"} {
		path := filepath.Join(tt.a.repo.Dir, name)
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tt.a.refresh()
	tt.settle()
	tt.click(t, "以树形式查看")
	tt.Frame()
	if !prefs.TreeView || !tt.HasText("src") || !tt.HasText("ui") || !tt.HasText("以列表形式查看") {
		t.Fatalf("tree %v, texts %q", prefs.TreeView, tt.Texts())
	}
	tt.click(t, "view.go")
	tt.settle()
	if ch := tt.a.detail.change; ch == nil || ch.Path != "src/ui/view.go" {
		t.Fatalf("change %+v", ch)
	}
	tt.click(t, "ui") // folds it
	tt.Frame()
	if slices.Contains(tt.Texts(), "view.go") || !tt.HasText("main.go") || tt.a.detail.change.Path != "src/ui/view.go" {
		t.Fatalf("folded: texts %q", tt.Texts())
	}
	tt.click(t, "main.go")
	tt.settle()
	if ch := tt.a.detail.change; ch.Path != "src/main.go" {
		t.Fatalf("change %+v", ch)
	}
	tt.click(t, "以列表形式查看")
	tt.Frame()
	if prefs.TreeView || !tt.HasText("view.go") || !tt.HasText("src/ui") {
		t.Fatalf("list: texts %q", tt.Texts())
	}
}

// TestUntrackedGroup checks that the untracked files show apart from the
// other changes, under a group of their own that folds, and stages them
// all, as IDEA's Unversioned Files (issue #26).
func TestUntrackedGroup(t *testing.T) {
	defer func(on bool) { prefs.TreeView = on }(prefs.TreeView)
	prefs.TreeView = false
	tt := newTester(t)
	_ = os.MkdirAll(filepath.Join(tt.a.repo.Dir, "src"), 0o755)
	tt.write(t, map[string]string{"main.go": "package main\n\n// changed\n", "src/new.go": "package src\n"}) // notes.txt is untracked already
	tt.a.refresh()
	tt.settle()
	order := func() []string {
		var out []string
		for _, s := range tt.Texts() {
			if s == "main.go" || s == "未跟踪的文件" || s == "notes.txt" || s == "new.go" {
				out = append(out, s)
			}
		}
		return out
	}
	if got, want := order(), []string{"main.go", "未跟踪的文件", "notes.txt", "new.go"}; !slices.Equal(got, want) {
		t.Fatalf("order %q, want %q", got, want)
	}
	tt.click(t, "未跟踪的文件") // folds it
	tt.Frame()
	if got := order(); !slices.Equal(got, []string{"main.go", "未跟踪的文件"}) {
		t.Fatalf("folded: %q", got)
	}
	tt.click(t, "未跟踪的文件")
	tt.Frame()
	r, ok := tt.Find("未跟踪的文件")
	if !ok {
		t.Fatal("no group row")
	}
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()
	tt.click(t, "暂存未跟踪的文件")
	var staged, unstaged []string
	for _, c := range tt.a.staged {
		staged = append(staged, c.Path)
	}
	for _, c := range tt.a.unstaged {
		unstaged = append(unstaged, c.Path)
	}
	if !slices.Equal(staged, []string{"notes.txt", "src/new.go"}) || !slices.Equal(unstaged, []string{"main.go"}) {
		t.Fatalf("staged %q, unstaged %q", staged, unstaged)
	}
	if tt.HasText("未跟踪的文件") {
		t.Fatalf("a group of no files: %q", tt.Texts())
	}
}
