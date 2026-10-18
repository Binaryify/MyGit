package git

import (
	"context"
	"fmt"
	"strings"
)

// PullRef is where a pull request's head is fetched to, apart from the
// branches, and PullBaseRef its base branch.
func PullRef(n int) string     { return fmt.Sprintf("refs/mygit/pull/%d", n) }
func PullBaseRef(n int) string { return fmt.Sprintf("refs/mygit/pull/%d-base", n) }

// FetchPull fetches a pull request's commits from a remote that has them
// as GitHub's do, refs/pull/N/head, and its base branch's, and returns the
// commit the pull request's changes are against: where its head branched
// off the base, as GitHub diffs it.
func (r *Repo) FetchPull(ctx context.Context, remote string, n int, base string) (mergeBase, head string, err error) {
	if _, err := r.Git(ctx, "fetch", "--no-tags", remote,
		fmt.Sprintf("+refs/pull/%d/head:%s", n, PullRef(n)),
		fmt.Sprintf("+refs/heads/%s:%s", base, PullBaseRef(n))); err != nil {
		return "", "", err
	}
	out, err := r.Git(ctx, "rev-parse", PullRef(n))
	if err != nil {
		return "", "", err
	}
	head = strings.TrimSpace(out)
	out, err = r.Git(ctx, "merge-base", PullBaseRef(n), head)
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(out), head, nil
}
