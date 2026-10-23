package main

import (
	"cmp"
	"context"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mygit/internal/git"
	loc "mygit/locales"
)

// worktreeDialog checks a branch, a tag or a commit out in a new working
// tree, as IntelliJ's: where, and in a new branch when it can't be
// checked out as it is.
type worktreeDialog struct {
	open        bool
	rev         string
	dir, branch string
	asksBranch  bool // the branch is checked out already, or rev isn't a local one
	needsBranch bool // the branch is a local one: without a new one it can't be checked out
	placed      bool // dir was written, else it follows the new branch's name
}

// openRepo opens a repository's window; tests, without windows, swap it.
var openRepo func(*git.Repo)

func init() { openRepo = open } // set apart: open's view leads back to it

// loadWorktrees reads the working trees, and the superproject when the
// repository is a submodule, as the branches' popover opens.
func (a *app) loadWorktrees() {
	type found struct {
		trees []git.Worktree
		super string
	}
	work(a, func(ctx context.Context) (found, error) {
		trees, err := a.repo.Worktrees(ctx)
		return found{trees, a.repo.Superproject(ctx)}, err
	}, func(f found, _ error) {
		a.worktrees, a.super = f.trees, f.super
	})
}

// worktreeItems are what a working tree's menu does with it.
func (a *app) worktreeItems(m *ui.Menu, w git.Worktree) {
	current := w.Dir == a.repo.Dir
	if !current && a.chose(m.Item(loc.WorktreeOpen())) {
		a.openWorktree(w.Dir)
	}
	if a.chose(m.Item(loc.CopyPath())) {
		mygo.Clipboard.WriteText(w.Dir)
	}
	if !w.Main && !current {
		m.Separator()
		if a.chose(m.Item(loc.WorktreeRemove())) {
			a.removeWorktree(w.Dir)
		}
	}
}

// worktreeNote is what a working tree's row says beside its folder: the
// branch checked out there, or the commit.
func worktreeNote(w git.Worktree) string {
	if w.Branch != "" {
		return w.Branch
	}
	return shortRev(w.Head)
}

// openWorktree opens a working tree of the repository in its window.
func (a *app) openWorktree(dir string) {
	work(a, func(context.Context) (*git.Repo, error) { return a.repo.Open(dir) }, func(r *git.Repo, err error) {
		if err != nil {
			a.errMsg = err.Error()
			return
		}
		openRepo(r)
	})
}

// removeWorktree removes a working tree, and asks before removing one
// with what would be lost in it.
func (a *app) removeWorktree(dir string) {
	name := path.Base(filepath.ToSlash(dir))
	a.run(loc.OpRemoveWorktree(name), func(ctx context.Context) error {
		err := a.repo.RemoveWorktree(ctx, dir, false)
		if git.WorktreeNeedsForce(err) {
			a.update(func() {
				a.ask(loc.WorktreeRemoveTitle(name), loc.WorktreeRemoveMessage(), loc.DeleteAnyway(), func() {
					a.run(loc.OpRemoveWorktree(name), func(ctx context.Context) error { return a.repo.RemoveWorktree(ctx, dir, true) }, nil)
				})
			})
			return nil
		}
		return err
	}, nil)
}

// newWorktree asks where to check rev out, a local branch or not, and in
// which new branch when it can't be as it is: a local branch checked out
// in a working tree already, or a remote's, which gets its own name.
func (a *app) newWorktree(rev string, local bool) {
	used := slices.ContainsFunc(a.worktrees, func(w git.Worktree) bool { return w.Branch == rev })
	d := worktreeDialog{open: true, rev: rev, asksBranch: !local || used, needsBranch: local}
	if slices.Contains(a.remotes, rev) {
		_, d.branch, _ = strings.Cut(rev, "/")
	}
	d.dir = a.worktreeDir(cmp.Or(d.branch, shortRev(rev)))
	a.worktree = d
}

// worktreeDir is where a working tree named after name goes by default,
// beside the repository's own, or a submodule's beside its superproject,
// out of the superproject's working tree.
func (a *app) worktreeDir(name string) string {
	main := a.repo.Dir
	if len(a.worktrees) > 0 {
		main = a.worktrees[0].Dir
	}
	beside := main
	if a.super != "" {
		beside = a.super
	}
	folder := path.Base(filepath.ToSlash(main)) + "-" + strings.ReplaceAll(name, "/", "-")
	return a.repo.Join(beside, "../"+folder)
}

func (a *app) worktreeView(c *ui.Context) {
	d := &a.worktree
	if !d.open {
		return
	}
	t := c.Theme()
	ui.Modal(c, &d.open, func() {
		ui.Column(c).Gap(10).Width(dialogWidth(c, 460)).Children(func() {
			ui.Text(c, loc.WorktreeTitle(shortRev(d.rev))).Bold().FontSize(14)
			if a.super != "" {
				ui.Box(c).Padding(8, 10).Radius(6).Background(t.Accent.Alpha(0.1)).Children(func() {
					ui.Text(c, loc.WorktreeSubmoduleInfo()).Wrap()
				})
			}
			ui.Text(c, loc.WorktreeLocation()).FontSize(11).TextColor(t.TextMuted)
			in := ui.TextInput(c, &d.dir).Label(loc.WorktreeLocation()).AutoFocus()
			if in.Changed() {
				d.placed = true
			}
			submitted := in.Submitted()
			if d.asksBranch {
				hint := loc.WorktreeBranchDetached()
				if d.needsBranch {
					hint = loc.WorktreeBranchNeeded()
				}
				ui.Text(c, loc.WorktreeBranch()).FontSize(11).TextColor(t.TextMuted)
				in := ui.TextInput(c, &d.branch).Label(loc.WorktreeBranch()).Placeholder(hint)
				if in.Changed() && !d.placed {
					d.dir = a.worktreeDir(cmp.Or(strings.TrimSpace(d.branch), shortRev(d.rev)))
				}
				if in.Submitted() {
					submitted = true
				}
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, loc.Cancel()).Clicked() {
					d.open = false
				}
				dir, branch := strings.TrimSpace(d.dir), strings.TrimSpace(d.branch)
				ok := dir != "" && (branch != "" || !d.asksBranch || !d.needsBranch)
				if (ui.PrimaryButton(c, loc.Create()).Disabled(!ok).Clicked() || submitted) && ok {
					d.open = false
					a.addWorktree(dir, d.rev, branch)
				}
			})
		})
	})
}

// addWorktree checks rev out at dir, in branch when given, and opens it.
// A relative dir is the repository's, as git takes it.
func (a *app) addWorktree(dir, rev, branch string) {
	if !filepath.IsAbs(dir) && !path.IsAbs(dir) {
		dir = a.repo.Path(dir)
	}
	a.run(loc.OpNewWorktree(path.Base(filepath.ToSlash(dir))), func(ctx context.Context) error {
		return a.repo.AddWorktree(ctx, dir, rev, branch)
	}, func() { a.openWorktree(dir) })
}
