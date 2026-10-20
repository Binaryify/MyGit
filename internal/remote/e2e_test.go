package remote

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"mygit/internal/git"
)

// TestE2E reaches a real computer over SSH, with the helpers built by
// internal/tools/helpers:
//
//	go run ./internal/tools/helpers
//	MYGIT_E2E_HOST=box MYGIT_HELPER_DIR=$PWD/build/helpers go test ./internal/remote -run E2E -v
//
// It works in ~/.mygit-e2e there and removes it.
func TestE2E(t *testing.T) {
	host, dir := os.Getenv("MYGIT_E2E_HOST"), os.Getenv("MYGIT_HELPER_DIR")
	if host == "" || dir == "" {
		t.Skip("MYGIT_E2E_HOST and MYGIT_HELPER_DIR not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	s := SSH{Store: Store{Dir: dir}}

	start := time.Now()
	c, err := s.Dial(ctx, host, Platform{})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("first connection, %s: %v", c.Platform, time.Since(start))

	const work = ".mygit-e2e"
	defer c.RemoveAll(context.Background(), work)
	_ = c.RemoveAll(ctx, work)
	if err := c.WriteFile(ctx, work+"/hello world.txt", []byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.name", "Ada"},
		{"config", "user.email", "ada@example.com"},
		{"config", "commit.gpgsign", "false"},
	} {
		if err := c.Exec(ctx, work, nil, nil, nil, nil, args...); err != nil {
			t.Fatal(args, err)
		}
	}
	r, err := git.OpenOn(c, work)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("repository %s on %s", r.Dir, r.Host())

	changed := make(chan struct{}, 10)
	wctx, stop := context.WithCancel(ctx)
	defer stop()
	go func() { _ = c.Watch(wctx, r.Dir, func() { changed <- struct{}{} }) }()

	st, err := r.Status(ctx)
	if err != nil || len(st.Changes) != 1 || st.Changes[0].Path != "hello world.txt" {
		t.Fatalf("status %+v %v", st, err)
	}
	if err := r.Stage(ctx, "hello world.txt"); err != nil {
		t.Fatal(err)
	}
	if err := r.Commit(ctx, "First, with 'quotes' and $dollars", false); err != nil {
		t.Fatal(err)
	}
	var commits []git.Commit
	if err := r.Log(ctx, git.Scope{}, 100, func(b []git.Commit) { commits = append(commits, b...) }); err != nil {
		t.Fatal(err)
	}
	if len(commits) != 1 || commits[0].Subject != "First, with 'quotes' and $dollars" {
		t.Fatalf("log %+v", commits)
	}
	if err := r.WriteFile(ctx, "hello world.txt", "hello\nthere\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changed:
	case <-time.After(10 * time.Second):
		t.Error("the change wasn't seen")
	}
	diff, err := r.Git(ctx, "diff")
	if err != nil || !strings.Contains(diff, "+there") {
		t.Errorf("diff %q %v", diff, err)
	}
	if r.Operation(ctx) != git.NoOperation {
		t.Error("an operation under way")
	}
	home, err := c.List(ctx, "")
	if err != nil || home.Dir == "" || home.Parent == "" {
		t.Errorf("home %+v %v", home, err)
	}
	l, err := c.List(ctx, r.Dir)
	if err != nil || l.Parent != home.Dir {
		t.Errorf("listing %+v %v, home %q", l, err, home.Dir)
	}
	t.Logf("home %s, %d folders", home.Dir, len(home.Entries))
	stop()
	if err := c.Close(); err != nil {
		t.Logf("close: %v", err)
	}

	start = time.Now()
	c2, err := s.Dial(ctx, host, c.Platform)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()
	t.Logf("again, the helper there already: %v", time.Since(start))
	if !c2.Exists(ctx, work+"/hello world.txt") {
		t.Error("the file isn't there")
	}
}
