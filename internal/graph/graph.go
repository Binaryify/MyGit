// Package graph lays out the history's graph: which column each commit
// sits in and the lines that join commits to their parents, one row per
// commit, as the history lists them (children before parents).
//
// It keeps a lane for each line waiting for a commit, as IntelliJ's and
// gitk's graphs do. A commit takes the lane waiting for it, the leftmost
// when several are (branches merging back), or the first free one; its
// first parent continues in that lane and its other parents take lanes of
// their own. Lanes stay where they are while they run, so lines run
// straight down and only bend where they fork or join.
package graph

// Point is where a line ends in a row: its lane and its height in the
// row, Top, Mid (the commit's dot) or Bottom.
type Point struct {
	Lane int
	Y    Y
}

type Y uint8

const (
	Top Y = iota
	Mid
	Bottom
)

// Line joins two points of a row, in the color of its lane.
type Line struct {
	From, To Point
	Color    int
}

// Row is a commit's row of the graph.
type Row struct {
	Lane  int // the commit's dot
	Color int
	Lines []Line
	Width int // lanes the row uses, for its width
}

// Layout lays out rows one commit after another, so that a history can be
// laid out as it is read.
type Layout struct {
	lanes  []lane
	colors int // colors handed out, the next lane's
}

type lane struct {
	want  string // the hash the lane runs to, "" for a free lane
	color int
}

// Add lays out the next commit's row.
func (l *Layout) Add(hash string, parents []string) Row {
	var row Row
	// The lanes coming in that wait for this commit join at its dot; the
	// leftmost of them becomes its lane.
	at := -1
	for i, ln := range l.lanes {
		if ln.want != hash {
			continue
		}
		if at < 0 {
			at = i
		}
		row.Lines = append(row.Lines, Line{Point{i, Top}, Point{at, Mid}, ln.color})
	}
	if at < 0 { // a branch's tip: nothing ran to it
		at = l.free()
		l.lanes[at] = lane{color: l.color()}
	}
	row.Lane, row.Color = at, l.lanes[at].color
	for i := range l.lanes {
		if l.lanes[i].want == hash {
			l.lanes[i].want = ""
		}
	}
	// The lanes that pass by run straight down.
	for i, ln := range l.lanes {
		if ln.want != "" {
			row.Lines = append(row.Lines, Line{Point{i, Top}, Point{i, Bottom}, ln.color})
		}
	}
	// The parents leave the dot: the first in this lane, the others in a
	// lane already running to them, or a new one.
	for k, p := range parents {
		to := -1
		if k == 0 {
			to = at
			l.lanes[at].want = p
		} else {
			for i, ln := range l.lanes {
				if ln.want == p {
					to = i
					break
				}
			}
			if to < 0 {
				to = l.free()
				l.lanes[to] = lane{want: p, color: l.color()}
			}
		}
		row.Lines = append(row.Lines, Line{Point{at, Mid}, Point{to, Bottom}, l.lanes[to].color})
	}
	if len(parents) == 0 {
		l.lanes[at].want = ""
	}
	row.Width = len(l.lanes)
	for len(l.lanes) > 0 && l.lanes[len(l.lanes)-1].want == "" {
		l.lanes = l.lanes[:len(l.lanes)-1] // free lanes on the right go
	}
	return row
}

// free returns the leftmost free lane, adding one if every lane is taken.
func (l *Layout) free() int {
	for i, ln := range l.lanes {
		if ln.want == "" {
			return i
		}
	}
	l.lanes = append(l.lanes, lane{})
	return len(l.lanes) - 1
}

func (l *Layout) color() int {
	l.colors++
	return l.colors - 1
}
