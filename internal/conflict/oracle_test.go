package conflict

import (
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Where both git merge-file and New merge random edits of a text without
// a conflict, they merge them alike.
func TestAsGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	r := rand.New(rand.NewPCG(7, 8))
	dir := t.TempDir()
	clean := 0
	for range 300 {
		base := randomLines(r, 3+r.IntN(10))
		edit := func() []string {
			out := slices.Clone(base)
			for range 1 + r.IntN(2) {
				i := r.IntN(len(out))
				switch r.IntN(3) {
				case 0:
					out = slices.Insert(out, i, "new"+string(rune('a'+r.IntN(3)))+"\n")
				case 1:
					out = slices.Delete(out, i, i+1)
				default:
					out[i] = "changed" + string(rune('a'+r.IntN(3))) + "\n"
				}
			}
			return out
		}
		texts := []string{strings.Join(edit(), ""), strings.Join(base, ""), strings.Join(edit(), "")}
		args := []string{"merge-file", "-p"}
		for i, text := range texts {
			path := filepath.Join(dir, string(rune('0'+i)))
			if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
			args = append(args, path)
		}
		out, err := exec.Command("git", args...).Output()
		m := New(texts[1], texts[0], texts[2])
		if err != nil || m.Unresolved() > 0 {
			continue
		}
		clean++
		if string(out) != m.Result() {
			t.Fatalf("base %q, ours %q, theirs %q: git merges %q, New %q", texts[1], texts[0], texts[2], out, m.Result())
		}
	}
	if clean < 100 {
		t.Fatalf("only %d merges clean", clean)
	}
}
