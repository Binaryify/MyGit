package git

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	loc "mygit/locales"
)

// RebaseAction is what an interactive rebase does with a commit.
type RebaseAction string

const (
	Pick   RebaseAction = "pick"   // keep it
	Reword RebaseAction = "reword" // keep it with a new message
	Edit   RebaseAction = "edit"   // stop after it, to amend it
	Squash RebaseAction = "squash" // meld it into the one before, messages joined
	Fixup  RebaseAction = "fixup"  // meld it into the one before, its message dropped
	Drop   RebaseAction = "drop"   // leave it out
)

// RebaseStep is a commit of an interactive rebase and what to do with it.
type RebaseStep struct {
	Action  RebaseAction
	Hash    string
	Subject string
	Message string // a reworded commit's new message
}

// ErrMerges says the commits to rebase have merges, which an interactive
// rebase would flatten.
var ErrMerges error = mergesError{}

type mergesError struct{}

func (mergesError) Error() string { return loc.GitMerges() }

// RebaseSteps lists the commits from the one after base to HEAD, oldest
// first, all picked; base "" starts from the root.
func (r *Repo) RebaseSteps(ctx context.Context, base string) ([]RebaseStep, error) {
	rng := "HEAD"
	if base != "" {
		rng = base + "..HEAD"
	}
	out, err := r.Git(ctx, "log", "--reverse", "-z", "--format=%H\x1f%P\x1f%s", rng)
	if err != nil {
		return nil, err
	}
	var steps []RebaseStep
	for _, rec := range strings.Split(out, "\x00") {
		f := strings.Split(strings.TrimPrefix(rec, "\n"), "\x1f")
		if len(f) != 3 {
			continue
		}
		if strings.Contains(f[1], " ") {
			return nil, ErrMerges
		}
		steps = append(steps, RebaseStep{Action: Pick, Hash: f[0], Subject: f[2]})
	}
	return steps, nil
}

// CheckSteps tells what's wrong with a rebase's steps, nil when nothing.
func CheckSteps(steps []RebaseStep) error {
	for _, s := range steps {
		switch s.Action {
		case Drop:
			continue
		case Squash, Fixup:
			return errors.New(loc.GitSquashFirst())
		case Reword:
			if strings.TrimSpace(s.Message) == "" {
				return errors.New(loc.GitRewordEmpty())
			}
		}
		return nil
	}
	return nil
}

// RebaseInteractive replays the steps on base, "" for the root. Git reads
// the todo list from a file written for it, and a reworded commit is
// picked and then amended with its message, so git opens no editor.
// Changes not committed are stashed meanwhile. A rebase stopped by a
// conflict or an edit goes on with Continue.
func (r *Repo) RebaseInteractive(ctx context.Context, base string, steps []RebaseStep) error {
	if err := CheckSteps(steps); err != nil {
		return err
	}
	// The files stay while the rebase may stop and go on.
	dir := r.run.Join(r.GitDir, "mygit-rebase")
	_ = r.run.RemoveAll(ctx, dir)
	var todo strings.Builder
	for i, s := range steps {
		if s.Action != Reword {
			fmt.Fprintf(&todo, "%s %s\n", s.Action, s.Hash)
			continue
		}
		msg := r.run.Join(dir, fmt.Sprintf("message-%d", i))
		if err := r.run.WriteFile(ctx, msg, []byte(s.Message)); err != nil {
			return err
		}
		fmt.Fprintf(&todo, "pick %s\nexec git commit --amend --only --allow-empty -F %s\n", s.Hash, shellQuote(msg))
	}
	todoFile := r.run.Join(dir, "todo")
	if err := r.run.WriteFile(ctx, todoFile, []byte(todo.String())); err != nil {
		return err
	}
	args := []string{"rebase", "-i", "--autostash"}
	if base == "" {
		args = append(args, "--root")
	} else {
		args = append(args, base)
	}
	_, err := git(ctx, r.run, r.Dir, "", []string{"GIT_SEQUENCE_EDITOR=cp " + shellQuote(todoFile)}, args...)
	return err
}

