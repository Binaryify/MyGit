package main

import (
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"

	loc "mygit/locales"
)

// TestAnnotateDiff shows who last changed each line beside a commit's
// diff, and a click on a line shows the commit that made it (#51).
func TestAnnotateDiff(t *testing.T) {
	defer func(on, unified bool) { prefs.Annotate, prefs.Unified = on, unified }(prefs.Annotate, prefs.Unified)
	prefs.Annotate, prefs.Unified = false, true
	tt := newTester(t)
	tt.click(t, "Write main")
	if tt.a.detail.notes != nil {
		t.Fatal("annotated while off")
	}
	tt.click(t, loc.Annotate())
	tt.settle()
	d := &tt.a.detail
	n := d.notes
	if !prefs.Annotate || n == nil || n.diff != d.diff {
		t.Fatalf("annotations %+v", n)
	}
	// main.go: "package main" from Start, the two lines after from Write main.
	if old, now := n.sides[editor.DiffOld], n.sides[editor.DiffNew]; len(old) != 1 || old[0].Summary != "Start" ||
		len(now) != 3 || now[0].Summary != "Start" || now[2].Summary != "Write main" {
		t.Fatalf("blamed %+v", n.sides)
	}
	rows, lh := annotatedRows(d.diff, []editor.DiffSide{editor.DiffNew, editor.DiffOld})
	if len(rows) != 3 || lh <= 0 || rows[0].line != 0 || rows[2].line != 2 {
		t.Fatalf("rows %+v, %v", rows, lh)
	}
	col, ok := tt.Find(loc.AnnotateTip())
	if !ok {
		t.Fatalf("no column; texts %q", tt.Texts())
	}
	// Its first line is Start's.
	tt.ClickAt(col.X+col.W/2, col.Y+float32(rows[0].top+lh/2))
	tt.settle()
	if c := tt.a.detail.commit; c == nil || c.Subject != "Start" {
		t.Fatalf("shown %+v", c)
	}
	// Turned off, no column shows.
	tt.click(t, loc.Annotate())
	if _, ok := tt.Find(loc.AnnotateTip()); ok || prefs.Annotate {
		t.Fatal("still annotated")
	}
}

// TestAnnotateSplit annotates each side of a split diff beside it, and a
// staged change's index against HEAD.
func TestAnnotateSplit(t *testing.T) {
	defer func(on, unified bool) { prefs.Annotate, prefs.Unified = on, unified }(prefs.Annotate, prefs.Unified)
	prefs.Annotate, prefs.Unified = true, false
	tt := newTester(t)
	tt.write(t, map[string]string{"main.go": "package main\n\nfunc main() { println() }\n"})
	tt.a.stagePaths("main.go")
	tt.settle()
	tt.click(t, "main.go")
	tt.settle()
	d := &tt.a.detail
	if d.change == nil || !d.staged || d.notes == nil {
		t.Fatalf("change %+v, notes %+v", d.change, d.notes)
	}
	old, now := d.notes.sides[editor.DiffOld], d.notes.sides[editor.DiffNew]
	if len(old) != 3 || old[2].Summary != "Write main" || len(now) != 3 || !now[2].NotCommitted() || now[0].Summary != "Start" {
		t.Fatalf("blamed %+v", d.notes.sides)
	}
	n := 0
	for _, r := range tt.a.detail.notes.has {
		if r {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("sides %v", d.notes.has)
	}
}

// TestDumpAnnotate renders the annotations beside a split diff, a unified
// one and a line's tooltip into MYGIT_DUMP, to look at:
//
//	MYGIT_DUMP=/tmp go test -run DumpAnnotate
func TestDumpAnnotate(t *testing.T) {
	out := os.Getenv("MYGIT_DUMP")
	if out == "" {
		t.Skip()
	}
	defer func(on, unified, wrap bool) { prefs.Annotate, prefs.Unified, prefs.Wrap = on, unified, wrap }(prefs.Annotate, prefs.Unified, prefs.Wrap)
	prefs.Annotate, prefs.Unified, prefs.Wrap = true, false, false
	tt := newTester(t)
	dir := tt.a.repo.Dir
	commit := func(who, when, msg string, lines []string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "util.go"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{{"add", "."}, {"commit", "-q", "-m", msg}} {
			cmd := exec.Command("git", args...)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME="+who, "GIT_AUTHOR_EMAIL=a@example.com", "GIT_AUTHOR_DATE="+when,
				"GIT_COMMITTER_NAME="+who, "GIT_COMMITTER_EMAIL=a@example.com", "GIT_COMMITTER_DATE="+when)
			if b, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, b)
			}
		}
	}
	lines := []string{"package main", "", "import (", "\t\"fmt\"", "\t\"strings\"", ")", "",
		"func greet(name string) string {", "\treturn \"hello, \" + name", "}", "",
		"func shout(s string) string {", "\treturn strings.ToUpper(s)", "}", "",
		"func main() {", "\tfmt.Println(greet(\"world\"))", "}"}
	commit("Ada Lovelace", "2025-09-28T10:00:00", "Add util.go", lines)
	lines[8] = "\treturn fmt.Sprintf(\"hello, %s\", name)"
	commit("Grace Hopper", "2025-11-12T15:30:00", "Format the greeting", lines)
	lines[12] = "\treturn strings.ToUpper(s) + \"!\""
	commit("Linus", "2026-06-08T09:00:00", "Shout louder", lines)
	lines = append(lines[:16], "\tfmt.Println(shout(greet(\"world\")))", "}")
	lines[4] = "\t\"strings\" // for shout"
	tt.write(t, map[string]string{"util.go": strings.Join(lines, "\n") + "\n"})
	tt.a.refresh()
	tt.settle()
	tt.click(t, "util.go")
	tt.settle()
	save := func(name string) {
		t.Helper()
		tt.Frame()
		f, err := os.Create(filepath.Join(out, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	save("annotate-split")
	col, ok := tt.Find(loc.AnnotateTip())
	if !ok {
		t.Fatal("no column")
	}
	rows, lh := annotatedRows(tt.a.detail.diff, []editor.DiffSide{editor.DiffOld})
	tt.Move(col.X+col.W/2, col.Y+float32(rows[8].top+lh/2))
	tt.Frame()
	tt.Advance(time.Second) // the tooltip's delay
	tt.Frame()
	tt.Move(col.X+col.W/2+1, col.Y+float32(rows[8].top+lh/2))
	tt.Advance(time.Second)
	save("annotate-tooltip")
	prefs.Unified = true
	tt.a.reloadDiff()
	tt.settle()
	save("annotate-unified")
	tt.click(t, "Shout louder")
	tt.settle()
	save("annotate-commit")
}
