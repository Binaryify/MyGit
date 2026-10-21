package conflict

import (
	"slices"
	"strings"
)

// Side is one of the two versions merged.
type Side int

const (
	Ours Side = iota
	Theirs
)

// State is what became of a side's change to a chunk.
type State uint8

const (
	Unchanged State = iota // the side left the chunk as the base has it
	Pending                // not taken into the result nor dismissed yet
	Applied                // taken into the result
	Ignored                // dismissed, or the chunk was edited by hand
)

// Chunk is a part of the file one side or both changed: the base's lines
// Base, each side's lines Sides, and the result's Result.
type Chunk struct {
	Base   Range
	Sides  [2]Range
	Result Range
	State  [2]State
	// Conflict says both sides changed the chunk, and differently.
	Conflict bool
}

// Resolved reports whether nothing is left to choose in the chunk.
func (c *Chunk) Resolved() bool { return c.State[Ours] != Pending && c.State[Theirs] != Pending }

// Merge merges two versions of a text with their common ancestor, the
// base, as IntelliJ's merge window does: the result starts as the base
// with the changes of one side alone taken in, and leaves the conflicts,
// the chunks both changed, as the base has them, to take a side's change
// into or dismiss it, a chunk at a time, or to edit by hand.
type Merge struct {
	base   []string
	sides  [2][]string
	result []string
	Chunks []Chunk
}

// Edit replaces the bytes Start to End of the result with Text.
type Edit struct {
	Start, End int
	Text       string
}

// New merges ours and theirs, with base their common ancestor, "" when
// they have none. With ignoreSpace, lines alike but for their spaces are
// alike, and keep our spaces, as git's ignore-space-change has them.
func New(base, ours, theirs string, ignoreSpace bool) *Merge {
	m := &Merge{base: Lines(base), sides: [2][]string{Lines(ours), Lines(theirs)}}
	key := func(lines []string) []string { return lines }
	if ignoreSpace {
		key = func(lines []string) []string {
			out := make([]string, len(lines))
			for i, l := range lines {
				out[i] = strings.Join(strings.Fields(l), " ")
			}
			return out
		}
	}
	kb, ko, kt := key(m.base), key(m.sides[Ours]), key(m.sides[Theirs])
	m.Chunks = diff3(kb, Diff(kb, ko), Diff(kb, kt))
	at := 0                                 // the base's line the result got to
	alike := func(base, end int) []string { // our lines alike the base's at to base, ending at ours' end
		return m.sides[Ours][end-(base-at) : end]
	}
	for i := range m.Chunks {
		c := &m.Chunks[i]
		m.result = append(m.result, alike(c.Base.Start, c.Sides[Ours].Start)...)
		start := len(m.result)
		o, t := c.Sides[Ours], c.Sides[Theirs]
		switch {
		case c.State == [2]State{Pending, Pending} && !slices.Equal(ko[o.Start:o.End], kt[t.Start:t.End]):
			c.Conflict = true
			m.result = append(m.result, m.base[c.Base.Start:c.Base.End]...)
		case c.State[Ours] == Pending: // theirs too when alike
			m.result = append(m.result, m.lines(Ours, c)...)
		default:
			m.result = append(m.result, m.lines(Theirs, c)...)
		}
		if !c.Conflict {
			for s := range c.State {
				if c.State[s] == Pending {
					c.State[s] = Applied
				}
			}
		}
		c.Result = Range{start, len(m.result)}
		at = c.Base.End
	}
	m.result = append(m.result, alike(len(m.base), len(m.sides[Ours]))...)
	return m
}

// diff3 groups the changes of the two sides to the base into chunks: the
// changes that overlap or touch, as git takes them, are one, a conflict
// when the sides changed it differently.
func diff3(base []string, ours, theirs []Hunk) []Chunk {
	var chunks []Chunk
	hunks := [2][]Hunk{ours, theirs}
	var delta [2]int // what each side's hunks so far added
	for len(hunks[Ours]) > 0 || len(hunks[Theirs]) > 0 {
		// The group starts with the hunk first in the base, and takes
		// those of either side that reach into it.
		first := Theirs
		if len(hunks[Ours]) > 0 && (len(hunks[Theirs]) == 0 || hunks[Ours][0].A.Start <= hunks[Theirs][0].A.Start) {
			first = Ours
		}
		lo, hi := hunks[first][0].A.Start, hunks[first][0].A.End
		var group [2][]Hunk
		for grew := true; grew; {
			grew = false
			for s := range 2 {
				// A side's own hunks have a line alike between them, so
				// one touching the group touches the other side's.
				for len(hunks[s]) > 0 && hunks[s][0].A.Start <= hi {
					h := hunks[s][0]
					group[s] = append(group[s], h)
					hunks[s] = hunks[s][1:]
					hi = max(hi, h.A.End)
					grew = true
				}
			}
		}
		c := Chunk{Base: Range{lo, hi}}
		for s := range 2 {
			g := group[s]
			if len(g) == 0 {
				c.Sides[s] = Range{lo + delta[s], hi + delta[s]}
				continue
			}
			c.Sides[s] = Range{g[0].B.Start - (g[0].A.Start - lo), g[len(g)-1].B.End + (hi - g[len(g)-1].A.End)}
			for _, h := range g {
				delta[s] += h.B.Len() - h.A.Len()
			}
			c.State[s] = Pending
		}
		chunks = append(chunks, c)
	}
	return chunks
}

