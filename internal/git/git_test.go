package git

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"mygit/internal/helper"
)

// repo makes a repository in a temporary folder.
func repo(t *testing.T) *Repo {
	t.Helper()
	dir := t.TempDir()
	if _, err := git(context.Background(), Local, dir, "", nil, "init", "-q", "-b", "main"); err != nil {
		t.Skip("git: ", err)
	}
	for _, kv := range [][2]string{{"user.name", "Ada"}, {"user.email", "ada@example.com"}, {"commit.gpgsign", "false"}} {
		if _, err := git(context.Background(), Local, dir, "", nil, "config", kv[0], kv[1]); err != nil {
			t.Fatal(err)
		}
	}
	r, err := OpenOn(testRunner(t), dir)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// testRunner is Local, or a helper over pipes when the tests run again
// as another computer's would.
func testRunner(t *testing.T) Runner {
	if os.Getenv("MYGIT_TEST_HELPER") == "" {
		return Local
	}
	toServer, fromClient := io.Pipe()
	toClient, fromServer := io.Pipe()
	go func() { _ = helper.Serve(toServer, fromServer) }()
	c, err := helper.NewClient("box", toClient, fromClient, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fromClient.Close() })
	return c
}

// TestOverHelper runs the tests again with git run by a helper, as for a
// repository on another computer.
func TestOverHelper(t *testing.T) {
	if os.Getenv("MYGIT_TEST_HELPER") != "" || testing.Short() {
		t.Skip()
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run", "^Test[^O]", "-test.count=1")
	cmd.Env = append(os.Environ(), "MYGIT_TEST_HELPER=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
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
	if m := r.RecentMessages(ctx, 10); !slices.Equal(m, []string{"second", "first"}) {
		t.Fatal("recent messages", m)
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
	if err := r.Tag(ctx, "v0", log[len(log)-1].Hash, ""); err != nil {
		t.Fatal(err)
	}
	if tags, _ := r.Tags(ctx); !slices.Equal(slices.Sorted(slices.Values(tags)), []string{"v0", "v1"}) {
		t.Fatalf("tags %v", tags)
	}
	if err := r.DeleteTag(ctx, "v0"); err != nil {
		t.Fatal(err)
	}
	if tags, _ := r.Tags(ctx); !slices.Equal(tags, []string{"v1"}) {
		t.Fatalf("tags after deleting v0: %v", tags)
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
	if op := r.Operation(ctx); op != Merging {
		t.Fatalf("operation %q", op)
	}
	st, _ := r.Status(ctx)
	if len(st.Changes) != 1 || !st.Changes[0].Conflict {
		t.Fatalf("conflict %+v", st.Changes)
	}
	if err := r.Abort(ctx, Merging); err != nil {
		t.Fatal(err)
	}
	if op := r.Operation(ctx); op != NoOperation {
		t.Fatalf("operation after abort %q", op)
	}
	// Resolved and staged, a rebase continues with git's message, no editor.
	if err := r.Rebase(ctx, "other"); err == nil {
		t.Fatal("rebase with a conflict went through")
	}
	if op := r.Operation(ctx); op != Rebasing {
		t.Fatalf("operation %q", op)
	}
	write(t, r, "a.txt", "both\n")
	if err := r.Stage(ctx, "a.txt"); err != nil {
		t.Fatal(err)
	}
	if err := r.Continue(ctx, Rebasing); err != nil {
		t.Fatal(err)
	}
	if op := r.Operation(ctx); op != NoOperation {
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
	if r.Operation(ctx) != NoOperation {
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
	if _, err := git(ctx, Local, origin, "", nil, "init", "-q", "--bare"); err != nil {
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
	if !r.Pushed(ctx, lines[0].Hash) {
		t.Fatal("pushed commit not pushed")
	}
}

func TestReword(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	for _, name := range []string{"a", "b", "c"} {
		write(t, r, name+".txt", name)
		_ = r.Stage(ctx, name+".txt")
		_ = r.Commit(ctx, name, false)
	}
	write(t, r, "a.txt", "not committed") // stashed meanwhile, staged left out
	write(t, r, "d.txt", "staged")
	_ = r.Stage(ctx, "d.txt")
	logged := func() []Commit {
		var log []Commit
		_ = r.Log(ctx, Scope{}, 10, func(c []Commit) { log = append(log, c...) })
		return log
	}
	if err := r.Reword(ctx, logged()[0], "cee\n\nbody"); err != nil {
		t.Fatal(err)
	}
	if err := r.Reword(ctx, logged()[2], "ay"); err != nil {
		t.Fatal(err)
	}
	var subjects []string
	for _, c := range logged() {
		subjects = append(subjects, c.Subject)
	}
	if !slices.Equal(subjects, []string{"cee", "b", "ay"}) || r.LastMessage(ctx) != "cee\n\nbody" {
		t.Fatalf("history %v, last %q", subjects, r.LastMessage(ctx))
	}
	if out, _ := r.Git(ctx, "status", "--porcelain"); out != " M a.txt\nA  d.txt\n" {
		t.Fatalf("changes %q", out)
	}
	if out, _ := r.Git(ctx, "show", "--stat", "--format=", "HEAD"); strings.Contains(out, "d.txt") {
		t.Fatalf("staged change amended in: %s", out)
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

// Trusting a repository on another computer writes its path to git's
// safe.directory there, the user's.
func TestTrustOn(t *testing.T) {
	r := repo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	if err := TrustOn(context.Background(), r.run, r.Dir); err != nil {
		t.Fatal(err)
	}
	out, err := git(context.Background(), Local, r.Dir, "", nil, "config", "--global", "--get-all", "safe.directory")
	if err != nil || !strings.Contains(out, filepath.ToSlash(r.Dir)) {
		t.Errorf("safe.directory %q, %v", out, err)
	}
}

// Thousands of paths go to git in runs short enough for a command line,
// each path once, in order.
func TestInBatches(t *testing.T) {
	var paths []string
	for i := range 3000 {
		paths = append(paths, fmt.Sprintf("some/folder/deep/down/file-%04d.txt", i))
	}
	var got []string
	runs := 0
	err := inBatches(paths, func(batch []string) error {
		runs++
		size := 0
		for _, p := range batch {
			size += len(p) + 3
		}
		if size > 16<<10 && len(batch) > 1 {
			t.Fatalf("a run of %d bytes", size)
		}
		got = append(got, batch...)
		return nil
	})
	if err != nil || runs < 2 || !slices.Equal(got, paths) {
		t.Fatalf("runs %d, %d paths back, err %v", runs, len(got), err)
	}
	if err := inBatches(nil, func([]string) error { t.Fatal("ran for no paths"); return nil }); err != nil {
		t.Fatal(err)
	}
}

// Worktrees are listed, added and removed; removing one with changes in
// it takes force. A submodule's are named by its working tree, not by
// its repository in the superproject's .git/modules.
func TestWorktrees(t *testing.T) {
	ctx := context.Background()
	r := repo(t)
	write(t, r, "a.txt", "a")
	if err := r.Stage(ctx, "a.txt"); err != nil {
		t.Fatal(err)
	}
	if err := r.Commit(ctx, "a", false); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(filepath.Dir(r.Dir), filepath.Base(r.Dir)+"-topic")
	if err := r.AddWorktree(ctx, dir, "HEAD", "topic"); err != nil {
		t.Fatal(err)
	}
	trees, err := r.Worktrees(ctx)
	if err != nil || len(trees) != 2 || trees[0].Dir != r.Dir || !trees[0].Main || trees[0].Branch != "main" ||
		trees[1].Branch != "topic" || trees[1].Main || trees[1].Head == "" {
		t.Fatalf("worktrees %+v, %v", trees, err)
	}
	other, err := r.Open(trees[1].Dir)
	if err != nil || other.Dir != trees[1].Dir {
		t.Fatalf("opening %s: %v", trees[1].Dir, err)
	}
	if again, _ := other.Worktrees(ctx); len(again) != 2 || again[0].Dir != r.Dir {
		t.Fatalf("from the linked one %+v", again)
	}
	if err := os.WriteFile(filepath.Join(other.Dir, "new.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := r.RemoveWorktree(ctx, other.Dir, false); !WorktreeNeedsForce(err) {
		t.Fatalf("removing one with a file in it: %v", err)
	}
	if err := r.RemoveWorktree(ctx, other.Dir, true); err != nil {
		t.Fatal(err)
	}
	if trees, _ := r.Worktrees(ctx); len(trees) != 1 || r.Superproject(ctx) != "" {
		t.Fatalf("worktrees after removing %+v", trees)
	}

	// A submodule of another repository, and a worktree of it.
	super := repo(t)
	if _, err := super.Git(ctx, "-c", "protocol.file.allow=always", "submodule", "add", "-q", r.Dir, "lib"); err != nil {
		t.Fatal(err)
	}
	sub, err := super.Open(super.Path("lib"))
	if err != nil {
		t.Fatal(err)
	}
	if sub.Superproject(ctx) != super.Dir {
		t.Fatalf("superproject %q", sub.Superproject(ctx))
	}
	if err := sub.AddWorktree(ctx, dir, "main", "lib-topic"); err != nil {
		t.Fatal(err)
	}
	linked, _ := sub.Open(dir)
	for _, from := range []*Repo{sub, linked} {
		if trees, _ := from.Worktrees(ctx); len(trees) != 2 || trees[0].Dir != sub.Dir || trees[1].Dir != linked.Dir {
			t.Fatalf("submodule's worktrees from %s: %+v", from.Dir, trees)
		}
	}
}

func TestPullStashesChanges(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	write(t, r, "a.txt", "one\ntwo\nthree\nfour\nfive\n")
	_ = r.Stage(ctx, "a.txt")
	_ = r.Commit(ctx, "a", false)
	origin := t.TempDir()
	if _, err := git(ctx, Local, origin, "", nil, "init", "-q", "--bare"); err != nil {
		t.Fatal(err)
	}
	_, _ = r.Git(ctx, "remote", "add", "origin", origin)
	st, _ := r.Status(ctx)
	if err := r.Push(ctx, st, false); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "other")
	if _, err := git(ctx, Local, "", "", nil, "clone", "-q", origin, other); err != nil {
		t.Fatal(err)
	}
	// Each pull brings a change to the file's last line in, while its first
	// line is changed here and not committed.
	for i, mode := range []PullMode{PullFastForward, PullRebase, PullMerge} {
		last := fmt.Sprintf("five %d\n", i)
		if err := os.WriteFile(filepath.Join(other, "a.txt"), []byte("one\ntwo\nthree\nfour\n"+last), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := git(ctx, Local, other, "", nil, "-c", "user.name=b", "-c", "user.email=b@b", "commit", "-qam", last); err != nil {
			t.Fatal(err)
		}
		if _, err := git(ctx, Local, other, "", nil, "push", "-q"); err != nil {
			t.Fatal(err)
		}
		write(t, r, "a.txt", "ONE\ntwo\nthree\nfour\nfive\n")
		if i > 0 {
			write(t, r, "a.txt", fmt.Sprintf("ONE\ntwo\nthree\nfour\nfive %d\n", i-1))
		}
		if err := r.Pull(ctx, mode); err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		if b, _ := os.ReadFile(filepath.Join(r.Dir, "a.txt")); string(b) != "ONE\ntwo\nthree\nfour\n"+last {
			t.Fatalf("%s: a.txt %q", mode, b)
		}
	}
}

func TestSquash(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	for _, name := range []string{"a", "b", "c", "d"} {
		write(t, r, name+".txt", name)
		_ = r.Stage(ctx, name+".txt")
		_ = r.Commit(ctx, name, false)
	}
	var log []Commit
	_ = r.Log(ctx, Scope{}, 10, func(c []Commit) { log = append(log, c...) })
	// b and d, apart: d moves after b and melds into it, c follows.
	if err := r.Squash(ctx, log[2], []string{log[0].Hash}, "b and d"); err != nil {
		t.Fatal(err)
	}
	log = nil
	_ = r.Log(ctx, Scope{}, 10, func(c []Commit) { log = append(log, c...) })
	var subjects []string
	for _, c := range log {
		subjects = append(subjects, c.Subject)
	}
	if !slices.Equal(subjects, []string{"c", "b and d", "a"}) {
		t.Fatalf("history %v", subjects)
	}
	if out, _ := r.Git(ctx, "show", "--stat", "--format=", "HEAD~1"); !strings.Contains(out, "b.txt") || !strings.Contains(out, "d.txt") {
		t.Fatalf("squashed commit: %s", out)
	}
	if err := r.Squash(ctx, log[1], []string{"0123456789012345678901234567890123456789"}, "x"); err == nil {
		t.Fatal("squashed a commit not in the history")
	}
}
