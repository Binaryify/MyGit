package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"mygit/internal/git"
	loc "mygit/locales"
)

// TestOneList checks the one list as IDEA's commit window has it (#37): one
// list of changes, a check saying what the commit includes instead of
// staging, tracked changes checked by default and untracked files not, and
// the whole file shown against HEAD.
func TestOneList(t *testing.T) {
	defer func(one bool) { prefs.OneList = one }(prefs.OneList)
	prefs.OneList = true
	old := trash
	trash = os.Remove // what the trash does, without the trash
	t.Cleanup(func() { trash = old })
	tt := newTester(t)
	ctx := context.Background()
	indexEmpty := func() {
		t.Helper()
		if out, err := tt.a.repo.Git(ctx, "diff", "--cached", "--name-only"); err != nil || out != "" {
			t.Fatalf("index %q, %v", out, err)
		}
	}

	// Untracked, notes.txt is not checked; the tracked change made below
	// starts checked.
	tt.write(t, map[string]string{"main.go": "package main\n\nfunc main() { println(1) }\n"})
	tt.a.refresh()
	tt.settle()
	main, ok := find(tt.a.status.Changes, "main.go")
	if !ok || !tt.a.inCommit(main) {
		t.Fatalf("main.go not checked: %+v", tt.a.status.Changes)
	}
	if notes, ok := find(tt.a.status.Changes, "notes.txt"); !ok || tt.a.inCommit(notes) {
		t.Fatalf("notes.txt checked: %+v", tt.a.status.Changes)
	}
	indexEmpty()
	if len(tt.a.staged) != 0 || len(tt.a.unstaged) != 2 || tt.HasText(loc.Staged()) {
		t.Fatalf("lists: staged %v, unstaged %v, %q", tt.a.staged, tt.a.unstaged, tt.Texts())
	}

	// Chosen, the change shows the whole file against HEAD, editable.
	tt.click(t, "main.go")
	if d := tt.a.detail; d.change == nil || d.change.Path != "main.go" || !d.whole || d.diff == nil || d.staged {
		t.Fatalf("whole diff: %+v", d)
	}

	// Committing takes the checked change only, without staging it first.
	tt.a.message = "Change main"
	tt.click(t, loc.CommitFiles(1))
	if out, _ := tt.a.repo.Git(ctx, "log", "-1", "--format=%s"); strings.TrimSpace(out) != "Change main" {
		t.Fatalf("log %q", out)
	}
	if out, _ := tt.a.repo.Git(ctx, "show", "HEAD:main.go"); strings.TrimSpace(out) != "package main\n\nfunc main() { println(1) }" {
		t.Fatalf("committed %q", out)
	}
	if out, _ := tt.a.repo.Git(ctx, "status", "--porcelain"); strings.TrimSpace(out) != "?? notes.txt" {
		t.Fatalf("status %q", out)
	}
	indexEmpty()

	// Unchecked, a change stays out of the commit; checked, a new file goes
	// in, added as IDEA adds it.
	tt.write(t, map[string]string{"main.go": "package main\n\nfunc main() { println(2) }\n"})
	tt.a.refresh()
	tt.settle()
	tt.click(t, loc.ExcludeFromCommit("main.go"))
	tt.click(t, loc.IncludeInCommit("notes.txt"))
	tt.a.message = "Add notes"
	if !tt.a.canCommit() {
		t.Fatal("nothing to commit")
	}
	tt.click(t, loc.CommitFiles(1))
	if out, _ := tt.a.repo.Git(ctx, "show", "HEAD:notes.txt"); strings.TrimSpace(out) != "remember" {
		t.Fatalf("notes.txt committed %q", out)
	}
	if out, _ := tt.a.repo.Git(ctx, "status", "--porcelain"); strings.TrimSpace(out) != "M main.go" {
		t.Fatalf("status %q", out)
	}

	// A file added, not in HEAD, has its addition undone and the file
	// trashed when its change is discarded.
	tt.write(t, map[string]string{"added.txt": "x\n"})
	if _, err := tt.a.repo.Git(ctx, "add", "added.txt"); err != nil {
		t.Fatal(err)
	}
	tt.a.refresh()
	tt.settle()
	added, ok := find(tt.a.status.Changes, "added.txt")
	if !ok || added.Staged != 'A' {
		t.Fatalf("added %+v", tt.a.status.Changes)
	}
	if err := tt.a.discardChanges(ctx, []git.Change{added}); err != nil {
		t.Fatal(err)
	}
	tt.a.refresh()
	tt.settle()
	if out, _ := tt.a.repo.Git(ctx, "status", "--porcelain"); strings.TrimSpace(out) != "M main.go" {
		t.Fatalf("status after discard %q", out)
	}
}

