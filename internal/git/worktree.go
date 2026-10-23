package git

import (
	"context"
	"path"
	"strings"
)

// Worktree is a working tree of the repository, as git worktree list
// tells them, the main one first.
type Worktree struct {
	Dir    string
	Branch string // checked out there, "" when its HEAD is detached
	Head   string // the commit checked out
	Main   bool   // the repository's own working tree, which can't be removed
}

// Worktrees are the repository's working trees. A submodule's main one,
// which git names by the repository in the superproject's .git/modules,
// is named by its working tree, as IntelliJ does.
func (r *Repo) Worktrees(ctx context.Context) ([]Worktree, error) {
	out, err := r.Git(ctx, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var trees []Worktree
	for _, f := range strings.Split(out, "\n") {
		k, v, _ := strings.Cut(f, " ")
		switch {
		case k == "worktree":
			trees = append(trees, Worktree{Dir: r.run.Join(v, ""), Main: len(trees) == 0})
		case len(trees) == 0:
		case k == "HEAD":
			trees[len(trees)-1].Head = v
		case k == "branch":
			trees[len(trees)-1].Branch = strings.TrimPrefix(v, "refs/heads/")
		}
	}
	if len(trees) > 0 && trees[0].Dir == r.commonDir() {
		if r.GitDir == trees[0].Dir {
			trees[0].Dir = r.Dir
		} else if top, err := git(ctx, r.run, trees[0].Dir, "", nil, "rev-parse", "--show-toplevel"); err == nil {
			trees[0].Dir = r.run.Join(strings.TrimSpace(top), "")
		}
	}
	return trees, nil
}

// commonDir is the repository the working trees share: GitDir, or for a
// linked working tree's, <common>/worktrees/<name>, the folder above.
func (r *Repo) commonDir() string {
	parent := r.run.Join(r.GitDir, "..")
	if path.Base(strings.ReplaceAll(parent, `\`, "/")) == "worktrees" {
		return r.run.Join(parent, "..")
	}
	return r.GitDir
}

// AddWorktree checks rev out in a new working tree at dir: in a new
// branch, or else a local branch as it is and anything else detached.
func (r *Repo) AddWorktree(ctx context.Context, dir, rev, branch string) error {
	args := []string{"worktree", "add"}
	if branch != "" {
		args = append(args, "-b", branch)
	}
	_, err := r.Git(ctx, append(args, "--", dir, rev)...)
	return err
}

// RemoveWorktree removes a linked working tree and its folder; forced, with
// the changes in it and its submodules.
func (r *Repo) RemoveWorktree(ctx context.Context, dir string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	_, err := r.Git(ctx, append(args, "--", dir)...)
	return err
}

// WorktreeNeedsForce tells whether RemoveWorktree failed for what removing
// it would lose: changes, files git doesn't track, or submodules.
func WorktreeNeedsForce(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "use --force") || strings.Contains(err.Error(), "containing submodules"))
}

// Superproject is the working tree of the repository this one is a
// submodule of, "" when it isn't one.
func (r *Repo) Superproject(ctx context.Context) string {
	out, err := r.Git(ctx, "rev-parse", "--show-superproject-working-tree")
	if err != nil || strings.TrimSpace(out) == "" {
		return ""
	}
	return r.run.Join(strings.TrimSpace(out), "")
}

// Join joins a path of the repository's computer and a slash-separated one
// under it.
func (r *Repo) Join(dir, rel string) string { return r.run.Join(dir, rel) }

// Open opens the repository dir is in, on the same computer.
func (r *Repo) Open(dir string) (*Repo, error) { return OpenOn(r.run, dir) }
