package conflict

import "testing"

const merged = `package main
<<<<<<< HEAD
mine
||||||| base
old
=======
theirs
more
>>>>>>> other
middle
<<<<<<< HEAD
=======
added
>>>>>>> other
end
`

func TestParse(t *testing.T) {
	blocks := Parse(merged)
	if len(blocks) != 2 {
		t.Fatalf("%d blocks", len(blocks))
	}
	b := blocks[0]
	if b.Ours != "mine\n" || b.Base != "old\n" || b.Theirs != "theirs\nmore\n" || b.FirstLine != 1 || b.LastLine != 8 {
		t.Fatalf("first %+v", b)
	}
	if got := merged[b.Start:b.End]; got[:7] != "<<<<<<<" || got[len(got)-14:] != ">>>>>>> other\n" {
		t.Fatalf("first spans %q", got)
	}
	if b := blocks[1]; b.Ours != "" || b.Theirs != "added\n" {
		t.Fatalf("second %+v", b)
	}
}

func TestResolve(t *testing.T) {
	text := merged
	// Resolve from the last, so the earlier offsets hold.
	blocks := Parse(text)
	text = text[:blocks[1].Start] + blocks[1].Text(Theirs) + text[blocks[1].End:]
	text = text[:blocks[0].Start] + blocks[0].Text(Both) + text[blocks[0].End:]
	want := "package main\nmine\ntheirs\nmore\nmiddle\nadded\nend\n"
	if text != want {
		t.Fatalf("got\n%s\nwant\n%s", text, want)
	}
	if len(Parse(text)) != 0 {
		t.Fatal("conflicts left")
	}
}

func TestUnfinished(t *testing.T) {
	if blocks := Parse("<<<<<<< HEAD\nmine\n=======\ntheirs\n"); len(blocks) != 0 {
		t.Fatalf("a conflict without its end: %+v", blocks)
	}
}