// TestOneListRename checks that a rename goes into the commit as both of
// its paths, and that discarding it puts the file back as HEAD has it (#37).
func TestOneListRename(t *testing.T) {
	defer func(one bool) { prefs.OneList = one }(prefs.OneList)
	prefs.OneList = true
	old := trash
	trash = os.Remove // what the trash does, without the trash
	t.Cleanup(func() { trash = old })
	tt := newTester(t)
	ctx := context.Background()
	mv := func(from, to string) {
		t.Helper()
		cmd := exec.Command("git", "mv", from, to)
		cmd.Dir = tt.a.repo.Dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
		tt.a.refresh()
		tt.settle()
	}

	mv("main.go", "renamed.go")
	if ch, ok := find(tt.a.status.Changes, "renamed.go"); !ok || ch.OldPath != "main.go" || !tt.a.inCommit(ch) {
		t.Fatalf("rename %+v", tt.a.status.Changes)
	}
	tt.a.message = "Rename main"
	tt.click(t, loc.CommitFiles(1))
	if out, _ := tt.a.repo.Git(ctx, "show", "HEAD:renamed.go"); strings.TrimSpace(out) != "package main\n\nfunc main() {}" {
		t.Fatalf("renamed %q", out)
	}
	if _, err := tt.a.repo.Git(ctx, "show", "HEAD:main.go"); err == nil {
		t.Fatal("the old path is still committed")
	}

	mv("renamed.go", "other.go")
	ch, ok := find(tt.a.status.Changes, "other.go")
	if !ok || ch.OldPath != "renamed.go" {
		t.Fatalf("rename back %+v", tt.a.status.Changes)
	}
	if err := tt.a.discardChanges(ctx, []git.Change{ch}); err != nil {
		t.Fatal(err)
	}
	tt.a.refresh()
	tt.settle()
	if out, _ := tt.a.repo.Git(ctx, "status", "--porcelain"); strings.TrimSpace(out) != "?? notes.txt" {
		t.Fatalf("status %q", out)
	}
}

// TestOneListImageSides checks that the one list's pictures compare HEAD
// with the working tree, as its diffs do (#37).
func TestOneListImageSides(t *testing.T) {
	defer func(one bool) { prefs.OneList = one }(prefs.OneList)
	prefs.OneList = true
	old, now := changeSides(git.Change{Path: "logo.png", Work: 'M'}, false)
	if old == nil || old.rev != "HEAD" || old.path != "logo.png" || now == nil || !now.worktree {
		t.Fatalf("modified: %+v, %+v", old, now)
	}
	old, now = changeSides(git.Change{Path: "new.png", Work: '?'}, false)
	if old != nil || now == nil || !now.worktree {
		t.Fatalf("new: %+v, %+v", old, now)
	}
	old, now = changeSides(git.Change{Path: "gone.png", Staged: 'D'}, false)
	if old == nil || old.rev != "HEAD" || now != nil {
		t.Fatalf("deleted: %+v, %+v", old, now)
	}
}

