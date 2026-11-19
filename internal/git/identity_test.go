package git

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestIdentity(t *testing.T) {
	ctx := context.Background()
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), ".gitconfig"))
	r := repo(t) // sets Ada locally

	if got := r.Identity(ctx); got != (Identity{Name: "Ada", Email: "ada@example.com"}) {
		t.Fatalf("local: %+v", got)
	}
	// A global one replaces the repository's own, which would override it.
	if err := r.SetIdentity(ctx, Identity{Name: "Bea", Email: "bea@example.com", Global: true}); err != nil {
		t.Fatal(err)
	}
	if got := r.Identity(ctx); got != (Identity{Name: "Bea", Email: "bea@example.com", Global: true}) {
		t.Fatalf("global: %+v", got)
	}
	if r.config(ctx, "--local", "user.name") != "" {
		t.Fatal("the local name stayed")
	}
	// A local one overrides it here, the global one kept.
	if err := r.SetIdentity(ctx, Identity{Name: "Cy", Email: "cy@example.com"}); err != nil {
		t.Fatal(err)
	}
	if got := r.Identity(ctx); got != (Identity{Name: "Cy", Email: "cy@example.com"}) {
		t.Fatalf("local again: %+v", got)
	}
	if r.config(ctx, "--global", "user.name") != "Bea" {
		t.Fatal("the global name changed")
	}
	write(t, r, "a.txt", "a\n")
	if _, err := r.Git(ctx, "add", "a.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Git(ctx, "commit", "-qm", "a"); err != nil {
		t.Fatal(err)
	}
	if out, _ := r.Git(ctx, "log", "-1", "--format=%an <%ae>"); out != "Cy <cy@example.com>\n" {
		t.Fatalf("committed as %q", out)
	}
}

func TestNoIdentity(t *testing.T) {
	if !NoIdentity(errors.New("Author identity unknown\n\n*** Please tell me who you are.")) || NoIdentity(nil) || NoIdentity(errors.New("nothing to commit")) {
		t.Fatal("NoIdentity")
	}
}
