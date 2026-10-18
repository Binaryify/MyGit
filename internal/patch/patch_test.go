package patch

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui/editor"
)

const before = "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten\n"

// after changes two at the top and two lines apart at the bottom.
const after = "one\nTWO\nthree\nfour\nfive\nsix\nseven\nEIGHT\nnine\nten\neleven\n"

type repo struct {
	t   *testing.T
	dir string
}

func newRepo(t *testing.T) *repo {
	r := &repo{t, t.TempDir()}
	r.git("init", "-q")
	r.write(before)
	r.git("add", ".")
	r.git("-c", "user.name=a", "-c", "user.email=a@a", "commit", "-q", "-m", "x")
	r.write(after)
	return r
}

func (r *repo) write(text string) {
	if err := os.WriteFile(filepath.Join(r.dir, "f.txt"), []byte(text), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func (r *repo) git(args ...string) string {
	r.t.Helper()
	return r.gitIn("", args...)
}

func (r *repo) gitIn(input string, args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.dir
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s\ninput:\n%s", args, err, out, input)
	}
	return string(out)
}

func (r *repo) patch(args ...string) editor.FilePatch {
	r.t.Helper()
	files, err := editor.ParsePatch(r.git(append([]string{"diff"}, args...)...))
	if err != nil || len(files) != 1 {
		r.t.Fatalf("%d files, %v", len(files), err)
	}
	return files[0]
}

// changed picks the patch's lines that read as given, "+TWO" say.
func changed(p editor.FilePatch, texts ...string) map[Line]bool {
	chosen := map[Line]bool{}
	for h, hunk := range p.Hunks {
		for i, l := range hunk.Lines {
			for _, t := range texts {
				if l == t {
					chosen[Line{h, i}] = true
				}
			}
		}
	}
	return chosen
}

func show(r *repo, what string) string { return r.git("show", what) }

func TestStageLines(t *testing.T) {
	r := newRepo(t)
	p := r.patch()
	// Stage TWO's change, and EIGHT added without eight deleted.
	r.gitIn(Select(p, changed(p, "-two", "+TWO", "+EIGHT"), false), "apply", "--cached", "-")
	want := "one\nTWO\nthree\nfour\nfive\nsix\nseven\neight\nEIGHT\nnine\nten\n"
	if got := show(r, ":f.txt"); got != want {
		t.Fatalf("index\n%s\nwant\n%s", got, want)
	}
	if got := show(r, ":f.txt"); strings.Contains(got, "eleven") {
		t.Fatal("eleven staged")
	}
}

func TestUnstageLines(t *testing.T) {
	r := newRepo(t)
	r.git("add", ".")
	p := r.patch("--cached")
	// Take eleven and EIGHT's change back out of the index.
	r.gitIn(Select(p, changed(p, "+eleven", "-eight", "+EIGHT"), true), "apply", "--cached", "--reverse", "-")
	want := "one\nTWO\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten\n"
	if got := show(r, ":f.txt"); got != want {
		t.Fatalf("index\n%s\nwant\n%s", got, want)
	}
}

func TestDiscardLines(t *testing.T) {
	r := newRepo(t)
	p := r.patch()
	r.gitIn(Select(p, changed(p, "-two", "+TWO"), true), "apply", "--reverse", "-")
	b, _ := os.ReadFile(filepath.Join(r.dir, "f.txt"))
	want := "one\ntwo\nthree\nfour\nfive\nsix\nseven\nEIGHT\nnine\nten\neleven\n"
	if string(b) != want {
		t.Fatalf("file\n%s\nwant\n%s", b, want)
	}
}

func TestNoNewlineAtEnd(t *testing.T) {
	r := newRepo(t)
	r.write(before + "no newline")
	p := r.patch()
	r.gitIn(Select(p, changed(p, "+no newline"), false), "apply", "--cached", "-")
	if got := show(r, ":f.txt"); got != before+"no newline" {
		t.Fatalf("index %q", got)
	}
}

func TestNothingChosen(t *testing.T) {
	r := newRepo(t)
	if s := Select(r.patch(), nil, false); s != "" {
		t.Fatalf("patch of nothing: %q", s)
	}
}

func TestLines(t *testing.T) {
	r := newRepo(t)
	p := r.patch()
	old, new := Lines(p)
	// The new side's lines, as the diff numbers them, are the patch's
	// context and additions.
	for _, l := range new {
		if c := p.Hunks[l.Hunk].Lines[l.Index][0]; c == '-' {
			t.Fatal("deletion on the new side")
		}
	}
	for _, l := range old {
		if c := p.Hunks[l.Hunk].Lines[l.Index][0]; c == '+' {
			t.Fatal("addition on the old side")
		}
	}
	if !Partial(p) {
		t.Fatal("a modified file can't be cut")
	}
}

func TestFileLines(t *testing.T) {
	r := newRepo(t)
	p := r.patch()
	old, new := FileLines(p)
	// "two" is the old file's line 1, "TWO" the new one's, "eleven" its 10.
	at := func(m map[int]Line, i int) string {
		l, ok := m[i]
		if !ok {
			return ""
		}
		return p.Hunks[l.Hunk].Lines[l.Index]
	}
	if at(old, 1) != "-two" || at(new, 1) != "+TWO" || at(new, 10) != "+eleven" || at(new, 0) != " one" {
		t.Fatalf("old 1 %q, new 1 %q, new 10 %q, new 0 %q", at(old, 1), at(new, 1), at(new, 10), at(new, 0))
	}
}