// TestOneListMerge checks that the one list concludes a merge: git takes
// no partial commit then, so the files checked are staged and the merge
// commits the index.
func TestOneListMerge(t *testing.T) {
	defer func(one bool) { prefs.OneList = one }(prefs.OneList)
	prefs.OneList = true
	tt := newTester(t)
	dir := tt.a.repo.Dir
	gitIn(t, dir, "switch", "-q", "-c", "side")
	os.WriteFile(filepath.Join(dir, "feature.go"), []byte("package main\n\nfunc feature() { println() }\n"), 0o644)
	gitIn(t, dir, "commit", "-q", "-am", "Side")
	gitIn(t, dir, "switch", "-q", "main")
	gitIn(t, dir, "merge", "-q", "--no-ff", "--no-commit", "side")
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() { feature() }\n"), 0o644)
	tt.a.refresh()
	tt.settle()
	if tt.a.op != git.Merging || tt.a.commitCount() != 2 {
		t.Fatalf("op %q, %d to commit: %+v", tt.a.op, tt.a.commitCount(), tt.a.status.Changes)
	}
	tt.a.message = "Merge side"
	tt.a.commit()
	tt.settle()
	if tt.a.errMsg != "" {
		t.Fatalf("merge commit: %s", tt.a.errMsg)
	}
	if out := gitIn(t, dir, "log", "-1", "--format=%P %s"); len(strings.Fields(out)) != 4 || !strings.HasSuffix(strings.TrimSpace(out), "Merge side") {
		t.Fatalf("not a merge: %q", out)
	}
	if out := gitIn(t, dir, "show", "HEAD:main.go"); !strings.Contains(out, "feature()") {
		t.Fatalf("main.go %q", out)
	}
}

// TestOneListAmend checks that amending with a file checked keeps the
// index's other changes staged, and out of the commit.
func TestOneListAmend(t *testing.T) {
	defer func(one bool) { prefs.OneList = one }(prefs.OneList)
	prefs.OneList = true
	tt := newTester(t)
	dir := tt.a.repo.Dir
	gitIn(t, dir, "add", "notes.txt")
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() { println() }\n"), 0o644)
	tt.a.refresh()
	tt.settle()
	notes, _ := find(tt.a.status.Changes, "notes.txt")
	tt.a.chooseCommit(notes, false)
	tt.a.message, tt.a.amend = "Write main\r\n\r\nAmended\r", true
	tt.a.commit()
	tt.settle()
	if tt.a.errMsg != "" {
		t.Fatalf("amend: %s", tt.a.errMsg)
	}
	if out := gitIn(t, dir, "diff", "--cached", "--name-only"); strings.TrimSpace(out) != "notes.txt" {
		t.Fatalf("index %q", out)
	}
	if out := gitIn(t, dir, "show", "--name-only", "--format=%B", "HEAD"); !strings.Contains(out, "main.go") || strings.Contains(out, "notes.txt") || strings.Contains(out, "\r") {
		t.Fatalf("amended %q", out)
	}
}

// TestOneListFirstCommit checks the one list before the first commit,
// when there is no HEAD to show the changes against.
func TestOneListFirstCommit(t *testing.T) {
	defer func(one bool) { prefs.OneList = one }(prefs.OneList)
	prefs.OneList = true
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(dir, "first.txt"), []byte("hello\n"), 0o644)
	gitIn(t, dir, "add", ".")
	repo, err := git.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	tt := newTesterOn(repo)
	tt.click(t, "first.txt")
	if d := tt.a.detail; tt.a.errMsg != "" || d.diff == nil {
		t.Fatalf("first file: error %q, %+v", tt.a.errMsg, d)
	}
	tt.a.message = "First"
	tt.a.commit()
	tt.settle()
	if out := gitIn(t, dir, "show", "HEAD:first.txt"); out != "hello" || tt.a.errMsg != "" {
		t.Fatalf("committed %q, %q", out, tt.a.errMsg)
	}
}
