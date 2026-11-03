package main

import (
	"slices"
	"time"

	"github.com/egoist/mygo/ui/editor"
)

// patchLanguage is l for the lines of a patch, which start at starts: its
// tokenizer highlights each hunk from the start, as hunkHighlighter does.
// A language highlighting in its own way stays as it is.
func patchLanguage(l *editor.Language, starts []int) *editor.Language {
	if l == nil || l.Tokenizer == nil || l.Highlighter != nil || len(starts) < 2 {
		return l
	}
	p := *l
	p.Highlighter = func(text editor.Snapshot, _ func()) editor.Highlighter {
		return &hunkHighlighter{t: l.Tokenizer, text: text, starts: starts}
	}
	return &p
}

// hunkStarts returns the lines of a patch's sides that its hunks start
// at, as the diff of the patch lays them one after another.
func hunkStarts(p editor.FilePatch) (old, new []int) {
	o, n := 0, 0
	for _, h := range p.Hunks {
		old, new = append(old, o), append(new, n)
		for _, l := range h.Lines {
			if l == "" || l[0] == '\\' {
				continue
			}
			if l[0] != '+' {
				o++
			}
			if l[0] != '-' {
				n++
			}
		}
	}
	return old, new
}

// hunkHighlighter highlights a patch's lines as mygo's default
// highlighter does, a line at a time from the state the line before ends
// in, but each hunk from the start: the file's lines between two hunks
// are not there, so one hunk ending in a block comment would leave the
// next all comment.
type hunkHighlighter struct {
	t       editor.Tokenizer
	text    editor.Snapshot
	starts  []int    // the lines the hunks start at, in order
	states  []uint64 // the state line i starts in, for the lines known
	scratch []editor.Token
}

func (h *hunkHighlighter) Edit(_, new editor.Snapshot, changes []editor.Change) {
	h.text = new
	known := len(h.states)
	for _, c := range changes {
		known = min(known, new.LineOf(c.NewStart)+1)
	}
	h.states = h.states[:known]
}

func (h *hunkHighlighter) Highlight(last int, budget time.Duration) bool {
	last = min(last, h.text.LineCount()-1)
	if len(h.states) == 0 {
		h.states = append(h.states, 0)
	}
	start := time.Now()
	for i := len(h.states) - 1; i < last; i++ {
		if i&63 == 63 && time.Since(start) > budget {
			return false
		}
		var end uint64
		h.scratch, end = h.t.Tokenize(h.text.Line(i), h.states[i], h.scratch[:0])
		if _, ok := slices.BinarySearch(h.starts, i+1); ok {
			end = 0
		}
		h.states = append(h.states, end)
	}
	return true
}

func (h *hunkHighlighter) Tokens(i int, text string, dst []editor.Token) ([]editor.Token, bool) {
	state, ok := uint64(0), i < len(h.states)
	if ok {
		state = h.states[i]
	}
	if len(text) > 10000 { // left plain, as mygo's highlighter leaves it
		return dst, ok
	}
	dst, _ = h.t.Tokenize(text, state, dst)
	return dst, ok
}
