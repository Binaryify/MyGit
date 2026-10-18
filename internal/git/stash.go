package git

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// Stash is a stash entry: "stash@{0}", with the commit that holds it.
type Stash struct {
	Ref, Hash string
	Time      time.Time
	Subject   string // "On main: message", or "WIP on main: …"
}

// Stashes lists the stash, newest first.
func (r *Repo) Stashes(ctx context.Context) ([]Stash, error) {
	out, err := r.Git(ctx, "stash", "list", "-z", "--format=%gd\x1f%H\x1f%at\x1f%gs")
	if err != nil {
		return nil, err
	}
	var list []Stash
	for _, rec := range strings.Split(out, "\x00") {
		f := strings.Split(rec, "\x1f")
		if len(f) != 4 {
			continue
		}
		sec, _ := strconv.ParseInt(f[2], 10, 64)
		list = append(list, Stash{Ref: f[0], Hash: f[1], Time: time.Unix(sec, 0), Subject: f[3]})
	}
	return list, nil
}

// StashPush stashes the working tree's and the index's changes, the
// untracked files too when asked.
func (r *Repo) StashPush(ctx context.Context, message string, untracked bool) error {
	args := []string{"stash", "push"}
	if message != "" {
		args = append(args, "-m", message)
	}
	if untracked {
		args = append(args, "--include-untracked")
	}
	_, err := r.Git(ctx, args...)
	return err
}

// StashApply applies a stash's changes, restoring what was staged as
// staged; pop drops the stash once applied.
func (r *Repo) StashApply(ctx context.Context, ref string, pop bool) error {
	verb := "apply"
	if pop {
		verb = "pop"
	}
	_, err := r.Git(ctx, "stash", verb, "--index", ref)
	return err
}

// StashDrop deletes a stash.
func (r *Repo) StashDrop(ctx context.Context, ref string) error {
	_, err := r.Git(ctx, "stash", "drop", ref)
	return err
}

// StashBranch makes a branch at the stash's commit, checks it out and
// pops the stash on it.
func (r *Repo) StashBranch(ctx context.Context, name, ref string) error {
	_, err := r.Git(ctx, "stash", "branch", name, ref)
	return err
}

// StashShow is the patch a stash holds, its untracked files included.
func (r *Repo) StashShow(ctx context.Context, ref string) (string, error) {
	return r.Git(ctx, "stash", "show", "--patch", "--include-untracked", "-M", ref)
}
