package conflict

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ResolveSimple resolves conflict i when the sides changed different
// words of its lines, as IntelliJ's magic wand does: the result takes
// both their changes. It reports whether it did.
func (m *Merge) ResolveSimple(i int) (Edit, bool) {
	c := &m.Chunks[i]
	if !c.Conflict || c.State != [2]State{Pending, Pending} {
		return Edit{}, false
	}
	words := func(lines []string) []string { return Words(strings.Join(lines, "")) }
	merged, ok := merge3(words(m.base[c.Base.Start:c.Base.End]), words(m.lines(Ours, c)), words(m.lines(Theirs, c)))
	if !ok {
		return Edit{}, false
	}
	c.State = [2]State{Applied, Applied}
	return m.replace(i, Lines(strings.Join(merged, ""))), true
}

// merge3 merges the changes of ours and theirs to base, and reports
// whether none of them conflict.
func merge3(base, ours, theirs []string) ([]string, bool) {
	var out []string
	at := 0
	for _, c := range diff3(base, Diff(base, ours), Diff(base, theirs)) {
		out = append(out, base[at:c.Base.Start]...)
		o, t := ours[c.Sides[Ours].Start:c.Sides[Ours].End], theirs[c.Sides[Theirs].Start:c.Sides[Theirs].End]
		switch {
		case c.State == [2]State{Pending, Pending} && !slices.Equal(o, t):
			return nil, false
		case c.State[Ours] == Pending:
			out = append(out, o...)
		default:
			out = append(out, t...)
		}
		at = c.Base.End
	}
	return append(out, base[at:]...), true
}

// Words splits a text into words, runs of spaces, line ends, and the
// other characters one by one.
func Words(text string) []string {
	var out []string
	for len(text) > 0 {
		r, n := utf8.DecodeRuneInString(text)
		class := runeClass(r)
		if class != 0 {
			for n < len(text) {
				r, size := utf8.DecodeRuneInString(text[n:])
				if runeClass(r) != class {
					break
				}
				n += size
			}
		}
		out = append(out, text[:n])
		text = text[n:]
	}
	return out
}

// runeClass is 1 for the runes of words, 2 for spaces but line ends, and
// 0 for the others, each a word of its own.
func runeClass(r rune) int {
	switch {
	case r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
		return 1
	case r != '\n' && r != '\r' && unicode.IsSpace(r):
		return 2
	}
	return 0
}

// ChangedWords are the words side s changed of chunk i's base lines, as
// IntelliJ tints them within the lines: ranges of the bytes of the side's
// text. A side that left the chunk alone, or added or deleted whole lines
// there, or changed every word of them, has none, as its lines say it.
func (m *Merge) ChangedWords(i int, s Side) []Range {
	c := &m.Chunks[i]
	side, base := m.lines(s, c), m.base[c.Base.Start:c.Base.End]
	if c.State[s] == Unchanged || len(side) == 0 || len(base) == 0 {
		return nil
	}
	words := Words(strings.Join(side, ""))
	at := 0
	for _, l := range m.sides[s][:c.Sides[s].Start] {
		at += len(l)
	}
	offs := make([]int, len(words)+1) // the bytes of the side's text each word starts at
	for j, w := range words {
		offs[j], at = at, at+len(w)
	}
	offs[len(words)] = at
	solid := func(ws []string) (n int) { // the words but spaces and line ends
		for _, w := range ws {
			if strings.TrimSpace(w) != "" {
				n++
			}
		}
		return n
	}
	var out []Range
	changed := 0
	for _, h := range Diff(Words(strings.Join(base, "")), words) {
		if h.B.Len() > 0 {
			out = append(out, Range{offs[h.B.Start], offs[h.B.End]})
			changed += solid(words[h.B.Start:h.B.End])
		}
	}
	if changed == solid(words) {
		return nil
	}
	return out
}
