package git

import (
	"context"
	"strings"
)

// Branches are the local branches, by name.
func (r *Repo) Branches(ctx context.Context) ([]string, error) {
	out, err := r.Git(ctx, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return nil, err
	}
	return strings.Fields(out), nil
}

// Switch checks a local branch out.
func (r *Repo) Switch(ctx context.Context, branch string) error {
	_, err := r.Git(ctx, "switch", branch)
	return err
}

// Fetch updates every remote's branches.
func (r *Repo) Fetch(ctx context.Context) error {
	_, err := r.Git(ctx, "fetch", "--all", "--prune")
	return err
}

// Pull brings the upstream's commits in when that needs no merge.
func (r *Repo) Pull(ctx context.Context) error {
	_, err := r.Git(ctx, "pull", "--ff-only")
	return err
}

// Push sends the branch to its upstream, or to origin under its own name,
// which becomes its upstream, when it has none.
func (r *Repo) Push(ctx context.Context, st Status) error {
	if st.Upstream == "" && st.Branch != "" {
		_, err := r.Git(ctx, "push", "-u", "origin", st.Branch)
		return err
	}
	_, err := r.Git(ctx, "push")
	return err
}

// Refs sums up where HEAD and every ref point, to tell when the history
// changed.
func (r *Repo) Refs(ctx context.Context) string {
	out, _ := r.Git(ctx, "show-ref", "--head")
	return out
}
