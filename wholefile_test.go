package main

import (
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui/editor"
)

// linesOf is n lines of a file, the ones at changed saying so.
func linesOf(n int, changed ...int) string {
	var b strings.Builder
	yes := map[int]bool{}
	for _, i := range changed {
		yes[i] = true
	}
	for i := range n {
		if yes[i] {
			b.WriteString("changed ")
		}
		b.WriteString("line " + strconv.Itoa(i) + "\n")
	}
	return b.String()
}

// showFileNamed shows the file of the commit shown by its name.
func showFileNamed(t *testing.T, tt *tester, name string) {
	t.Helper()
	for i, f := range tt.a.detail.files {
		if fileName(f) == name {
			tt.a.showFile(i)
			tt.settle()
			return
		}
	}
	t.Fatalf("no file %q in %v", name, tt.a.detail.files)
}

// A commit's diff shows the whole file, as IntelliJ's does, its changes
// marked where they are in it, not the patch's hunks alone.
func TestCommitShowsWholeFile(t *testing.T) {
	defer func(on bool) { prefs.FoldUnchanged = on }(prefs.FoldUnchanged)
	prefs.FoldUnchanged = false
	tt := newTester(t)
	tt.commitFiles(t, "Change two lines", map[string]string{"a.txt": linesOf(40)})
	tt.commitFiles(t, "Change two lines", map[string]string{"a.txt": linesOf(40, 5, 30)})
	tt.click(t, "Change two lines")
	showFileNamed(t, tt, "a.txt")
	d := tt.a.detail
	if !d.patchWhole || d.diff == nil {
		t.Fatalf("shown by hunks alone: %+v", d)
	}
	if got, want := d.diff.New.LineCount(), 41; got != want {
		t.Errorf("new side %d lines, want the file's %d", got, want)
	}
	if got, want := d.diff.Old.LineCount(), 41; got != want {
		t.Errorf("old side %d lines, want the file's %d", got, want)
	}
	hunks := d.diff.Hunks()
	if len(hunks) != 2 {
		t.Fatalf("%d changes, want 2", len(hunks))
	}
	if hunks[0].NewStart != 5 || hunks[1].NewStart != 30 {
		t.Errorf("changes at %d and %d, want the file's lines 5 and 30", hunks[0].NewStart, hunks[1].NewStart)
	}
	// A file short enough shows all its lines, away from its changes too.
	if d.fold || !d.diff.ExpandUnchanged {
		t.Error("a short file opens folded")
	}
}

// Folding a commit's file is kept for the files opened next, short ones
// too, until it is turned off (#47).
func TestCommitFoldKept(t *testing.T) {
	defer func(on bool) { prefs.FoldUnchanged = on }(prefs.FoldUnchanged)
	prefs.FoldUnchanged = false
	tt := newTester(t)
	tt.commitFiles(t, "Change two files", map[string]string{"a.txt": linesOf(40), "b.txt": linesOf(40)})
	tt.commitFiles(t, "Change two files", map[string]string{"a.txt": linesOf(40, 5), "b.txt": linesOf(40, 30)})
	tt.click(t, "Change two files")
	showFileNamed(t, tt, "a.txt")
	tt.click(t, "折叠未更改")
	if !prefs.FoldUnchanged || tt.a.detail.diff.ExpandUnchanged {
		t.Fatalf("not folded: pref %v", prefs.FoldUnchanged)
	}
	showFileNamed(t, tt, "b.txt")
	if d := tt.a.detail; !d.fold || d.diff.ExpandUnchanged {
		t.Fatal("the next file opens unfolded")
	}
	tt.click(t, "折叠未更改")
	showFileNamed(t, tt, "a.txt")
	if d := tt.a.detail; prefs.FoldUnchanged || d.fold || !d.diff.ExpandUnchanged {
		t.Fatal("unfolding is not kept")
	}
}

