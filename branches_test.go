package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	loc "mygit/locales"
)

// A branch deleted comes back at the commit it was at, while the app runs,
// as IDEA brings back what was deleted by mistake.
func TestScenarioUndoTheBranchDeleted(t *testing.T) {
	tt := newTester(t)
	ctx := context.Background()
	before, err := tt.a.repo.BranchHash(ctx, "feature")
	if err != nil || before == "" {
		t.Fatalf("feature at %q, %v", before, err)
	}
	tt.click(t, loc.Branches())
	tt.rightClickBranch(t, 0) // feature, the first of the local ones
	if err := tt.ChooseMenuItem(loc.Delete()); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	tt.settle()
	if b, _ := tt.a.repo.Branches(ctx); slices.Contains(b, "feature") {
		t.Fatalf("feature still there: %v", b)
	}
	if len(tt.a.undoBranches) != 1 || tt.a.undoBranches[0].name != "feature" || tt.a.undoBranches[0].hash != before {
		t.Fatalf("to bring back %+v, was at %q", tt.a.undoBranches, before)
	}
	if !tt.HasText(loc.BranchDeleted("feature")) {
		t.Fatalf("nothing offers it back: %q", tt.Texts())
	}
	tt.click(t, loc.Undo())
	tt.settle()
	after, err := tt.a.repo.BranchHash(ctx, "feature")
	if err != nil || after != before {
		t.Fatalf("feature at %q, was %q, %v", after, before, err)
	}
	if len(tt.a.undoBranches) != 0 || tt.HasText(loc.BranchDeleted("feature")) {
		t.Fatalf("still offering it back: %+v, %q", tt.a.undoBranches, tt.Texts())
	}
	// The current branch's menu offers no delete: git takes none for the
	// branch it is on.
	tt.click(t, loc.Branches())
	now, ok := tt.Find(loc.BranchCurrent())
	if !ok {
		t.Fatalf("no current branch; texts %q", tt.Texts())
	}
	head, _ := tt.Find(loc.BranchesLocal())
	tt.RightClickAt(head.X+head.W-20, now.Y+now.H/2)
	tt.settle()
	if menu := tt.Menu(); slices.Contains(menu, loc.Delete()) {
		t.Fatalf("the current branch offers to be deleted: %v", menu)
	}
	tt.Key(0, ui.KeyEscape)
	tt.settle()
	if b, _ := tt.a.repo.Branches(ctx); !slices.Contains(b, "main") {
		t.Fatalf("branches %v", b)
	}
}

// rightClickBranch presses the secondary button on a row of the local
// branches, row from the first: right of its name, as a press on the name
// would be the text's own menu, and 26 DIPs below the section's header.
func (tt *tester) rightClickBranch(t *testing.T, row int) {
	t.Helper()
	if !tt.a.branchesOpen {
		t.Fatal("the branches' popover is closed")
	}
	head, ok := tt.Find(loc.BranchesLocal())
	if !ok {
		t.Fatalf("no local branches; texts %q", tt.Texts())
	}
	tt.RightClickAt(head.X+head.W-20, head.Y+head.H+13+float32(row)*27)
	tt.settle()
}

// A close on the offer to bring a branch back takes it off, as goes on.
func TestUndoBranchOfferGoes(t *testing.T) {
	tt := newTester(t)
	tt.a.deleteBranch("feature")
	tt.settle()
	if len(tt.a.undoBranches) != 1 {
		t.Fatalf("offering %+v", tt.a.undoBranches)
	}
	tt.click(t, loc.Close())
	tt.Frame()
	if len(tt.a.undoBranches) != 0 || tt.HasText(loc.BranchDeleted("feature")) {
		t.Fatalf("still offering %+v: %q", tt.a.undoBranches, tt.Texts())
	}
	// Only the last few are kept, the oldest going.
	for _, b := range []string{"a", "b", "c", "d"} {
		if err := tt.a.repo.CreateBranch(context.Background(), b, "HEAD", false); err != nil {
			t.Fatal(err)
		}
		tt.a.deleteBranch(b)
		tt.settle()
	}
	var names []string
	for _, u := range tt.a.undoBranches {
		names = append(names, u.name)
	}
	if want := []string{"b", "c", "d"}; !slices.Equal(names, want) {
		t.Fatalf("offering %v, want %v", names, want)
	}
}

// Deleting a branch of a remote takes its name typed again: it goes for
// everyone who fetches the repository, and nothing here brings it back.
func TestScenarioDeleteRemoteBranchAsksForItsName(t *testing.T) {
	tt := newTester(t)
	bare := filepath.Join(t.TempDir(), "remote.git")
	if err := os.MkdirAll(bare, 0o755); err != nil {
		t.Fatal(err)
	}
	gitIn(t, bare, "init", "-q", "--bare", "-b", "main")
	gitIn(t, tt.a.repo.Dir, "remote", "add", "origin", bare)
	gitIn(t, tt.a.repo.Dir, "push", "-q", "origin", "feature")
	tt.a.refresh()
	tt.settle()
	if !slices.Contains(tt.a.remotes, "origin/feature") {
		t.Fatalf("no remote branch: %v", tt.a.remotes)
	}
	tt.a.askDeleteRemote("origin/feature")
	tt.Frame()
	if !tt.a.prompt.open {
		t.Fatal("the dialog did not open")
	}
	// The name as it is does not do, nor does part of it.
	for _, typed := range []string{"feature", "origin/feat", "main"} {
		tt.a.prompt.value = typed
		tt.Frame()
		tt.Key(0, ui.KeyEnter)
		tt.settle()
		if !tt.a.prompt.open {
			t.Fatalf("closed on %q", typed)
		}
		if slices.Contains(tt.a.remotes, "") {
			t.Fatal("went anyway")
		}
	}
	tt.a.prompt.value = "origin/feature"
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.settle()
	if tt.a.prompt.open {
		t.Fatal("the name typed again did not go")
	}
	if slices.Contains(tt.a.remotes, "origin/feature") || tt.a.errMsg != "" {
		t.Fatalf("remotes %v, error %q", tt.a.remotes, tt.a.errMsg)
	}
	// The dialog names what to type, and says what it costs.
	tt.a.askDeleteRemote("origin/feature")
	tt.Frame()
	if !tt.HasText(loc.BranchDeleteRemoteWarning("origin/feature")) {
		t.Fatalf("no warning: %q", tt.Texts())
	}
}
