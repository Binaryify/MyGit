package main

import (
	"context"

	"mygit/internal/git"
)

// The one list, as IDEA's commit window has it (#37): a check says what
// the commit includes. It is not git's index: checking a file stages
// nothing, and committing takes the working tree's version of the files
// checked, as `git commit -- <paths>` does. Tracked changes start
// checked, untracked files unchecked.

// inCommit tells whether a change of the one list goes into the commit:
// the user's choice, or for one not chosen yet, every tracked change.
func (a *app) inCommit(c git.Change) bool {
	if on, ok := a.chosen[c.Path]; ok {
		return on
	}
	return c.Work != '?'
}

// chooseCommit includes a change of the one list in the commit, or leaves
// it out, changing nothing of git's index.
func (a *app) chooseCommit(c git.Change, on bool) {
	a.chosen[c.Path] = on
}

// toggleCommit flips whether a change of the one list goes into the
// commit.
func (a *app) toggleCommit(c git.Change) { a.chooseCommit(c, !a.inCommit(c)) }

// allInCommit tells whether every change of the one list goes into the
// commit.
func (a *app) allInCommit() bool {
	for _, c := range a.status.Changes {
		if !a.inCommit(c) {
			return false
		}
	}
	return len(a.status.Changes) > 0
}

// chooseAllCommits includes every change of the one list in the commit,
// or none.
func (a *app) chooseAllCommits(on bool) {
	for _, c := range a.status.Changes {
		a.chosen[c.Path] = on
	}
}

// choosePathsCommits includes the changes at paths in the commit, or none,
// as a folder's check does.
func (a *app) choosePathsCommits(on bool, paths ...string) {
	for _, p := range paths {
		a.chosen[p] = on
	}
}

// chosenChanges are the changes of the one list the commit includes, in
// the list's order.
func (a *app) chosenChanges() []git.Change {
	var out []git.Change
	for _, c := range a.status.Changes {
		if a.inCommit(c) {
			out = append(out, c)
		}
	}
	return out
}

// commitCount is how many changes the commit would take: the one list's
// chosen, else the staged.
func (a *app) commitCount() int {
	if prefs.OneList {
		return len(a.chosenChanges())
	}
	return len(a.staged)
}

// discardRename puts a renamed file back as HEAD has it: the new path goes,
// from the index and the working tree, and the old one returns. The one
// list's changes are against HEAD, so both sides of a rename are its own.
func (a *app) discardRename(ctx context.Context, c git.Change) error {
	if c.Staged != 0 {
		if err := a.repo.Unstage(ctx, c.Path); err != nil {
			return err
		}
	}
	if err := a.repo.RestoreHead(ctx, c.OldPath); err != nil {
		return err
	}
	if _, err := a.repo.ReadBytes(ctx, c.Path); err != nil {
		return nil // the working tree's gone already
	}
	if !a.local() {
		return a.repo.Remove(ctx, c.Path)
	}
	return trash(a.repo.Path(c.Path))
}

// commitChanges commits what the changes to commit are: the staged ones,
// or in the one list those chosen, which the caller reads on the main
// thread, each as its working tree has it, without the index holding them
// first. New files are added, as IDEA adds the ones checked.
func (a *app) commitChanges(ctx context.Context, msg string, amend bool, chosen []git.Change) error {
	if !prefs.OneList {
		return a.repo.Commit(ctx, msg, amend)
	}
	var paths, added []string
	for _, c := range chosen {
		paths = append(paths, c.Path)
		if c.OldPath != "" { // a rename goes as both, as git stages it
			paths = append(paths, c.OldPath)
		}
		if c.Work == '?' {
			added = append(added, c.Path)
		}
	}
	// git takes no partial commit in a merge: the files checked go into the
	// index, and the merge commits it, as IDEA's does.
	if a.repo.Operation(ctx) == git.Merging {
		if err := a.repo.Stage(ctx, paths...); err != nil {
			return err
		}
		return a.repo.Commit(ctx, msg, amend)
	}
	if len(paths) == 0 { // amending the message alone: what isn't checked stays out
		if amend {
			if err := a.repo.Unstage(ctx, "."); err != nil {
				return err
			}
		}
		return a.repo.Commit(ctx, msg, amend)
	}
	if err := a.repo.Stage(ctx, added...); err != nil {
		return err
	}
	// A partial commit starts from HEAD, amending too, so the index's other
	// changes stay staged and out of it.
	return a.repo.CommitPaths(ctx, msg, amend, paths)
}