// lines returns a side's lines of a chunk.
func (m *Merge) lines(s Side, c *Chunk) []string { return m.sides[s][c.Sides[s].Start:c.Sides[s].End] }

// Result returns the merged text.
func (m *Merge) Result() string { return strings.Join(m.result, "") }

// Unresolved returns how many chunks have changes left to choose.
func (m *Merge) Unresolved() int {
	n := 0
	for i := range m.Chunks {
		if !m.Chunks[i].Resolved() {
			n++
		}
	}
	return n
}

// Apply takes a side's change to chunk i into the result: in place of
// the base's lines, or after the other side's change when it was taken.
func (m *Merge) Apply(i int, s Side) Edit {
	c := &m.Chunks[i]
	if c.State[s] != Pending {
		return Edit{}
	}
	var lines []string
	if c.State[1-s] == Applied {
		lines = append(slices.Clone(m.result[c.Result.Start:c.Result.End]), m.lines(s, c)...)
	} else {
		lines = m.lines(s, c)
	}
	c.State[s] = Applied
	return m.replace(i, lines)
}

// Ignore dismisses a side's change to chunk i, leaving the result as it
// is.
func (m *Merge) Ignore(i int, s Side) {
	if c := &m.Chunks[i]; c.State[s] == Pending {
		c.State[s] = Ignored
	}
}

// Unignore takes back Ignore.
func (m *Merge) Unignore(i int, s Side) {
	if c := &m.Chunks[i]; c.State[s] == Ignored {
		c.State[s] = Pending
	}
}

// replace puts lines in place of chunk i's in the result, and moves the
// chunks after it.
func (m *Merge) replace(i int, lines []string) Edit {
	c := &m.Chunks[i]
	e := Edit{Start: m.offset(c.Result.Start), End: m.offset(c.Result.End), Text: strings.Join(lines, "")}
	m.result = slices.Concat(m.result[:c.Result.Start], lines, m.result[c.Result.End:])
	moved := len(lines) - c.Result.Len()
	c.Result.End = c.Result.Start + len(lines)
	for j := i + 1; j < len(m.Chunks); j++ {
		m.Chunks[j].Result.Start += moved
		m.Chunks[j].Result.End += moved
	}
	return e
}

// offset returns the byte the result's line starts at.
func (m *Merge) offset(line int) int {
	n := 0
	for _, l := range m.result[:line] {
		n += len(l)
	}
	return n
}

// Sync takes the result as it was edited by hand, moving the chunks with
// their lines. A chunk the edit changed is what its text now is: as the
// base has it, its changes are pending again, as a side's or both have
// it, they are taken, and otherwise it is resolved by hand.
func (m *Merge) Sync(text string) {
	lines := Lines(text)
	if slices.Equal(lines, m.result) {
		return
	}
	hunks := Diff(m.result, lines)
	for i := range m.Chunks {
		c := &m.Chunks[i]
		r, touched := moveRange(c.Result, hunks)
		c.Result = r
		if touched {
			defer m.derive(c) // once the result is the new one
		}
	}
	m.result = lines
}

// moveRange returns where the lines r are after the hunks, and whether
// the hunks changed them: within them, or lines added at their ends.
func moveRange(r Range, hunks []Hunk) (Range, bool) {
	out, touched := Range{mapLine(r.Start, hunks, false), mapLine(r.End, hunks, true)}, false
	for _, h := range hunks {
		if h.A.Start < r.End && h.A.End > r.Start || h.A.Len() == 0 && h.A.Start >= r.Start && h.A.Start <= r.End {
			touched = true
		}
	}
	return out, touched
}

// mapLine returns where the boundary before line p is after the hunks:
// after the lines added there when end, before them otherwise.
func mapLine(p int, hunks []Hunk, end bool) int {
	q := p
	for _, h := range hunks {
		switch {
		case h.A.Start < p && p < h.A.End: // within lines replaced
			if end {
				return h.B.End
			}
			return h.B.Start
		case h.A.End < p || h.A.End == p && (end || h.A.Len() > 0):
			q += h.B.Len() - h.A.Len()
		}
	}
	return q
}

// derive works out the states of a chunk edited by hand from its text.
func (m *Merge) derive(c *Chunk) {
	got := m.result[c.Result.Start:c.Result.End]
	base := m.base[c.Base.Start:c.Base.End]
	var changed [2]bool
	for s := range c.State {
		changed[s] = c.State[s] != Unchanged
	}
	set := func(o, t State) {
		if changed[Ours] {
			c.State[Ours] = o
		}
		if changed[Theirs] {
			c.State[Theirs] = t
		}
	}
	ours, theirs := m.lines(Ours, c), m.lines(Theirs, c)
	switch {
	case slices.Equal(got, base):
		set(Pending, Pending)
	case changed[Ours] && changed[Theirs] && (slices.Equal(got, slices.Concat(ours, theirs)) || slices.Equal(got, slices.Concat(theirs, ours))):
		set(Applied, Applied)
	case changed[Ours] && slices.Equal(got, ours):
		set(Applied, Ignored)
	case changed[Theirs] && slices.Equal(got, theirs):
		set(Ignored, Applied)
	default:
		set(Ignored, Ignored)
	}
}
