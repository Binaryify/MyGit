package conflict

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// lcs returns the length of the longest common subsequence of a and b.
func lcs(a, b []string) int {
	dp := make([]int, len(b)+1)
	for i := range a {
		prev := 0
		for j := range b {
			cur := dp[j+1]
			if a[i] == b[j] {
				dp[j+1] = prev + 1
			} else {
				dp[j+1] = max(dp[j+1], dp[j])
			}
			prev = cur
		}
	}
	return dp[len(b)]
}

// apply turns a into b with hunks.
func apply(a, b []string, hunks []Hunk) []string {
	var out []string
	at := 0
	for _, h := range hunks {
		out = append(out, a[at:h.A.Start]...)
		out = append(out, b[h.B.Start:h.B.End]...)
		at = h.A.End
	}
	return append(out, a[at:]...)
}

func randomLines(r *rand.Rand, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = string(rune('a'+r.IntN(4))) + "\n"
	}
	return out
}

// Diff's hunks turn one text into the other, keep as many lines as can
// be kept, and leave a line alike between them.
func TestDiff(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for range 2000 {
		a, b := randomLines(r, r.IntN(12)), randomLines(r, r.IntN(12))
		hunks := Diff(a, b)
		if got := apply(a, b, hunks); !slices.Equal(got, b) {
			t.Fatalf("%q to %q: hunks %v make %q", a, b, hunks, got)
		}
		changed := 0
		for i, h := range hunks {
			changed += h.A.Len()
			if h.A.Len() == 0 && h.B.Len() == 0 || i > 0 && h.A.Start <= hunks[i-1].A.End {
				t.Fatalf("%q to %q: hunks %v", a, b, hunks)
			}
		}
		if kept := len(a) - changed; kept != lcs(a, b) {
			t.Fatalf("%q to %q: kept %d lines, could %d", a, b, kept, lcs(a, b))
		}
	}
}

// Texts too different to match up are changed whole.
func TestDiffTooDifferent(t *testing.T) {
	var a, b []string
	for i := range 3000 {
		a = append(a, fmt.Sprintf("a%d\n", i))
		b = append(b, fmt.Sprintf("b%d\n", i))
	}
	b[0] = a[0]
	hunks := Diff(a, b)
	if len(hunks) != 1 || hunks[0] != (Hunk{Range{1, 3000}, Range{1, 3000}}) || !slices.Equal(apply(a, b, hunks), b) {
		t.Fatalf("%v", hunks)
	}
}

