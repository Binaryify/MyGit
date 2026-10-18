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
	if err := r.Log(ctx, Scope{}, 10, func(c []Commit) { n += len(c) }); err != nil || n != 0 {
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
	if d, err := r.Diff(ctx, "a.txt", false, false); err != nil || !slices.Contains(lines(d), "+two") {
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
	if err := r.Log(ctx, Scope{}, 1, func(c []Commit) { all = append(all, c...) }); err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].Subject != "second" || all[1].Subject != "first" ||
		len(all[0].Parents) != 1 || all[0].Parents[0] != all[1].Hash || !slices.Contains(all[0].Refs, "main") {
		t.Fatalf("log %+v", all)
	}
	if p, err := r.Show(ctx, all[0].Hash, false); err != nil || !slices.Contains(lines(p), "+two") {
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

func TestLogScope(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	commit := func(name, text, msg string) {
		write(t, r, name, text)
		if err := r.Stage(ctx, name); err != nil {
			t.Fatal(err)
		}
		if err := r.Commit(ctx, msg, false); err != nil {
			t.Fatal(err)
		}
	}
	commit("a.txt", "1", "a1")
	commit("b.txt", "1", "b1")
	commit("a.txt", "2", "a2")
	var got []Commit
	if err := r.Log(ctx, Scope{Path: "a.txt"}, 10, func(c []Commit) { got = append(got, c...) }); err != nil {
		t.Fatal(err)
	}
	// b1 is left out, and a2's parent becomes a1.
	if len(got) != 2 || got[0].Subject != "a2" || got[1].Subject != "a1" || got[0].Parents[0] != got[1].Hash {
		t.Fatalf("log of a.txt %+v", got)
	}
}

func TestBranchesAndCommits(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	commit := func(name, msg string) {
		write(t, r, name, msg)
		if err := r.Stage(ctx, name); err != nil {
			t.Fatal(err)
		}
		if err := r.Commit(ctx, msg, false); err != nil {
			t.Fatal(err)
		}
	}
	commit("a.txt", "a")
	if err := r.CreateBranch(ctx, "topic", "", true); err != nil {
		t.Fatal(err)
	}
	commit("b.txt", "b")
	if err := r.Switch(ctx, "main"); err != nil {
		t.Fatal(err)
	}
	if err := r.DeleteBranch(ctx, "topic", false); !NotMerged(err) {
		t.Fatalf("deleting an unmerged branch: %v", err)
	}
	if err := r.Merge(ctx, "topic"); err != nil {
		t.Fatal(err)
	}
	if err := r.RenameBranch(ctx, "topic", "done"); err != nil {
		t.Fatal(err)
	}
	if b, _ := r.Branches(ctx); !slices.Equal(b, []string{"done", "main"}) {
		t.Fatalf("branches %v", b)
	}
	if err := r.DeleteBranch(ctx, "done", false); err != nil {
		t.Fatal(err)
	}
	var log []Commit
	_ = r.Log(ctx, Scope{}, 10, func(c []Commit) { log = append(log, c...) })
	if err := r.Revert(ctx, log[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(r.Dir, "b.txt")); !os.IsNotExist(err) {
		t.Fatalf("b.txt after reverting its commit: %v", err)
	}
	if err := r.Reset(ctx, log[0].Hash, ResetHard); err != nil {
		t.Fatal(err)
	}
	if err := r.Tag(ctx, "v1", log[0].Hash, "first"); err != nil {
		t.Fatal(err)
	}
	if !r.ValidBranchName(ctx, "feature/x") || r.ValidBranchName(ctx, "bad..name") {
		t.Fatal("branch names")
	}
}

func TestStashAndOperations(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	commit := func(name, text, msg string) {
		write(t, r, name, text)
		if err := r.Stage(ctx, name); err != nil {
			t.Fatal(err)
		}
		if err := r.Commit(ctx, msg, false); err != nil {
			t.Fatal(err)
		}
	}
	commit("a.txt", "base\n", "base")
	write(t, r, "a.txt", "wip\n")
	write(t, r, "new.txt", "new\n")
	if err := r.StashPush(ctx, "half done", true); err != nil {
		t.Fatal(err)
	}
	list, err := r.Stashes(ctx)
	if err != nil || len(list) != 1 || list[0].Ref != "stash@{0}" || !strings.HasSuffix(list[0].Subject, "half done") {
		t.Fatalf("stashes %+v, %v", list, err)
	}
	if p, err := r.StashShow(ctx, "stash@{0}"); err != nil || !strings.Contains(p, "+wip") || !strings.Contains(p, "+new") {
		t.Fatalf("stash patch %q, %v", p, err)
	}
	if err := r.StashApply(ctx, "stash@{0}", true); err != nil {
		t.Fatal(err)
	}
	if list, _ := r.Stashes(ctx); len(list) != 0 {
		t.Fatalf("stash after pop %+v", list)
	}
	if err := r.Discard(ctx, Change{Path: "a.txt", Work: 'M'}); err != nil {
		t.Fatal(err)
	}
	if err := r.Discard(ctx, Change{Path: "new.txt", Work: '?'}); err != nil {
		t.Fatal(err)
	}

	// A conflict stops a merge, which aborts back to where it was.
	if err := r.CreateBranch(ctx, "other", "", true); err != nil {
		t.Fatal(err)
	}
	commit("a.txt", "other\n", "other")
	if err := r.Switch(ctx, "main"); err != nil {
		t.Fatal(err)
	}
	commit("a.txt", "mine\n", "mine")
	if err := r.Merge(ctx, "other"); err == nil {
		t.Fatal("merge with a conflict went through")
	}
	if op := r.Operation(); op != Merging {
		t.Fatalf("operation %q", op)
	}
	st, _ := r.Status(ctx)
	if len(st.Changes) != 1 || !st.Changes[0].Conflict {
		t.Fatalf("conflict %+v", st.Changes)
	}
	if err := r.Abort(ctx, Merging); err != nil {
		t.Fatal(err)
	}
	if op := r.Operation(); op != NoOperation {
		t.Fatalf("operation after abort %q", op)
	}
	// Resolved and staged, a rebase continues with git's message, no editor.
	if err := r.Rebase(ctx, "other"); err == nil {
		t.Fatal("rebase with a conflict went through")
	}
	if op := r.Operation(); op != Rebasing {
		t.Fatalf("operation %q", op)
	}
	write(t, r, "a.txt", "both\n")
	if err := r.Stage(ctx, "a.txt"); err != nil {
		t.Fatal(err)
	}
	if err := r.Continue(ctx, Rebasing); err != nil {
		t.Fatal(err)
	}
	if op := r.Operation(); op != NoOperation {
		t.Fatalf("operation after continuing %q", op)
	}
}

func TestRebaseInteractive(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	for _, name := range []string{"a", "b", "c", "d"} {
		write(t, r, name+".txt", name)
		if err := r.Stage(ctx, name+".txt"); err != nil {
			t.Fatal(err)
		}
		if err := r.Commit(ctx, name, false); err != nil {
			t.Fatal(err)
		}
	}
	var log []Commit
	_ = r.Log(ctx, Scope{}, 10, func(c []Commit) { log = append(log, c...) })
	base := log[3].Hash // a
	steps, err := r.RebaseSteps(ctx, base)
	if err != nil || len(steps) != 3 || steps[0].Subject != "b" {
		t.Fatalf("steps %+v, %v", steps, err)
	}
	// d first, reworded; b with c squashed into it... as c, b; drop nothing.
	b, c, d := steps[0], steps[1], steps[2]
	d.Action, d.Message = Reword, "dee\n\nbody"
	c.Action = Fixup
	if err := CheckSteps([]RebaseStep{c, b}); err == nil {
		t.Fatal("a fixup first is fine?")
	}
	if err := r.RebaseInteractive(ctx, base, []RebaseStep{d, b, c}); err != nil {
		t.Fatal(err)
	}
	log = nil
	_ = r.Log(ctx, Scope{}, 10, func(c []Commit) { log = append(log, c...) })
	var subjects []string
	for _, c := range log {
		subjects = append(subjects, c.Subject)
	}
	if !slices.Equal(subjects, []string{"b", "dee", "a"}) {
		t.Fatalf("history %v", subjects)
	}
	if out, _ := r.Git(ctx, "show", "--stat", "--format=", "HEAD"); !strings.Contains(out, "b.txt") || !strings.Contains(out, "c.txt") {
		t.Fatalf("b's commit doesn't hold c's change: %s", out)
	}
	if r.Operation() != NoOperation {
		t.Fatal("rebase still under way")
	}
	if !r.IsAncestor(ctx, base) {
		t.Fatal("a isn't HEAD's ancestor")
	}
}

func TestBlameAndRemote(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	write(t, r, "a.txt", "one\ntwo\n")
	_ = r.Stage(ctx, "a.txt")
	_ = r.Commit(ctx, "first\n\nwith a body", false)
	write(t, r, "a.txt", "one\nTWO\n")
	_ = r.Stage(ctx, "a.txt")
	_ = r.Commit(ctx, "second", false)
	lines, err := r.Blame(ctx, "", "a.txt")
	if err != nil || len(lines) != 2 || lines[0].Summary != "first" || lines[1].Summary != "second" ||
		lines[1].Text != "TWO" || lines[0].Author != "Ada" {
		t.Fatalf("blame %+v, %v", lines, err)
	}
	if m := r.Message(ctx, lines[0].Hash); m != "first\n\nwith a body" {
		t.Fatalf("message %q", m)
	}
	// A bare clone as origin: nothing to push once pushed.
	origin := t.TempDir()
	if _, err := run(ctx, origin, "init", "-q", "--bare"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Git(ctx, "remote", "add", "origin", origin); err != nil {
		t.Fatal(err)
	}
	st, _ := r.Status(ctx)
	if out, err := r.Outgoing(ctx, st); err != nil || len(out) != 2 {
		t.Fatalf("outgoing before pushing %d, %v", len(out), err)
	}
	if err := r.Push(ctx, st, false); err != nil {
		t.Fatal(err)
	}
	st, _ = r.Status(ctx)
	if out, err := r.Outgoing(ctx, st); err != nil || len(out) != 0 || st.Upstream != "origin/main" {
		t.Fatalf("outgoing after pushing %d, %v, upstream %q", len(out), err, st.Upstream)
	}
	if err := r.Pull(ctx, PullRebase); err != nil {
		t.Fatal(err)
	}
}

func TestCloneInitRemotes(t *testing.T) {
	ctx := context.Background()
	src := repo(t)
	write(t, src, "a.txt", "a")
	_ = src.Stage(ctx, "a.txt")
	_ = src.Commit(ctx, "a", false)
	parent := t.TempDir()
	var lines []string
	if err := Clone(ctx, parent, src.Dir, "copy", func(l string) { lines = append(lines, l) }); err != nil {
		t.Fatal(err)
	}
	r, err := Open(filepath.Join(parent, "copy"))
	if err != nil || len(lines) == 0 {
		t.Fatalf("clone %v, progress %q", err, lines)
	}
	if err := Clone(ctx, parent, filepath.Join(parent, "nothing"), "x", nil); err == nil || !strings.HasPrefix(err.Error(), "fatal:") {
		t.Fatalf("cloning nothing: %v", err)
	}
	if err := r.AddRemote(ctx, "upstream", "https://example.com/a.git"); err != nil {
		t.Fatal(err)
	}
	if err := r.RenameRemote(ctx, "upstream", "mirror"); err != nil {
		t.Fatal(err)
	}
	if err := r.SetRemoteURL(ctx, "mirror", "https://example.com/b.git"); err != nil {
		t.Fatal(err)
	}
	list, _ := r.Remotes(ctx)
	if len(list) != 2 || !slices.Contains(list, Remote{"mirror", "https://example.com/b.git"}) {
		t.Fatalf("remotes %+v", list)
	}
	if err := r.RemoveRemote(ctx, "mirror"); err != nil {
		t.Fatal(err)
	}
	if err := r.FetchRemote(ctx, "origin"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := Init(ctx, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}
	for url, want := range map[string]string{
		"https://github.com/egoist/mygo.git": "mygo", "git@github.com:a/b.git": "b", "https://x.org/c/": "c", "/tmp/d": "d",
	} {
		if got := RepoName(url); got != want {
			t.Errorf("RepoName(%q) = %q", url, got)
		}
	}
}
