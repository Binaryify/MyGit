package git

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Change is a file that differs from HEAD, in the index, the working tree
// or both.
type Change struct {
	Path     string
	OldPath  string // where a renamed file was
	Staged   byte   // the index against HEAD: 'M', 'A', 'D', 'R', … or 0 for none
	Work     byte   // the working tree against the index, '?' untracked, 0 for none
	Conflict bool
}

// Status is the branch and the files changed.
type Status struct {
	Branch        string // "" when HEAD is detached
	Upstream      string
	Ahead, Behind int
	Changes       []Change
}

// Status reads git status --porcelain=v2.
func (r *Repo) Status(ctx context.Context) (Status, error) {
	out, err := r.Git(ctx, "status", "--porcelain=v2", "--branch", "-z", "--untracked-files=all")
	if err != nil {
		return Status{}, err
	}
	return parseStatus(out), nil
}

func parseStatus(out string) Status {
	var s Status
	fields := strings.Split(out, "\x00")
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		switch {
		case strings.HasPrefix(f, "# branch.head "):
			if b := strings.TrimPrefix(f, "# branch.head "); b != "(detached)" {
				s.Branch = b
			}
		case strings.HasPrefix(f, "# branch.upstream "):
			s.Upstream = strings.TrimPrefix(f, "# branch.upstream ")
		case strings.HasPrefix(f, "# branch.ab "):
			ab := strings.Fields(strings.TrimPrefix(f, "# branch.ab "))
			if len(ab) == 2 {
				s.Ahead, _ = strconv.Atoi(strings.TrimPrefix(ab[0], "+"))
				s.Behind, _ = strconv.Atoi(strings.TrimPrefix(ab[1], "-"))
			}
		case strings.HasPrefix(f, "1 "): // 1 XY sub mH mI mW hH hI path
			if p := strings.SplitN(f, " ", 9); len(p) == 9 {
				s.Changes = append(s.Changes, change(p[1], p[8]))
			}
		case strings.HasPrefix(f, "2 "): // 2 XY sub mH mI mW hH hI Xscore path, then origPath
			if p := strings.SplitN(f, " ", 10); len(p) == 10 {
				c := change(p[1], p[9])
				if i+1 < len(fields) {
					i++
					c.OldPath = fields[i]
				}
				s.Changes = append(s.Changes, c)
			}
		case strings.HasPrefix(f, "u "): // u XY sub m1 m2 m3 mW h1 h2 h3 path
			if p := strings.SplitN(f, " ", 11); len(p) == 11 {
				s.Changes = append(s.Changes, Change{Path: p[10], Work: 'U', Conflict: true})
			}
		case strings.HasPrefix(f, "? "):
			s.Changes = append(s.Changes, Change{Path: f[2:], Work: '?'})
		}
	}
	return s
}

func change(xy, path string) Change {
	c := Change{Path: path}
	if xy[0] != '.' {
		c.Staged = xy[0]
	}
	if xy[1] != '.' {
		c.Work = xy[1]
	}
	return c
}

// Diff is the patch of a change: the index against HEAD when staged, else
// the working tree against the index.
func (r *Repo) Diff(ctx context.Context, path string, staged bool) (string, error) {
	args := []string{"diff", "-M", "--no-ext-diff"}
	if staged {
		args = append(args, "--cached")
	}
	return r.Git(ctx, append(args, "--", path)...)
}

// ReadFile reads a file of the working tree, as an untracked file shows.
func (r *Repo) ReadFile(path string) (string, error) {
	b, err := os.ReadFile(filepath.Join(r.Dir, filepath.FromSlash(path)))
	return string(b), err
}

// Stage adds the files' changes to the index, deletions included.
func (r *Repo) Stage(ctx context.Context, paths ...string) error {
	_, err := r.Git(ctx, append([]string{"add", "--all", "--"}, paths...)...)
	return err
}

// Unstage takes the files' changes out of the index, leaving the working
// tree as it is.
func (r *Repo) Unstage(ctx context.Context, paths ...string) error {
	if _, err := r.Git(ctx, "rev-parse", "--verify", "-q", "HEAD"); err != nil {
		_, err := r.Git(ctx, append([]string{"rm", "--cached", "-q", "-r", "--"}, paths...)...) // no commit yet
		return err
	}
	_, err := r.Git(ctx, append([]string{"restore", "--staged", "--"}, paths...)...)
	return err
}

// Commit commits the index.
func (r *Repo) Commit(ctx context.Context, message string, amend bool) error {
	args := []string{"commit", "-m", message}
	if amend {
		args = append(args, "--amend")
	}
	_, err := r.Git(ctx, args...)
	return err
}

// LastMessage is HEAD's message, which amending starts from.
func (r *Repo) LastMessage(ctx context.Context) string {
	out, _ := r.Git(ctx, "log", "-1", "--format=%B")
	return strings.TrimSpace(out)
}

// Apply applies a patch: to the index when cached, else to the working
// tree, and backwards when reverse, as unstaging and discarding lines do.
func (r *Repo) Apply(ctx context.Context, patch string, cached, reverse bool) error {
	args := []string{"apply", "--whitespace=nowarn"}
	if cached {
		args = append(args, "--cached")
	}
	if reverse {
		args = append(args, "--reverse")
	}
	_, err := runInput(ctx, r.Dir, patch, append(args, "-")...)
	return err
}

// Discard throws away the working tree's changes of the files: tracked
// files go back to the index, untracked ones are deleted.
func (r *Repo) Discard(ctx context.Context, c Change) error {
	if c.Work == '?' {
		return os.RemoveAll(filepath.Join(r.Dir, filepath.FromSlash(c.Path)))
	}
	_, err := r.Git(ctx, "restore", "--worktree", "--", c.Path)
	return err
}
