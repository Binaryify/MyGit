package git

import (
	"context"
	"os"
	"path/filepath"
)

// Operation is what the repository is in the middle of, which stopped for
// conflicts or, rebasing, for a commit to edit.
type Operation string

const (
	NoOperation Operation = ""
	Merging     Operation = "merge"
	Rebasing    Operation = "rebase"
	CherryPick  Operation = "cherry-pick"
	Reverting   Operation = "revert"
)

// Operation reads which operation is under way from the files git keeps
// for it.
func (r *Repo) Operation() Operation {
	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(r.GitDir, name))
		return err == nil
	}
	switch {
	case exists("rebase-merge"), exists("rebase-apply"):
		return Rebasing
	case exists("MERGE_HEAD"):
		return Merging
	case exists("CHERRY_PICK_HEAD"):
		return CherryPick
	case exists("REVERT_HEAD"):
		return Reverting
	}
	return NoOperation
}

// Continue goes on with the operation once its conflicts are resolved and
// staged, with the message git proposes.
func (r *Repo) Continue(ctx context.Context, op Operation) error {
	_, err := r.Git(ctx, string(op), "--continue")
	return err
}

// Abort gives the operation up, putting HEAD back as it was before it.
func (r *Repo) Abort(ctx context.Context, op Operation) error {
	_, err := r.Git(ctx, string(op), "--abort")
	return err
}

// Skip leaves the commit being replayed out, rebasing or cherry-picking.
func (r *Repo) Skip(ctx context.Context, op Operation) error {
	_, err := r.Git(ctx, string(op), "--skip")
	return err
}
