package main

import (
	"context"
	"slices"
	"testing"

	"mygit/internal/git"
	loc "mygit/locales"
)

// The history shows every branch's commits when asked, which stay shown
// as the branch checked out changes (#45).
func TestHistoryAllBranches(t *testing.T) {
	tt := newTester(t)
	ctx := context.Background()
	for _, args := range [][]string{{"switch", "-q", "-c", "side"}, {"commit", "-q", "--allow-empty", "-m", "Side only"}, {"switch", "-q", "main"}} {
		if _, err := tt.a.repo.Git(ctx, args...); err != nil {
			t.Fatal(err)
		}
	}
	tt.a.refresh()
	tt.settle()
	subjects := func() []string {
		var s []string
		for _, c := range tt.a.commits {
			s = append(s, c.Subject)
		}
		return s
	}
	if slices.Contains(subjects(), "Side only") || !tt.HasText(loc.HistoryCurrentBranch()+" ▾") {
		t.Fatalf("current branch: %q", subjects())
	}
	tt.a.setScope(git.Scope{})
	tt.settle()
	if !slices.Contains(subjects(), "Side only") || !tt.HasText(loc.HistoryAllBranches()+" ▾") {
		t.Fatalf("all branches: %q", subjects())
	}
	tt.a.switchTo("side")
	tt.settle()
	if tt.a.status.Branch != "side" || tt.a.scope.Ref != "" {
		t.Fatalf("switched: on %q, scope %+v", tt.a.status.Branch, tt.a.scope)
	}
}
