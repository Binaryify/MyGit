// Package patch cuts a file's patch down to some of its changed lines, so
// that git apply stages, unstages or discards those lines alone, as
// IntelliJ's and git gui's "stage selected lines" do.
package patch

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui/editor"
)

// Line is a changed line of a patch: the i-th of hunk h's lines.
type Line struct{ Hunk, Index int }

// Lines numbers a patch's lines as a diff of it shows them: Old has, for
// each line of the old side (context and deletions) in order, its place in
// the patch, and New the same for the new side (context and additions).
func Lines(p editor.FilePatch) (old, new []Line) {
	for h, hunk := range p.Hunks {
		for i, l := range hunk.Lines {
			if l == "" || l[0] == '\\' {
				continue
			}
			if l[0] != '+' {
				old = append(old, Line{h, i})
			}
			if l[0] != '-' {
				new = append(new, Line{h, i})
			}
		}
	}
	return old, new
}

// Select is a patch of the lines chosen of p's changes, for git apply.
//
// Forward, it applies to p's old side, as staging applies a working tree's
// changes to the index: the deletions not chosen stay as context and the
// additions not chosen go. Reverse, it applies backwards to p's new side,
// as unstaging takes changes out of the index and discarding out of the
// working tree: the additions not chosen stay and the deletions go.
//
// It returns "" when no change is chosen.
func Select(p editor.FilePatch, chosen map[Line]bool, reverse bool) string {
	var b strings.Builder
	any := false
	for h, hunk := range p.Hunks {
		var lines []string
		oldN, newN, changed := 0, 0, false
		for i, l := range hunk.Lines {
			if l == "" {
				continue
			}
			kind := l[0]
			if kind == '\\' {
				// "No newline at end of file" goes with the line before it,
				// if that one stayed.
				if len(lines) > 0 && lines[len(lines)-1] != "" {
					lines = append(lines, l)
				}
				continue
			}
			if (kind == '+' || kind == '-') && !chosen[Line{h, i}] {
				keep := '-'
				if reverse {
					keep = '+'
				}
				if rune(kind) != keep {
					lines = append(lines, "") // dropped: a "\" after it goes too
					continue
				}
				kind, l = ' ', " "+l[1:]
			}
			if kind == '+' || kind == '-' {
				changed = true
			}
			if kind != '+' {
				oldN++
			}
			if kind != '-' {
				newN++
			}
			lines = append(lines, l)
		}
		if !changed {
			continue
		}
		any = true
		// The old side starts where p's does; a forward patch's new side
		// starts there too, as the hunks before it applied, if any did,
		// git apply finds the place by the context.
		oldStart := hunk.OldStart
		if reverse {
			oldStart = hunk.NewStart
		}
		fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", start(oldStart, oldN), oldN, start(oldStart, newN), newN)
		for _, l := range lines {
			if l != "" {
				b.WriteString(l)
				b.WriteByte('\n')
			}
		}
	}
	if !any {
		return ""
	}
	return header(p) + b.String()
}

// start is a hunk's first line, 0 for a side with no lines, as diff
// writes it.
func start(at, n int) int {
	if n == 0 && at > 0 {
		return at - 1
	}
	return at
}

func header(p editor.FilePatch) string {
	return fmt.Sprintf("diff --git a/%[1]s b/%[1]s\n--- a/%[1]s\n+++ b/%[1]s\n", p.NewName)
}

// Partial tells whether lines of p can be chosen: a file changed in
// place, not one added, deleted, renamed or binary, whose patch git apply
// can't cut.
func Partial(p editor.FilePatch) bool {
	return !p.Binary && p.OldName != "" && p.OldName == p.NewName && len(p.Hunks) > 0
}

// FileLines numbers a patch's lines by the lines of the files, from 0:
// old has the old file's lines the patch holds (context and deletions),
// new the new file's (context and additions), as a diff of the whole
// files numbers them.
func FileLines(p editor.FilePatch) (old, new map[int]Line) {
	old, new = map[int]Line{}, map[int]Line{}
	for h, hunk := range p.Hunks {
		o, n := hunk.OldStart-1, hunk.NewStart-1
		for i, l := range hunk.Lines {
			if l == "" || l[0] == '\\' {
				continue
			}
			if l[0] != '+' {
				old[o] = Line{h, i}
				o++
			}
			if l[0] != '-' {
				new[n] = Line{h, i}
				n++
			}
		}
	}
	return old, new
}
