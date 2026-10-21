package conflict

import "strings"

// Range is the lines Start to End of a text, End not among them, from 0.
type Range struct{ Start, End int }

// Len returns how many lines the range holds.
func (r Range) Len() int { return r.End - r.Start }

// Hunk is a difference of two texts: a's lines A are b's lines B.
type Hunk struct{ A, B Range }

// Lines splits a text into its lines, each with its \n but the last's
// when the text doesn't end with one.
func Lines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.SplitAfter(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// maxEdits is how many lines Diff adds and removes at most before taking
// two texts as changed whole, which keeps it from taking time and memory
// squared in their size where they have little in common: 16 MB at most.
const maxEdits = 2000

// Diff returns the hunks that turn a into b, in order, as few lines
// changed as it finds (Myers'): the lines between them are alike.
func Diff(a, b []string) []Hunk {
	// What both start and end with is alike.
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	x, y := ids(a[pre:len(a)-suf], b[pre:len(b)-suf])
	var hunks []Hunk
	ax, by := pre, pre // the end of the last match
	for _, p := range matches(x, y) {
		if p[0]+pre > ax || p[1]+pre > by {
			hunks = append(hunks, Hunk{Range{ax, p[0] + pre}, Range{by, p[1] + pre}})
		}
		ax, by = p[0]+pre+1, p[1]+pre+1
	}
	if ea, eb := len(a)-suf, len(b)-suf; ea > ax || eb > by {
		hunks = append(hunks, Hunk{Range{ax, ea}, Range{by, eb}})
	}
	slide(a, b, hunks)
	return hunks
}

// slide moves the lines added or removed alone as far down as alike lines
// let them, as git does, so that two diffs from a text put a change of
// one of several alike lines, as the second of three, on the same line:
// the last.
func slide(a, b []string, hunks []Hunk) {
	for i := range hunks {
		h := &hunks[i]
		// The hunk keeps a line alike before the next.
		nextA, nextB := len(a)+1, len(b)+1
		if i+1 < len(hunks) {
			nextA, nextB = hunks[i+1].A.Start, hunks[i+1].B.Start
		}
		switch {
		case h.B.Len() == 0 && h.A.Len() > 0:
			for h.A.End+1 < nextA && h.A.End < len(a) && a[h.A.Start] == a[h.A.End] {
				h.A.Start, h.A.End, h.B.Start, h.B.End = h.A.Start+1, h.A.End+1, h.B.Start+1, h.B.End+1
			}
		case h.A.Len() == 0 && h.B.Len() > 0:
			for h.B.End+1 < nextB && h.B.End < len(b) && b[h.B.Start] == b[h.B.End] {
				h.A.Start, h.A.End, h.B.Start, h.B.End = h.A.Start+1, h.A.End+1, h.B.Start+1, h.B.End+1
			}
		}
	}
}

// ids numbers the lines of a and b, alike lines alike, to compare them
// as numbers.
func ids(a, b []string) (x, y []int) {
	seen := map[string]int{}
	id := func(lines []string) []int {
		out := make([]int, len(lines))
		for i, l := range lines {
			n, ok := seen[l]
			if !ok {
				n = len(seen)
				seen[l] = n
			}
			out[i] = n
		}
		return out
	}
	return id(a), id(b)
}

// matches returns the lines of a and b that a shortest edit keeps, as
// pairs of their indexes, in order.
func matches(a, b []int) [][2]int {
	n, m := len(a), len(b)
	if n == 0 || m == 0 {
		return nil
	}
	limit := min(n+m, maxEdits)
	off := limit + 1
	v := make([]int32, 2*limit+3)
	// trace[d] is how far each diagonal k, from -d to d, reached after d
	// edits.
	var trace [][]int32
	d, found := 0, false
	for ; d <= limit && !found; d++ {
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || k != d && v[off+k-1] < v[off+k+1] {
				x = int(v[off+k+1])
			} else {
				x = int(v[off+k-1]) + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x, y = x+1, y+1
			}
			v[off+k] = int32(x)
			if x >= n && y >= m {
				found = true
			}
		}
		trace = append(trace, append([]int32(nil), v[off-d:off+d+1]...))
	}
	if !found {
		return nil // too different: all of it changed
	}
	var out [][2]int
	x, y := n, m
	for d--; d > 0; d-- {
		prev := trace[d-1] // diagonals -(d-1) to d-1
		at := func(k int) int { return int(prev[k+d-1]) }
		k, pk := x-y, x-y-1
		if k == -d || k != d && at(k-1) < at(k+1) {
			pk = k + 1
		}
		px := at(pk)
		py := px - pk
		// The edit went down (a line of b's added) or right (a's removed),
		// then along the matches to x, y.
		sx, sy := px, py+1
		if pk == k-1 {
			sx, sy = px+1, py
		}
		for x > sx && y > sy {
			x, y = x-1, y-1
			out = append(out, [2]int{x, y})
		}
		x, y = px, py
	}
	for x > 0 && y > 0 {
		x, y = x-1, y-1
		out = append(out, [2]int{x, y})
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}
