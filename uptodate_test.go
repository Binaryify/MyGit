package main

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	loc "mygit/locales"
)

// TestUpToDateToast checks that a pull, a push or a merge with nothing to
// do says so with a toast, and one that did something doesn't (#57).
func TestUpToDateToast(t *testing.T) {
	tt := newTester(t)
	origin := filepath.Join(t.TempDir(), "origin.git")
	gitIn(t, tt.a.repo.Dir, "init", "-q", "--bare", "-b", "main", origin)
	gitIn(t, tt.a.repo.Dir, "remote", "add", "origin", origin)
	gitIn(t, tt.a.repo.Dir, "push", "-q", "-u", "origin", "main")
	gitIn(t, tt.a.repo.Dir, "commit", "-q", "--allow-empty", "-m", "Unpushed")
	tt.a.refresh()
	tt.settle()
	toasted := func() bool { return slices.Contains(tt.Texts(), loc.UpToDate()) }

	tt.a.sync() // pushes the commit
	tt.settle()
	if toasted() || tt.a.errMsg != "" || tt.a.status.Ahead != 0 {
		t.Fatalf("pushing: toast %v, error %q, ahead %d", toasted(), tt.a.errMsg, tt.a.status.Ahead)
	}

	tt.a.sync() // nothing to pull or push
	tt.settle()
	if !toasted() {
		t.Fatalf("synced up to date: no toast; texts %q", tt.Texts())
	}

	// A merge says it off the UI thread: what it hands the thread sets it.
	saidAfter := func(fn func(ctx context.Context) error) string {
		tt.a.toast = ""
		if err := tt.a.upToDate(fn)(context.Background()); err != nil {
			t.Fatal(err)
		}
		for len(tt.queue) > 0 {
			(<-tt.queue)()
		}
		return tt.a.toast
	}
	gitIn(t, tt.a.repo.Dir, "switch", "-q", "-c", "topic")
	gitIn(t, tt.a.repo.Dir, "commit", "-q", "--allow-empty", "-m", "On the topic")
	gitIn(t, tt.a.repo.Dir, "switch", "-q", "main")
	merge := func(ctx context.Context) error { return tt.a.repo.Merge(ctx, "topic") }
	if got := saidAfter(merge); got != "" {
		t.Fatalf("merging new commits said %q", got)
	}
	if got := saidAfter(merge); got != loc.UpToDate() {
		t.Fatalf("merging again said %q", got)
	}
}

// TestPushToAnotherRemote checks that the push dialog, with remotes to
// choose from, pushes to the one chosen, under the branch's name, and
// leaves the upstream as it is (#56).
func TestPushToAnotherRemote(t *testing.T) {
	tt := newTester(t)
	dirs := map[string]string{}
	for _, name := range []string{"origin", "mirror"} {
		dirs[name] = filepath.Join(t.TempDir(), name+".git")
		gitIn(t, tt.a.repo.Dir, "init", "-q", "--bare", "-b", "main", dirs[name])
		gitIn(t, tt.a.repo.Dir, "remote", "add", name, dirs[name])
	}
	gitIn(t, tt.a.repo.Dir, "push", "-q", "-u", "origin", "main")
	tt.a.refresh()
	tt.settle()

	tt.a.openPush()
	tt.settle()
	if tt.a.push.remote != "origin" || len(tt.a.push.commits) != 0 {
		t.Fatalf("opened to %q with %d commits", tt.a.push.remote, len(tt.a.push.commits))
	}
	tt.a.push.remote = "mirror" // as the select sets it
	tt.a.loadOutgoing()
	tt.settle()
	if n := len(tt.a.push.commits); n != 4 || !slices.Contains(tt.Texts(), loc.PushTitle("main", loc.NewRemoteBranch("mirror/main"))) {
		t.Fatalf("to the mirror: %d commits; texts %q", n, tt.Texts())
	}
	tt.click(t, loc.Push())
	if got := gitIn(t, dirs["mirror"], "log", "-1", "--format=%s", "main"); got != "Merge the feature" || tt.a.errMsg != "" {
		t.Fatalf("mirror has %q, error %q", got, tt.a.errMsg)
	}
	if tt.a.status.Upstream != "origin/main" {
		t.Fatalf("upstream %q", tt.a.status.Upstream)
	}
}
