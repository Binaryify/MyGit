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

// RemoteBranches are the remotes' branches, "origin/main", without the
// remotes' HEADs.
func (r *Repo) RemoteBranches(ctx context.Context) ([]string, error) {
	out, err := r.Git(ctx, "for-each-ref", "--format=%(refname:short)", "refs/remotes")
	if err != nil {
		return nil, err
	}
	var branches []string
	for _, b := range strings.Fields(out) {
		if !strings.HasSuffix(b, "/HEAD") && strings.Contains(b, "/") {
			branches = append(branches, b)
		}
	}
	return branches, nil
}

// CreateBranch makes a branch at start, HEAD when empty, and checks it out
// when asked.
func (r *Repo) CreateBranch(ctx context.Context, name, start string, checkout bool) error {
	args := []string{"branch", name}
	if checkout {
		args = []string{"switch", "-c", name}
	}
	if start != "" {
		args = append(args, start)
	}
	_, err := r.Git(ctx, args...)
	return err
}

// CheckoutRemote checks a remote's branch out as a local branch of its
// name, which tracks it.
func (r *Repo) CheckoutRemote(ctx context.Context, remoteBranch string) error {
	_, err := r.Git(ctx, "switch", "--track", remoteBranch)
	return err
}

// DeleteBranch deletes a local branch; without force, only one merged.
func (r *Repo) DeleteBranch(ctx context.Context, name string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err := r.Git(ctx, "branch", flag, name)
	return err
}

// NotMerged tells whether DeleteBranch failed for a branch not merged.
func NotMerged(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not fully merged")
}

// DeleteRemoteBranch deletes "origin/name" on its remote.
func (r *Repo) DeleteRemoteBranch(ctx context.Context, remoteBranch string) error {
	remote, name, _ := strings.Cut(remoteBranch, "/")
	_, err := r.Git(ctx, "push", remote, "--delete", name)
	return err
}

// RenameBranch renames a local branch.
func (r *Repo) RenameBranch(ctx context.Context, from, to string) error {
	_, err := r.Git(ctx, "branch", "-m", from, to)
	return err
}

// ValidBranchName tells whether git takes name for a branch.
func (r *Repo) ValidBranchName(ctx context.Context, name string) bool {
	_, err := r.Git(ctx, "check-ref-format", "--branch", name)
	return err == nil && name != ""
}