// A file long enough opens with the lines away from its changes folded,
// as IntelliJ folds them, and the button shows them all.
func TestLongFileOpensFolded(t *testing.T) {
	defer func(on bool) { prefs.FoldUnchanged = on }(prefs.FoldUnchanged)
	prefs.FoldUnchanged = false
	tt := newTester(t)
	tt.commitFiles(t, "Change one line", map[string]string{"a.txt": linesOf(2600)})
	tt.commitFiles(t, "Change one line", map[string]string{"a.txt": linesOf(2600, 1200)})
	tt.click(t, "Change one line")
	showFileNamed(t, tt, "a.txt")
	d := tt.a.detail
	if !d.patchWhole || d.diff == nil || d.diff.New.LineCount() != 2601 {
		t.Fatalf("not whole: %+v", d)
	}
	if !d.fold || d.diff.ExpandUnchanged {
		t.Error("a long file opens with its unchanged lines shown")
	}
	tt.click(t, "折叠未更改")
	if !d.diff.ExpandUnchanged {
		t.Error("the button does not show the unchanged lines")
	}
}

// The longest files are left to their hunks, read or not.
func TestPatchTooLong(t *testing.T) {
	short := editor.FilePatch{Hunks: []editor.PatchHunk{{OldStart: 10, OldLines: 7, NewStart: 10, NewLines: 7}}}
	if patchTooLong(short) {
		t.Error("a short patch is too long")
	}
	long := editor.FilePatch{Hunks: []editor.PatchHunk{
		{OldStart: 10, OldLines: 7, NewStart: 10, NewLines: 7},
		{OldStart: 60000, OldLines: 7, NewStart: 60000, NewLines: 7},
	}}
	if !patchTooLong(long) {
		t.Error("a patch of 60000 lines is not too long")
	}
}

// A diff leaving whitespace changes out does not make the new file from
// the old one, so the file shows by its hunks, as it did.
func TestWhitespaceDiffShowsHunks(t *testing.T) {
	defer func(w string) { prefs.Whitespace = w }(prefs.Whitespace)
	tt := newTester(t)
	tt.commitFiles(t, "Space", map[string]string{"a.txt": "one\ntwo \n"})
	tt.commitFiles(t, "Space", map[string]string{"a.txt": "one\ntwo	\nthree\n"})
	prefs.Whitespace = "all"
	tt.click(t, "Space") // the patch the diffs read leaves the whitespace out of
	showFileNamed(t, tt, "a.txt")
	d := tt.a.detail
	if d.patchWhole {
		t.Error("a diff ignoring whitespace shows the whole file")
	}
	if d.diff == nil || d.diff.New.LineCount() >= 4 {
		t.Fatalf("not the patch's lines alone: %+v", d.diff)
	}
}

// The texts of a commit's files come with its patch, in one call of git,
// so that choosing a file shows it at once.
func TestCommitKeepsTheFilesTexts(t *testing.T) {
	tt := newTester(t)
	tt.commitFiles(t, "Two files", map[string]string{"a.txt": linesOf(40), "b.txt": linesOf(60)})
	tt.commitFiles(t, "Two files", map[string]string{"a.txt": linesOf(40, 5), "b.txt": linesOf(60, 50)})
	tt.click(t, "Two files")
	d := tt.a.detail
	if len(d.texts) != 2 {
		t.Fatalf("%d files' texts kept, want 2: %v", len(d.texts), d.texts)
	}
	for i, f := range d.files {
		name := fileName(f)
		tt.a.showFile(i)
		if tt.a.detail.loading || tt.a.detail.diff == nil || !tt.a.detail.patchWhole {
			t.Fatalf("%s waits for git: loading %v", name, tt.a.detail.loading)
		}
	}
}

// A staged change shows whole too, its lines still staged one at a time.
func TestStagedChangeShowsWholeFile(t *testing.T) {
	tt := newTester(t)
	tt.commitFiles(t, "First", map[string]string{"a.txt": linesOf(40)})
	tt.write(t, map[string]string{"a.txt": linesOf(40, 5, 30)})
	if _, err := tt.a.repo.Git(t.Context(), "add", "a.txt"); err != nil {
		t.Fatal(err)
	}
	tt.a.refresh()
	tt.settle()
	tt.a.chooseChange(0, true)
	tt.settle()
	d := tt.a.detail
	if !d.patchWhole || d.diff == nil {
		t.Fatalf("staged change shown by hunks alone: %+v", d)
	}
	if got, want := d.diff.New.LineCount(), 41; got != want {
		t.Errorf("new side %d lines, want the file's %d", got, want)
	}
	if d.fold {
		t.Error("a file of 40 lines opens folded")
	}
	// The lines of the file are the patch's, staging one of them.
	_, newLine := tt.a.lineMap()
	if _, found := newLine(d.diff.Hunks()[0].NewStart); !found {
		t.Error("the first change's line is not in the patch")
	}
}