func TestLines(t *testing.T) {
	for text, want := range map[string][]string{
		"": nil, "a": {"a"}, "a\n": {"a\n"}, "a\nb": {"a\n", "b"}, "\n\n": {"\n", "\n"},
	} {
		if got := Lines(text); !slices.Equal(got, want) {
			t.Errorf("Lines(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestNew(t *testing.T) {
	base := "1\n2\n3\n4\n5\n6\n7\n"
	m := New(base, "1\nTWO\n3\n4\n5\n6\n7\nours\n", "1\n2\n3\n4\nfive\n6\n7\n", false)
	if got := m.Result(); got != "1\nTWO\n3\n4\nfive\n6\n7\nours\n" || m.Unresolved() != 0 || len(m.Chunks) != 3 {
		t.Fatalf("result %q, chunks %+v", got, m.Chunks)
	}
	if c := m.Chunks[1]; c.State != [2]State{Unchanged, Applied} || c.Base != (Range{4, 5}) || c.Result != (Range{4, 5}) {
		t.Errorf("their change %+v", c)
	}

	// A conflict shows as the base has it, between the changes taken.
	m = New(base, "1\n2\nours\n4\n5\n6\n7\n", "1\n2\ntheirs\n4\n5\n6\nseven\n", false)
	if got := m.Result(); got != "1\n2\n3\n4\n5\n6\nseven\n" || m.Unresolved() != 1 {
		t.Fatalf("result %q, chunks %+v", got, m.Chunks)
	}
	c := m.Chunks[0]
	if !c.Conflict || c.State != [2]State{Pending, Pending} || c.Sides != [2]Range{{2, 3}, {2, 3}} || c.Result != (Range{2, 3}) {
		t.Fatalf("conflict %+v", c)
	}

	// Changes alike are no conflict; changes touching are one, as git
	// has them.
	m = New(base, "1\nx\n3\n4\n5\n6\n7\n", "1\nx\n3\n4\n5\n6\n7\n", false)
	if m.Unresolved() != 0 || m.Result() != "1\nx\n3\n4\n5\n6\n7\n" || m.Chunks[0].State != [2]State{Applied, Applied} {
		t.Fatalf("alike: %q, %+v", m.Result(), m.Chunks)
	}
	m = New(base, "1\nx\n3\n4\n5\n6\n7\n", "1\n2\ny\n4\n5\n6\n7\n", false)
	if m.Unresolved() != 1 || m.Chunks[0].Base != (Range{1, 3}) || m.Result() != base {
		t.Fatalf("touching: %q, %+v", m.Result(), m.Chunks)
	}

	// Each side removing one of the same alike lines removes the same
	// one, not two.
	m = New("a\nz\nb\nb\nb\nc\n", "a\nz\nb\nb\nc\n", "x\nz\nb\nb\nc\n", false)
	if m.Unresolved() != 0 || m.Result() != "x\nz\nb\nb\nc\n" {
		t.Fatalf("alike lines: %q, %+v", m.Result(), m.Chunks)
	}

	// No base, as two files added.
	m = New("", "a\n", "b\n", false)
	if m.Unresolved() != 1 || m.Result() != "" || m.Chunks[0].Sides != [2]Range{{0, 1}, {0, 1}} {
		t.Fatalf("added: %q, %+v", m.Result(), m.Chunks)
	}
}

// With a side unchanged, the merge is the other side; taking a side's
// change everywhere makes the result that side where the other has no
// change of its own.
func TestNewRandom(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	for range 2000 {
		base, ours, theirs := randomLines(r, r.IntN(10)), randomLines(r, r.IntN(10)), randomLines(r, r.IntN(10))
		join := func(l []string) string { return strings.Join(l, "") }
		if m := New(join(base), join(ours), join(base), false); m.Result() != join(ours) || m.Unresolved() != 0 {
			t.Fatalf("theirs unchanged: %q, %q: %q", base, ours, m.Result())
		}
		if m := New(join(base), join(ours), join(ours), false); m.Result() != join(ours) || m.Unresolved() != 0 {
			t.Fatalf("alike: %q, %q: %q", base, ours, m.Result())
		}
		m := New(join(base), join(ours), join(theirs), false)
		for i := range m.Chunks {
			e := m.Apply(i, Ours)
			m.Ignore(i, Theirs)
			if e != (Edit{}) && !strings.HasPrefix(m.Result()[e.Start:], e.Text) {
				t.Fatalf("edit %+v of %q", e, m.Result())
			}
		}
		for i, c := range m.Chunks {
			if got := m.result[c.Result.Start:c.Result.End]; c.State[Ours] == Applied && !slices.Equal(got, m.lines(Ours, &m.Chunks[i])) {
				t.Fatalf("%q %q %q: chunk %+v holds %q", base, ours, theirs, c, got)
			}
		}
		if m.Unresolved() != 0 {
			t.Fatalf("unresolved after taking a side everywhere: %+v", m.Chunks)
		}
	}
}

func TestApply(t *testing.T) {
	m := New("a\nb\nc\n", "a\nours\nc\n", "a\ntheirs\nc\n", false)
	e := m.Apply(0, Theirs)
	if e != (Edit{Start: 2, End: 4, Text: "theirs\n"}) || m.Result() != "a\ntheirs\nc\n" || m.Chunks[0].Resolved() {
		t.Fatalf("edit %+v, result %q, %+v", e, m.Result(), m.Chunks[0])
	}
	// The other side goes after it.
	e = m.Apply(0, Ours)
	if e != (Edit{Start: 2, End: 9, Text: "theirs\nours\n"}) || m.Result() != "a\ntheirs\nours\nc\n" || !m.Chunks[0].Resolved() {
		t.Fatalf("edit %+v, result %q, %+v", e, m.Result(), m.Chunks[0])
	}
	if m.Apply(0, Ours) != (Edit{}) {
		t.Fatal("applied twice")
	}

	m = New("a\nb\nc\n", "a\nours\nc\n", "a\ntheirs\nc\n", false)
	m.Ignore(0, Ours)
	if m.Chunks[0].Resolved() || m.Result() != "a\nb\nc\n" {
		t.Fatalf("%+v", m.Chunks[0])
	}
	m.Ignore(0, Theirs)
	if !m.Chunks[0].Resolved() || m.Result() != "a\nb\nc\n" {
		t.Fatalf("%+v", m.Chunks[0])
	}
}

// Chunks move with the lines edited around them; one edited is what its
// text is, so undoing a side taken makes it pending again.
func TestSync(t *testing.T) {
	m := New("a\nb\nc\nd\ne\n", "a\nB1\nc\nd\nE\n", "a\nB2\nc\nd\ne\n", false)
	m.Sync("new\na\nb\nc\nd\nE\n")
	if m.Chunks[0].Result != (Range{2, 3}) || m.Chunks[1].Result != (Range{5, 6}) || m.Unresolved() != 1 {
		t.Fatalf("lines added above: %+v", m.Chunks)
	}
	m.Sync("new\na\nB2\nc\nd\nE\n")
	if c := m.Chunks[0]; c.State != [2]State{Ignored, Applied} || c.Result != (Range{2, 3}) {
		t.Fatalf("their side typed: %+v", c)
	}
	m.Sync("new\na\nb\nc\nd\nE\n") // undone
	if c := m.Chunks[0]; c.State != [2]State{Pending, Pending} {
		t.Fatalf("undone: %+v", c)
	}
	m.Sync("new\na\nsomething\nelse\nc\nd\nE\n")
	if c := m.Chunks[0]; !c.Resolved() || c.Result != (Range{2, 4}) || m.Chunks[1].Result != (Range{6, 7}) {
		t.Fatalf("by hand: %+v", m.Chunks)
	}
	m.Sync("new\na\nsomething\nelse\nc\nd\n")
	if c := m.Chunks[1]; c.Result != (Range{6, 6}) || c.State != [2]State{Ignored, Unchanged} {
		t.Fatalf("our change deleted: %+v", c)
	}

	// Lines typed into a chunk that is empty in the result are its.
	m = New("a\nb\nc\n", "a\nc\n", "a\nB\nc\n", false)
	if c := m.Chunks[0]; c.Result != (Range{1, 2}) {
		t.Fatalf("%+v", c)
	}
	m.Apply(0, Ours)
	if c := m.Chunks[0]; c.Result != (Range{1, 1}) || m.Result() != "a\nc\n" {
		t.Fatalf("%+v %q", c, m.Result())
	}
	m.Sync("a\ntyped\nc\n")
	if c := m.Chunks[0]; c.Result != (Range{1, 2}) || !c.Resolved() {
		t.Fatalf("%+v", c)
	}
}

func TestMapLine(t *testing.T) {
	hunks := []Hunk{{Range{1, 1}, Range{1, 3}}, {Range{3, 5}, Range{5, 6}}}
	for _, c := range []struct {
		p    int
		end  bool
		want int
	}{
		{0, false, 0}, {1, false, 1}, {1, true, 3}, {2, false, 4}, {3, false, 5}, {3, true, 5},
		{4, false, 5}, {4, true, 6}, {5, false, 6}, {6, true, 7},
	} {
		if got := mapLine(c.p, hunks, c.end); got != c.want {
			t.Errorf("mapLine(%d, end %v) = %d, want %d", c.p, c.end, got, c.want)
		}
	}
}

func TestResolveSimple(t *testing.T) {
	m := New("call(a, b)\n", "call(x, b)\n", "call(a, y)\n", false)
	if m.Unresolved() != 1 {
		t.Fatalf("%+v", m.Chunks)
	}
	e, ok := m.ResolveSimple(0)
	if !ok || e.Text != "call(x, y)\n" || m.Result() != "call(x, y)\n" || m.Unresolved() != 0 {
		t.Fatalf("%v %+v %q", ok, e, m.Result())
	}
	m = New("call(a, b)\n", "call(x, b)\n", "call(z, b)\n", false)
	if _, ok := m.ResolveSimple(0); ok || m.Unresolved() != 1 {
		t.Fatal("resolved a real conflict")
	}
	// Lines added on both sides of a line changed.
	m = New("a\nb\n", "top\na\nb\n", "a\nb\nbottom\n", false)
	if m.Unresolved() != 0 || m.Result() != "top\na\nb\nbottom\n" {
		t.Fatalf("%q", m.Result())
	}
}

func TestWords(t *testing.T) {
	got := Words("foo_1(bar,  baz)\n\t世界!")
	want := []string{"foo_1", "(", "bar", ",", "  ", "baz", ")", "\n", "\t", "世界", "!"}
	if !slices.Equal(got, want) {
		t.Fatalf("%q", got)
	}
}

// Ignoring spaces, a side changing only spaces changes nothing, and the
// lines keep our spaces.
func TestIgnoreSpace(t *testing.T) {
	base, ours, theirs := "a\n  b\nc\n", "a\n\tb\nc\n", "a\n  b \nC\n"
	if m := New(base, ours, theirs, false); m.Unresolved() != 1 {
		t.Fatalf("not ignoring: %+v", m.Chunks)
	}
	m := New(base, ours, theirs, true)
	if m.Unresolved() != 0 || m.Result() != "a\n\tb\nC\n" {
		t.Fatalf("%q, %+v", m.Result(), m.Chunks)
	}
	m = New("a\nb\n", "a \nB\n", "a\n b\n", true)
	if m.Unresolved() != 0 || m.Result() != "a \nB\n" {
		t.Fatalf("%q, %+v", m.Result(), m.Chunks)
	}
}

func TestUnignore(t *testing.T) {
	m := New("a\nb\nc\n", "a\nours\nc\n", "a\ntheirs\nc\n", false)
	m.Ignore(0, Ours)
	m.Unignore(0, Ours)
	if m.Chunks[0].State != [2]State{Pending, Pending} {
		t.Fatalf("%+v", m.Chunks[0])
	}
}

func TestChangedWords(t *testing.T) {
	m := New("a\nx := 1 // one\nb\nc\n", "a\nx := 2 // one\nb\nc\n", "a\ny := 1 // one\nb\nc\n", false)
	if len(m.Chunks) != 1 || !m.Chunks[0].Conflict {
		t.Fatalf("chunks %+v", m.Chunks)
	}
	if got := m.ChangedWords(0, Ours); !slices.Equal(got, []Range{{7, 8}}) {
		t.Errorf("ours %v", got)
	}
	if got := m.ChangedWords(0, Theirs); !slices.Equal(got, []Range{{2, 3}}) {
		t.Errorf("theirs %v", got)
	}
	// Every word changed, or a side alone changing: none, or the side's.
	m = New("a\nfoo\nb\nc\n", "a\nbar\nb\nc\n", "a\nfoo\nb\nc!\n", false)
	if len(m.Chunks) != 2 {
		t.Fatalf("chunks %+v", m.Chunks)
	}
	if got := m.ChangedWords(0, Ours); got != nil {
		t.Errorf("all changed: %v", got)
	}
	if got := m.ChangedWords(0, Theirs); got != nil {
		t.Errorf("theirs unchanged: %v", got)
	}
	if got := m.ChangedWords(1, Theirs); !slices.Equal(got, []Range{{9, 10}}) {
		t.Errorf("theirs' second %v", got)
	}
}
