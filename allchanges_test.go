package main

import (
	"context"
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
			t.Fatalf("%s: no diff fitting its lines, %q", f.path, f.note)
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

// A commit's files show one after another too, kept for the next commit;
// a file chosen in the list scrolls to it, and its name shows it alone.
// Wrapping wraps them all (#44).
func TestCommitAll(t *testing.T) {
	defer func(on, wrap bool) { prefs.CommitAll, prefs.Wrap = on, wrap }(prefs.CommitAll, prefs.Wrap)
	prefs.CommitAll, prefs.Wrap = false, false
	tt := newTester(t)
	ctx := context.Background()
	// No changes, so the list's button of the same name isn't there.
	tt.write(t, map[string]string{"a.txt": "a\n", "b.txt": "b\n"})
	for _, args := range [][]string{{"add", "a.txt", "b.txt"}, {"commit", "-qm", "Two files"}, {"clean", "-qf"}} {
		if _, err := tt.a.repo.Git(ctx, args...); err != nil {
			t.Fatal(err)
		}
	}
	tt.a.refresh()
	tt.settle()
	tt.click(t, "Two files")
	if d := tt.a.detail; d.diff == nil || d.commitAll != nil {
		t.Fatalf("one file: %+v", d)
	}
	tt.click(t, loc.ShowAllChanges())
	if d := tt.a.detail; !prefs.CommitAll || len(d.commitAll) != 2 || d.commitAll[0].diff == nil || !d.commitAll[0].diff.FitContent {
		t.Fatalf("all: %+v", d.commitAll)
	}
	if !tt.HasText("a.txt") || !tt.HasText(loc.WrapLines()) {
		t.Fatalf("texts %q", tt.Texts())
	}
	tt.click(t, loc.WrapLines())
	for _, f := range tt.a.detail.commitAll {
		if !f.diff.Wrap {
			t.Fatalf("%s not wrapped", f.path)
		}
	}

	// Another commit opens one after another too.
	tt.click(t, "Merge the feature")
	if d := tt.a.detail; d.commit == nil || d.commit.Subject != "Merge the feature" || d.commitAll == nil {
		t.Fatalf("next commit: %+v", d)
	}
	tt.click(t, "Two files")

	// The list scrolls to a file, staying one after another.
	tt.click(t, "b.txt")
	if d := tt.a.detail; d.fileSel != 1 || d.allReveal != 0 || d.commitAll == nil || d.diff != nil {
		t.Fatalf("revealed: sel %d, reveal %d", d.fileSel, d.allReveal)
	}

	// A name clicked shows its file alone, as its header does.
	prefs.CommitAll = false
	tt.a.commitAllChanged()
	tt.settle()
	if d := tt.a.detail; prefs.CommitAll || d.diff == nil || d.commitAll != nil {
		t.Fatalf("alone: %+v", d)
	}
}

// Shown as a tree, the tree's toggle and the commit's toggle both stay
// their own buttons once the commit shows all its files (#48).
func TestCommitAllBesideTree(t *testing.T) {
	defer func(on, tree bool) { prefs.CommitAll, prefs.TreeView = on, tree }(prefs.CommitAll, prefs.TreeView)
	prefs.CommitAll, prefs.TreeView = false, false
	tt := newTester(t)
	ctx := context.Background()
	tt.write(t, map[string]string{"a.txt": "a\n", "b.txt": "b\n"})
	for _, args := range [][]string{{"add", "a.txt", "b.txt"}, {"commit", "-qm", "Two files"}, {"clean", "-qf"}} {
		if _, err := tt.a.repo.Git(ctx, args...); err != nil {
			t.Fatal(err)
		}
	}
	tt.a.refresh()
	tt.settle()
	tt.click(t, "Two files")
	tt.click(t, loc.ViewAsTree())
	tt.click(t, loc.ShowAllChanges())
	if !prefs.TreeView || !prefs.CommitAll {
		t.Fatalf("tree %v, all %v", prefs.TreeView, prefs.CommitAll)
	}
	tt.click(t, loc.ShowFileAlone())
	if !prefs.TreeView || prefs.CommitAll {
		t.Fatalf("alone: tree %v, all %v", prefs.TreeView, prefs.CommitAll)
	}
	tt.click(t, loc.ShowAllChanges())
	tt.click(t, loc.ViewAsList())
	if prefs.TreeView || !prefs.CommitAll {
		t.Fatalf("list: tree %v, all %v", prefs.TreeView, prefs.CommitAll)
	}
}
