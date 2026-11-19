package main

import (
	"testing"

	loc "mygit/locales"
)

// The changes of a list show one after another in one scroll, follow the
// files as they change, and give way to one file chosen (#44).
func TestShowAllChanges(t *testing.T) {
	tt := newTester(t)
	tt.write(t, map[string]string{"main.go": "package main\n\nfunc main() { println(1) }\n"})
	tt.click(t, loc.ShowAllChanges())
	all := tt.a.detail.all
	if all == nil || all.staged || len(all.files) != 2 {
		t.Fatalf("all: %+v", all)
	}
	for _, f := range all.files {
		if f.diff == nil || !f.diff.FitContent {
			t.Fatalf("%s: no diff fitting its lines, %q", f.change.Path, f.note)
		}
	}
	if !tt.HasText(loc.AllChangesTitle(2)) || !tt.HasText("-1") {
		t.Fatalf("texts %q", tt.Texts())
	}

	// Refreshed unchanged, the diffs stay; changed, they follow.
	kept := all.files[0].diff
	tt.a.refresh()
	tt.settle()
	if tt.a.detail.all == nil || tt.a.detail.all.files[0].diff != kept {
		t.Fatal("the diffs read again unchanged")
	}
	tt.write(t, map[string]string{"main.go": "package main\n\nfunc main() { println(2) }\n"})
	if all := tt.a.detail.all; all == nil || all.files[0].diff == kept || all.files[0].diff == nil {
		t.Fatalf("not followed: %+v", all)
	}

	// Staged, the staged list shows its own.
	tt.click(t, loc.StageAll())
	tt.a.showAll(true)
	tt.settle()
	if all := tt.a.detail.all; all == nil || !all.staged || len(all.files) != 2 {
		t.Fatalf("staged all: %+v", all)
	}

	// A file chosen shows alone.
	tt.a.chooseChange(0, true)
	tt.settle()
	if d := tt.a.detail; d.all != nil || d.change == nil {
		t.Fatalf("chosen: %+v", d)
	}
}
