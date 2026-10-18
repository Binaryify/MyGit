package graph

import (
	"slices"
	"strconv"
	"testing"
)

type commit struct {
	hash    string
	parents []string
}

func layout(commits []commit) []Row {
	var l Layout
	rows := make([]Row, len(commits))
	for i, c := range commits {
		rows[i] = l.Add(c.hash, c.parents)
	}
	return rows
}

func lanes(rows []Row) []int {
	out := make([]int, len(rows))
	for i, r := range rows {
		out[i] = r.Lane
	}
	return out
}

func TestLinear(t *testing.T) {
	rows := layout([]commit{{"c", []string{"b"}}, {"b", []string{"a"}}, {"a", nil}})
	if got := lanes(rows); !slices.Equal(got, []int{0, 0, 0}) {
		t.Fatalf("lanes %v", got)
	}
	want := [][]Line{
		{{Point{0, Mid}, Point{0, Bottom}, 0}},
		{{Point{0, Top}, Point{0, Mid}, 0}, {Point{0, Mid}, Point{0, Bottom}, 0}},
		{{Point{0, Top}, Point{0, Mid}, 0}},
	}
	for i, r := range rows {
		if !slices.Equal(r.Lines, want[i]) || r.Width != 1 || r.Color != 0 {
			t.Errorf("row %d: %+v", i, r)
		}
	}
}

// m merges f into the line of b:
//
//	m        0
//	| \
//	f |      1
//	b |      0, f's line passing on the right
//	|/
//	a        0
func TestMerge(t *testing.T) {
	rows := layout([]commit{
		{"m", []string{"b", "f"}},
		{"f", []string{"a"}},
		{"b", []string{"a"}},
		{"a", nil},
	})
	if got := lanes(rows); !slices.Equal(got, []int{0, 1, 0, 0}) {
		t.Fatalf("lanes %v", got)
	}
	if w := rows[0].Width; w != 2 {
		t.Errorf("merge row is %d lanes wide", w)
	}
	if !slices.Contains(rows[0].Lines, Line{Point{0, Mid}, Point{1, Bottom}, 1}) {
		t.Errorf("merge row doesn't fork to f's lane: %+v", rows[0].Lines)
	}
	// f's lane passes by b, then joins a's dot.
	if !slices.Contains(rows[2].Lines, Line{Point{1, Top}, Point{1, Bottom}, 1}) {
		t.Errorf("f's lane doesn't pass b: %+v", rows[2].Lines)
	}
	if !slices.Contains(rows[3].Lines, Line{Point{1, Top}, Point{0, Mid}, 1}) {
		t.Errorf("f's lane doesn't join a: %+v", rows[3].Lines)
	}
	if rows[3].Width != 2 || rows[3].Color != 0 {
		t.Errorf("a: %+v", rows[3])
	}
}

// Two branches from one commit, their tips listed first: the second tip
// takes a lane of its own, which joins the first at their fork.
func TestTwoTips(t *testing.T) {
	rows := layout([]commit{
		{"x", []string{"a"}},
		{"y", []string{"a"}},
		{"a", nil},
	})
	if got := lanes(rows); !slices.Equal(got, []int{0, 1, 0}) {
		t.Fatalf("lanes %v", got)
	}
	if rows[0].Color == rows[1].Color {
		t.Error("two branches in one color")
	}
	if !slices.Contains(rows[2].Lines, Line{Point{1, Top}, Point{0, Mid}, rows[1].Color}) {
		t.Errorf("y's lane doesn't join a: %+v", rows[2].Lines)
	}
}

// A lane freed in the middle is taken by the next new line rather than a
// lane further right, so the graph stays narrow.
func TestReusesFreeLane(t *testing.T) {
	rows := layout([]commit{
		{"x", []string{"a"}},
		{"y", []string{"b"}},
		{"z", []string{"c"}},
		{"b", nil}, // y's lane, 1, ends here
		{"w", []string{"c"}},
	})
	if got := lanes(rows); !slices.Equal(got, []int{0, 1, 2, 1, 1}) {
		t.Fatalf("lanes %v", got)
	}
}

// Two roots: the history of an unrelated branch ends, and the next commit
// starts where it was.
func TestRoots(t *testing.T) {
	rows := layout([]commit{{"a", nil}, {"b", nil}})
	if got := lanes(rows); !slices.Equal(got, []int{0, 0}) {
		t.Fatalf("lanes %v", got)
	}
	if len(rows[0].Lines) != 0 || rows[0].Color == rows[1].Color {
		t.Errorf("roots %+v", rows)
	}
}

// A history of a main line with a branch merged into each commit.
func BenchmarkAdd(b *testing.B) {
	var commits []commit
	for i := range 50_000 {
		next := "m" + strconv.Itoa(i+1)
		commits = append(commits,
			commit{"m" + strconv.Itoa(i), []string{next, "f" + strconv.Itoa(i)}},
			commit{"f" + strconv.Itoa(i), []string{next}})
	}
	b.ResetTimer()
	for range b.N {
		layout(commits)
	}
}
