package git

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// repo makes a repository in a temporary folder.
func repo(t *testing.T) *Repo {
	t.Helper()
	dir := t.TempDir()
	if _, err := run(context.Background(), dir, "init", "-q", "-b", "main"); err != nil {
		t.Skip("git: ", err)
	}
	for _, kv := range [][2]string{{"user.name", "Ada"}, {"user.email", "ada@example.com"}, {"commit.gpgsign", "false"}} {
		if _, err := run(context.Background(), dir, "config", kv[0], kv[1]); err != nil {
			t.Fatal(err)
		}
	}
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func write(t *testing.T, r *Repo, name, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(r.Dir, name), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEmptyRepository(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	n := 0
	if err := r.Log(ctx, 10, func(c []Commit) { n += len(c) }); err != nil || n != 0 {
		t.Fatalf("log of an empty repository: %d commits, %v", n, err)
	}
	write(t, r, "a.txt", "a\n")
	st, err := r.Status(ctx)
	if err != nil || len(st.Changes) != 1 || st.Changes[0].Work != '?' || st.Branch != "main" {
		t.Fatalf("status %+v, %v", st, err)
	}
	if err := r.Stage(ctx, "a.txt"); err != nil {
		t.Fatal(err)
	}
	if err := r.Unstage(ctx, "a.txt"); err != nil { // no HEAD yet
		t.Fatal(err)
	}
	if st, _ := r.Status(ctx); st.Changes[0].Work != '?' {
		t.Fatalf("after unstaging: %+v", st.Changes)
	}
}

func TestStageCommitLog(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	write(t, r, "a.txt", "one\n")
	if err := r.Stage(ctx, "a.txt"); err != nil {
		t.Fatal(err)
	}
	if err := r.Commit(ctx, "first", false); err != nil {
		t.Fatal(err)
	}
	write(t, r, "a.txt", "one\ntwo\n")
	st, _ := r.Status(ctx)
	if len(st.Changes) != 1 || st.Changes[0].Work != 'M' || st.Changes[0].Staged != 0 {
		t.Fatalf("modified file: %+v", st.Changes)
	}
	if d, err := r.Diff(ctx, "a.txt", false); err != nil || !slices.Contains(lines(d), "+two") {
		t.Fatalf("diff %q, %v", d, err)
	}
	if err := r.Stage(ctx, "a.txt"); err != nil {
		t.Fatal(err)
	}
	if st, _ := r.Status(ctx); st.Changes[0].Staged != 'M' || st.Changes[0].Work != 0 {
		t.Fatalf("staged file: %+v", st.Changes)
	}
	if err := r.Commit(ctx, "second", false); err != nil {
		t.Fatal(err)
	}
	var all []Commit
	if err := r.Log(ctx, 1, func(c []Commit) { all = append(all, c...) }); err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].Subject != "second" || all[1].Subject != "first" ||
		len(all[0].Parents) != 1 || all[0].Parents[0] != all[1].Hash || !slices.Contains(all[0].Refs, "main") {
		t.Fatalf("log %+v", all)
	}
	if p, err := r.Show(ctx, all[0].Hash); err != nil || !slices.Contains(lines(p), "+two") {
		t.Fatalf("show %q, %v", p, err)
	}
	if r.LastMessage(ctx) != "second" {
		t.Fatal("last message", r.LastMessage(ctx))
	}
}

func TestParseStatusRename(t *testing.T) {
	out := "# branch.oid abc\x00# branch.head dev\x00# branch.upstream origin/dev\x00# branch.ab +2 -1\x00" +
		"2 R. N... 100644 100644 100644 aaa bbb R100 new name.go\x00old.go\x00" +
		"u UU N... 100644 100644 100644 100644 a b c both.go\x00? new.txt\x00"
	s := parseStatus(out)
	if s.Branch != "dev" || s.Upstream != "origin/dev" || s.Ahead != 2 || s.Behind != 1 {
		t.Fatalf("branch %+v", s)
	}
	want := []Change{
		{Path: "new name.go", OldPath: "old.go", Staged: 'R'},
		{Path: "both.go", Work: 'U', Conflict: true},
		{Path: "new.txt", Work: '?'},
	}
	if !slices.Equal(s.Changes, want) {
		t.Fatalf("changes %+v", s.Changes)
	}
}

func lines(s string) []string { return strings.Split(s, "\n") }
