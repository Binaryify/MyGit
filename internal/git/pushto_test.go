package git

import (
	"context"
	"testing"
)

// TestPushTo checks that a push to another remote than the upstream's
// sends the branch under its name there and leaves the upstream as it is,
// and that the commits it would send are those that remote hasn't (#56).
func TestPushTo(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	write(t, r, "a.txt", "one\n")
	_ = r.Stage(ctx, "a.txt")
	_ = r.Commit(ctx, "first", false)
	for _, name := range []string{"origin", "mirror"} {
		dir := t.TempDir()
		if _, err := git(ctx, Local, dir, "", nil, "init", "-q", "--bare"); err != nil {
			t.Fatal(err)
		}
		if _, err := r.Git(ctx, "remote", "add", name, dir); err != nil {
			t.Fatal(err)
		}
	}
	st, _ := r.Status(ctx)
	if err := r.Push(ctx, st, false); err != nil {
		t.Fatal(err)
	}
	write(t, r, "a.txt", "two\n")
	_ = r.Stage(ctx, "a.txt")
	_ = r.Commit(ctx, "second", false)
	st, _ = r.Status(ctx)

	// Not on the mirror at all: both commits; on origin: the one after.
	if out, err := r.OutgoingTo(ctx, "mirror", st.Branch); err != nil || len(out) != 2 {
		t.Fatalf("to the mirror before pushing %d, %v", len(out), err)
	}
	if out, err := r.OutgoingTo(ctx, "origin", st.Branch); err != nil || len(out) != 1 || out[0].Subject != "second" {
		t.Fatalf("to origin %+v, %v", out, err)
	}
	if err := r.PushTo(ctx, st, "mirror", false); err != nil {
		t.Fatal(err)
	}
	if out, err := r.OutgoingTo(ctx, "mirror", st.Branch); err != nil || len(out) != 0 {
		t.Fatalf("to the mirror after pushing %d, %v", len(out), err)
	}
	st, _ = r.Status(ctx)
	if st.Upstream != "origin/"+st.Branch || st.Ahead != 1 {
		t.Fatalf("upstream %q, ahead %d", st.Upstream, st.Ahead)
	}
}
