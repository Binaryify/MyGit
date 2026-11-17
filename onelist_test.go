package main

import (
	"testing"

	loc "mygit/locales"
)

// TestOneList lists the changes as IDEA's commit window does (#34): one
// list, a check staging a file and leaving it where it is, a file staged
// in part checked with a dash, and its diff shown, staged or not.
func TestOneList(t *testing.T) {
	defer func(one bool) { prefs.OneList = one }(prefs.OneList)
	prefs.OneList = true
	tt := newTester(t)
	tt.write(t, map[string]string{"main.go": "package main\n\nfunc main() { run() }\n"})
	tt.a.refresh()
	tt.settle()
	if tt.HasText(loc.Staged()) || !tt.HasText(loc.NewFiles()) {
		t.Fatalf("not one list: %q", tt.Texts())
	}
	where := func(name string) float32 {
		t.Helper()
		r, ok := tt.Find(name)
		if !ok {
			t.Fatalf("no %s: %q", name, tt.Texts())
		}
		return r.Y
	}
	notes, main := where("notes.txt"), where("main.go")

	// Checked, the new file stays among the new ones, the other in place.
	tt.click(t, loc.StageFile("notes.txt"))
	tt.click(t, loc.StageFile("main.go"))
	if len(tt.a.staged) != 2 || len(tt.a.unstaged) != 0 {
		t.Fatalf("staged %v, not %v", tt.a.staged, tt.a.unstaged)
	}
	if where("notes.txt") != notes || where("main.go") != main || !tt.HasText(loc.UnstageFile("main.go")) {
		t.Fatalf("moved: notes %v→%v, main %v→%v", notes, where("notes.txt"), main, where("main.go"))
	}

	// Chosen, a file staged whole shows its staged diff.
	tt.click(t, "main.go")
	if d := tt.a.detail; d.change == nil || !d.staged || d.diff == nil {
		t.Fatalf("staged main.go not shown: %+v", d)
	}

	// Changed again, it is staged in part: still checked, its diff now the
	// working tree's; a click stages the rest.
	tt.write(t, map[string]string{"main.go": "package main\n\nfunc main() { run(); run() }\n"})
	tt.a.refresh()
	tt.settle()
	if where("main.go") != main || !tt.HasText(loc.StageFile("main.go")) || tt.a.detail.staged {
		t.Fatalf("in part: staged %v, %q", tt.a.detail.staged, tt.Texts())
	}
	tt.click(t, loc.StageFile("main.go"))
	if len(tt.a.unstaged) != 0 {
		t.Fatalf("not staged whole: %v", tt.a.unstaged)
	}

	// Unchecked, it is unstaged, still in place.
	tt.click(t, loc.UnstageFile("main.go"))
	if _, ok := find(tt.a.unstaged, "main.go"); !ok || where("main.go") != main {
		t.Fatalf("not unstaged: %v", tt.a.unstaged)
	}
}