// shellQuote quotes a path for the shell git runs editors and execs in,
// with slashes, which Git for Windows' shell reads too.
func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(strings.ReplaceAll(path, `\`, "/"), "'", `'\''`) + "'"
}

// IsAncestor tells whether a commit is in HEAD's history.
func (r *Repo) IsAncestor(ctx context.Context, hash string) bool {
	_, err := r.Git(ctx, "merge-base", "--is-ancestor", hash, "HEAD")
	return err == nil
}

// Unpushed are the current branch's commits its upstream hasn't, by hash.
func (r *Repo) Unpushed(ctx context.Context) map[string]bool {
	out, err := r.Git(ctx, "rev-list", "@{upstream}..HEAD")
	if err != nil {
		return nil
	}
	hashes := map[string]bool{}
	for _, h := range strings.Fields(out) {
		hashes[h] = true
	}
	return hashes
}

// Pushed tells whether a commit is in the current branch's upstream.
func (r *Repo) Pushed(ctx context.Context, hash string) bool {
	_, err := r.Git(ctx, "merge-base", "--is-ancestor", hash, "@{upstream}")
	return err == nil
}

// Reword gives a commit of HEAD's history a new message: HEAD by
// amending only its message, an older one by rebasing what follows it.
func (r *Repo) Reword(ctx context.Context, cm Commit, message string) error {
	head, err := r.Git(ctx, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if strings.TrimSpace(head) == cm.Hash {
		_, err := r.Git(ctx, "commit", "--amend", "--only", "--allow-empty", "-m", message)
		return err
	}
	return r.Squash(ctx, cm, nil, message)
}

// Squash melds commits of HEAD's history into oldest, the oldest of them,
// as one commit with the message, where oldest is: oldest is reworded
// and the others, moved after it, fixed up into it. What follows oldest
// is rebased.
func (r *Repo) Squash(ctx context.Context, oldest Commit, others []string, message string) error {
	base, steps, err := r.stepsFrom(ctx, oldest)
	if err != nil {
		return err
	}
	steps[0].Action, steps[0].Message = Reword, message
	var melded, rest []RebaseStep
	for _, s := range steps[1:] {
		if slices.Contains(others, s.Hash) {
			s.Action = Fixup
			melded = append(melded, s)
		} else {
			rest = append(rest, s)
		}
	}
	if len(melded) != len(others) {
		return errors.New(loc.ErrRewordNotInHead())
	}
	return r.RebaseInteractive(ctx, base, slices.Concat(steps[:1], melded, rest))
}

// Drop takes commits of HEAD's history out of it, oldest the oldest of
// them and others the rest, rebasing what follows oldest.
func (r *Repo) Drop(ctx context.Context, oldest Commit, others []string) error {
	base, steps, err := r.stepsFrom(ctx, oldest)
	if err != nil {
		return err
	}
	dropped := 0
	for i := range steps {
		if i == 0 || slices.Contains(others, steps[i].Hash) {
			steps[i].Action = Drop
			dropped++
		}
	}
	if dropped != len(others)+1 {
		return errors.New(loc.ErrRewordNotInHead())
	}
	return r.RebaseInteractive(ctx, base, steps)
}

// stepsFrom lists HEAD's commits from oldest on, all picked, and the
// commit they go on, "" for the root.
func (r *Repo) stepsFrom(ctx context.Context, oldest Commit) (string, []RebaseStep, error) {
	base := ""
	if len(oldest.Parents) > 0 {
		base = oldest.Parents[0]
	}
	steps, err := r.RebaseSteps(ctx, base)
	if err != nil {
		return "", nil, err
	}
	if len(steps) == 0 || steps[0].Hash != oldest.Hash {
		return "", nil, errors.New(loc.ErrRewordNotInHead())
	}
	return base, steps, nil
}
