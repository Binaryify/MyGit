// Package conflict reads the conflicts git writes into a file it couldn't
// merge, between <<<<<<<, ||||||| (with diff3), ======= and >>>>>>>
// lines, and resolves them one at a time.
package conflict

import "strings"

// Block is a conflict: the text from Start to End, its markers included,
// and each side's lines in it.
type Block struct {
	Start, End          int // byte offsets in the text
	FirstLine, LastLine int // the lines of its markers, from 0
	Ours, Base, Theirs  string
}

// Choice is how a conflict is resolved.
type Choice int

const (
	Ours   Choice = iota // our lines
	Theirs               // their lines
	Both                 // ours, then theirs
)

// Parse finds the conflicts of a text, in order. A conflict missing a
// marker, as one the user is editing by hand, is not one.
func Parse(text string) []Block {
	var blocks []Block
	var b Block
	var ours, base, theirs strings.Builder
	state := 0 // 0 outside, 1 ours, 2 base, 3 theirs
	offset := 0
	for line, l := range strings.SplitAfter(text, "\n") {
		next := offset + len(l)
		switch {
		case strings.HasPrefix(l, "<<<<<<<"):
			b = Block{Start: offset, FirstLine: line}
			ours.Reset()
			base.Reset()
			theirs.Reset()
			state = 1
		case state == 1 && strings.HasPrefix(l, "|||||||"):
			state = 2
		case (state == 1 || state == 2) && strings.HasPrefix(l, "======="):
			state = 3
		case state == 3 && strings.HasPrefix(l, ">>>>>>>"):
			b.End, b.LastLine = next, line
			b.Ours, b.Base, b.Theirs = ours.String(), base.String(), theirs.String()
			blocks = append(blocks, b)
			state = 0
		case state == 1:
			ours.WriteString(l)
		case state == 2:
			base.WriteString(l)
		case state == 3:
			theirs.WriteString(l)
		}
		offset = next
	}
	return blocks
}

// Text is what a conflict resolves to.
func (b Block) Text(c Choice) string {
	switch c {
	case Ours:
		return b.Ours
	case Theirs:
		return b.Theirs
	}
	ours := b.Ours
	if ours != "" && !strings.HasSuffix(ours, "\n") {
		ours += "\n"
	}
	return ours + b.Theirs
}
