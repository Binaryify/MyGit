package git

import "context"

// Tag tags a commit, with a message when one is given (an annotated tag).
func (r *Repo) Tag(ctx context.Context, name, hash, message string) error {
	args := []string{"tag", name, hash}
	if message != "" {
		args = []string{"tag", "-a", name, "-m", message, hash}
	}
	_, err := r.Git(ctx, args...)
	return err
}

// DeleteTag deletes a local tag.
func (r *Repo) DeleteTag(ctx context.Context, name string) error {
	_, err := r.Git(ctx, "tag", "-d", name)
	return err
}

// CheckoutDetached checks a commit out with no branch.
func (r *Repo) CheckoutDetached(ctx context.Context, hash string) error {
	_, err := r.Git(ctx, "switch", "--detach", hash)
	return err
}

// mainline picks a merge commit's first parent as the one its change is
// taken against, as cherry-picking and reverting a merge need.
func mainline(c Commit) []string {
	if len(c.Parents) > 1 {
		return []string{"-m", "1"}
	}
	return nil
}

// CherryPick applies a commit's change on HEAD as a new commit.
func (r *Repo) CherryPick(ctx context.Context, c Commit) error {
	_, err := r.Git(ctx, append(append([]string{"cherry-pick"}, mainline(c)...), c.Hash)...)
	return err
}

// Revert commits the opposite of a commit's change.
func (r *Repo) Revert(ctx context.Context, c Commit) error {
	_, err := r.Git(ctx, append(append([]string{"revert", "--no-edit"}, mainline(c)...), c.Hash)...)
	return err
}

// ResetMode is how much of HEAD's state a reset takes back.
type ResetMode string

const (
	ResetSoft  ResetMode = "--soft"  // the branch only: the changes stay staged
	ResetMixed ResetMode = "--mixed" // and the index: the changes stay in the working tree
	ResetHard  ResetMode = "--hard"  // and the working tree: the changes go
)

// Reset moves the current branch to a commit.
func (r *Repo) Reset(ctx context.Context, hash string, mode ResetMode) error {
	_, err := r.Git(ctx, "reset", string(mode), hash)
	return err
}

// Merge merges a branch or commit into HEAD, with the message git
// proposes.
func (r *Repo) Merge(ctx context.Context, rev string) error {
	_, err := r.Git(ctx, "merge", "--no-edit", rev)
	return err
}

// Rebase replays HEAD's commits on another branch or commit.
func (r *Repo) Rebase(ctx context.Context, onto string) error {
	_, err := r.Git(ctx, "rebase", onto)
	return err
}
