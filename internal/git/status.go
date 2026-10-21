package git

import (
	"context"
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
	// Without optional locks status leaves the index as it is, so a watch
	// of the repository doesn't see it as a change.
	out, err := r.Git(ctx, "--no-optional-locks", "status", "--porcelain=v2", "--branch", "-z", "--untracked-files=all")
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
// the working tree against the index; ignoring whitespace changes when
// asked.
func (r *Repo) Diff(ctx context.Context, path string, staged, ignoreSpace bool) (string, error) {
	args := []string{"diff", "-M", "--no-ext-diff"}
	if staged {
		args = append(args, "--cached")
	}
	if ignoreSpace {
		args = append(args, "-w")
	}
	return r.Git(ctx, append(args, "--", path)...)
}

// ReadIndex reads a file as the index holds it, "" for one not in it.
func (r *Repo) ReadIndex(ctx context.Context, path string) string {
	out, err := r.Git(ctx, "show", ":"+path)
	if err != nil {
		return ""
	}
	return out
}

// WriteFile writes a file of the working tree, keeping its mode.
func (r *Repo) WriteFile(ctx context.Context, path, text string) error {
	return r.run.WriteFile(ctx, r.Path(path), []byte(text))
}

// ReadFile reads a file of the working tree, as an untracked file shows.
func (r *Repo) ReadFile(ctx context.Context, path string) (string, error) {
	b, err := r.ReadBytes(ctx, path)
	return string(b), err
}

// ReadBytes reads a file of the working tree as it is, as a picture.
func (r *Repo) ReadBytes(ctx context.Context, path string) ([]byte, error) {
	return r.run.ReadFile(ctx, r.Path(path))
}

// Remove deletes a file or a folder of the working tree.
func (r *Repo) Remove(ctx context.Context, path string) error {
	return r.run.RemoveAll(ctx, r.Path(path))
}

// Stage adds the files' changes to the index, deletions included.
func (r *Repo) Stage(ctx context.Context, paths ...string) error {
	return inBatches(paths, func(batch []string) error {
		_, err := r.Git(ctx, append([]string{"add", "--all", "--"}, batch...)...)
		return err
	})
}

// Unstage takes the files' changes out of the index, leaving the working
// tree as it is.
func (r *Repo) Unstage(ctx context.Context, paths ...string) error {
	args := []string{"restore", "--staged", "--"}
	if _, err := r.Git(ctx, "rev-parse", "--verify", "-q", "HEAD"); err != nil {
		args = []string{"rm", "--cached", "-q", "-r", "--"} // no commit yet
	}
	return inBatches(paths, func(batch []string) error {
		_, err := r.Git(ctx, append(args, batch...)...)
		return err
	})
}

// inBatches runs fn on runs of paths short enough for a command line,
// Windows' the shortest, as thousands of files filtered are staged at
// once; git versions of any age take them so.
func inBatches(paths []string, fn func(batch []string) error) error {
	const most = 16 << 10 // bytes, of Windows' 32K characters
	for len(paths) > 0 {
		n, size := 0, 0
		for n < len(paths) && (n == 0 || size+len(paths[n])+3 <= most) {
			size += len(paths[n]) + 3 // a space, and quotes it may need
			n++
		}
		if err := fn(paths[:n]); err != nil {
			return err
		}
		paths = paths[n:]
	}
	return nil
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
	_, err := git(ctx, r.run, r.Dir, patch, nil, append(args, "-")...)
	return err
}

// Discard throws away the working tree's changes of the files: tracked
// files go back to the index, untracked ones are deleted.
func (r *Repo) Discard(ctx context.Context, c Change) error {
	if c.Work == '?' {
		return r.Remove(ctx, c.Path)
	}
	_, err := r.Git(ctx, "restore", "--worktree", "--", c.Path)
	return err
}

// Stage is a version of a file in conflict, as the index holds it.
type Stage int

const (
	BaseStage   Stage = 1 // the common ancestor's
	OursStage   Stage = 2 // HEAD's
	TheirsStage Stage = 3 // the other side's
)

// ShowStage reads a version of a file in conflict, "" when that side has
// none (a file added on one side).
func (r *Repo) ShowStage(ctx context.Context, path string, s Stage) string {
	out, err := r.Git(ctx, "show", ":"+strconv.Itoa(int(s))+":"+path)
	if err != nil {
		return ""
	}
	return out
}

// Resolve writes a file's resolved text and stages it, which marks its
// conflict resolved.
func (r *Repo) Resolve(ctx context.Context, path, text string) error {
	if err := r.WriteFile(ctx, path, text); err != nil {
		return err
	}
	return r.Stage(ctx, path)
}

// TakeSide resolves a file in conflict with one side's version whole.
func (r *Repo) TakeSide(ctx context.Context, path string, s Stage) error {
	side := "--ours"
	if s == TheirsStage {
		side = "--theirs"
	}
	if _, err := r.Git(ctx, "checkout", side, "--", path); err != nil {
		return err
	}
	return r.Stage(ctx, path)
}

// Generated are those of paths .gitattributes marks generated or vendored,
// as GitHub's linguist reads them.
func (r *Repo) Generated(ctx context.Context, paths []string) map[string]bool {
	marked := map[string]bool{}
	if len(paths) == 0 {
		return marked
	}
	out, err := git(ctx, r.run, r.Dir, strings.Join(paths, "\x00")+"\x00", nil,
		"check-attr", "-z", "--stdin", "linguist-generated", "linguist-vendored")
	if err != nil {
		return marked
	}
	f := strings.Split(out, "\x00")
	for i := 0; i+2 < len(f); i += 3 {
		if v := f[i+2]; v == "set" || v == "true" {
			marked[f[i]] = true
		}
	}
	return marked
}

// Blob is a file's content at a revision, as git cat-file prints it: rev
// "" for the index's, else a commit's or tree's.
func (r *Repo) Blob(ctx context.Context, rev, path string) ([]byte, error) {
	out, err := r.Git(ctx, "cat-file", "blob", rev+":"+path)
	return []byte(out), err
}
