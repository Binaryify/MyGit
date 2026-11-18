package git

import (
	"context"
	"slices"
	"testing"
)

// Blobs reads many files' contents in one call of git, the index's among
// them, and nothing for those a revision has none of: the diff of a file
// reads its two sides that way rather than one call each. BlobSizes reads
// their sizes before them, so that what is worth reading can be chosen.
func TestBlobs(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	write(t, r, "a.txt", "one\n")
	write(t, r, "b.txt", "two\n")
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := r.Stage(ctx, name); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Commit(ctx, "first", false); err != nil {
		t.Fatal(err)
	}
	write(t, r, "a.txt", "one\ntwo\n")
	if err := r.Stage(ctx, "a.txt"); err != nil {
		t.Fatal(err)
	}
	var head string
	if err := r.Log(ctx, Scope{}, 1, func(c []Commit) { head = c[0].Hash }); err != nil {
		t.Fatal(err)
	}

	got, err := r.Blobs(ctx, []string{head + ":a.txt", ":a.txt", head + ":b.txt", head + ":none.txt", ":none.txt"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"one\n", "one\ntwo\n", "two\n", "", ""}
	if len(got) != len(want) {
		t.Fatalf("%d blobs, want %d", len(got), len(want))
	}
	for i, w := range want {
		if w == "" {
			if got[i] != nil {
				t.Errorf("blob %d is %q, want none", i, got[i])
			}
			continue
		}
		if string(got[i]) != w {
			t.Errorf("blob %d is %q, want %q", i, got[i], w)
		}
	}

	// Their sizes, read before them, say what is worth reading.
	n, err := r.BlobSizes(ctx, []string{head + ":a.txt", ":a.txt", head + ":none.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []int64{4, 8, -1}; !slices.Equal(n, want) {
		t.Errorf("sizes %v, want %v", n, want)
	}
}
