package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	loc "mygit/locales"
)

// A commit's file and a change pop out as a copy of their diff, in a
// window of its own; the main window goes on to other files (#46).
func TestPopOutDiff(t *testing.T) {
	tt := newTester(t)
	tt.commitFiles(t, "Change two lines", map[string]string{"a.txt": linesOf(40)})
	tt.commitFiles(t, "Change two lines", map[string]string{"a.txt": linesOf(40, 5, 30)})
	tt.click(t, "Change two lines")
	showFileNamed(t, tt, "a.txt")
	if !tt.HasText(loc.PopOutDiff()) {
		t.Fatalf("no button; texts %q", tt.Texts())
	}
	po := tt.a.popOut()
	if po == nil || po.diff == tt.a.detail.diff || len(po.diff.Hunks()) != 2 || po.diff.New.String() != tt.a.detail.diff.New.String() {
		t.Fatalf("copy %+v", po)
	}
	win := ui.NewTester(po.view, 900, 600)
	win.Frame()
	if !win.HasText(po.title) || !win.HasText(loc.ChangeOf(1, 2)) {
		t.Fatalf("window texts %q", win.Texts())
	}

	// A change of the working tree: the file as it is.
	tt.write(t, map[string]string{"a.txt": linesOf(40, 7)})
	tt.a.refresh()
	tt.settle()
	tt.click(t, "a.txt")
	po = tt.a.popOut()
	if po == nil || po.title != "a.txt" || !tt.HasText(loc.PopOutDiff()) {
		t.Fatalf("change: %+v", po)
	}
	if got := po.diff.New.String(); got != linesOf(40, 7) {
		t.Fatalf("new side %q", got)
	}
}
