package git

import (
	"context"
	"strings"
)

// Submodule is a repository a repository tracks in one of its folders.
type Submodule struct {
	Path        string // the folder, from the top of the superproject's working tree
	Initialized bool   // checked out, its folder a working tree; else empty
}

// Submodules are the repository's submodules, and theirs under them,
// as git submodule status --recursive lists them.
func (r *Repo) Submodules(ctx context.Context) ([]Submodule, error) {
	// Without optional locks, as Status, a watch of the repository not
	// seeing the index refreshed as a change.
	out, err := r.Git(ctx, "--no-optional-locks", "submodule", "status", "--recursive")
	if err != nil {
		return nil, err
	}
	return parseSubmodules(out), nil
}

// parseSubmodules reads git submodule status: a line each, its state, the
// commit, the folder and what describes the commit: "-" uninitialized,
// "+" another commit checked out, "U" in conflict, " " as recorded.
func parseSubmodules(out string) []Submodule {
	var subs []Submodule
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 2 {
			continue
		}
		fields := strings.Fields(line[1:])
		if len(fields) < 2 {
			continue
		}
		subs = append(subs, Submodule{Path: fields[1], Initialized: line[0] != '-'})
	}
	return subs
}

// OpenSubmodule opens the submodule in the folder rel, on the computer the
// repository is on.
func (r *Repo) OpenSubmodule(rel string) (*Repo, error) { return OpenOn(r.run, r.Path(rel)) }

// UpdateSubmodules checks the submodules out at the commits recorded,
// initializing those that aren't, theirs too, as a clone without
// --recurse-submodules leaves them empty.
func (r *Repo) UpdateSubmodules(ctx context.Context) error {
	_, err := r.Git(ctx, "submodule", "update", "--init", "--recursive")
	return err
}

// AddSubmodule clones url into dir as a submodule, dir "" for the name
// git takes from url.
func (r *Repo) AddSubmodule(ctx context.Context, url, dir string) error {
	args := []string{"submodule", "add", "--", url}
	if dir != "" {
		args = append(args, dir)
	}
	_, err := r.Git(ctx, args...)
	return err
}
