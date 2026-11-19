package git

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// A branch not checked out fast-forwards to its upstream, and says why
// when it can't (#45).
func TestUpdateBranch(t *testing.T) {
	ctx := context.Background()
	src := repo(t)
	commit := func(r *Repo, name string) {
		t.Helper()
		write(t, r, name, name)
		if err := r.Stage(ctx, name); err != nil {
			t.Fatal(err)
		}
		if err := r.Commit(ctx, name, false); err != nil {
			t.Fatal(err)
		}
	}
	commit(src, "a.txt")
	if err := src.CreateBranch(ctx, "feat", "main", false); err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	if err := Clone(ctx, parent, src.Dir, "copy", nil); err != nil {
		t.Fatal(err)
	}
	r, err := Open(filepath.Join(parent, "copy"))
	if err != nil {
		t.Fatal(err)
	}
	for _, kv := range [][2]string{{"user.name", "Ada"}, {"user.email", "ada@example.com"}, {"commit.gpgsign", "false"}} {
		if _, err := r.Git(ctx, "config", kv[0], kv[1]); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.CheckoutRemote(ctx, "origin/feat"); err != nil {
		t.Fatal(err)
	}
	if err := r.Switch(ctx, "main"); err != nil {
		t.Fatal(err)
	}

	if _, err := src.Git(ctx, "switch", "feat"); err != nil {
		t.Fatal(err)
	}
	commit(src, "b.txt")
	want, _ := src.BranchHash(ctx, "feat")
	if err := r.UpdateBranch(ctx, "feat"); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.BranchHash(ctx, "feat"); got != want {
		t.Fatalf("feat at %s, want %s", got, want)
	}
	if st, _ := r.Status(ctx); st.Branch != "main" {
		t.Fatalf("switched to %q", st.Branch)
	}

	// Diverged: a commit of its own here, another there.
	if err := r.Switch(ctx, "feat"); err != nil {
		t.Fatal(err)
	}
	commit(r, "mine.txt")
	if err := r.Switch(ctx, "main"); err != nil {
		t.Fatal(err)
	}
	commit(src, "theirs.txt")
	if err := r.UpdateBranch(ctx, "feat"); err == nil || !strings.Contains(err.Error(), "feat") {
		t.Fatalf("diverged: %v", err)
	}

	if err := r.CreateBranch(ctx, "solo", "main", false); err != nil {
		t.Fatal(err)
	}
	if err := r.UpdateBranch(ctx, "solo"); err == nil {
		t.Fatal("no upstream, no error")
	}
}
