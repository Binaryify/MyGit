package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Change is a file that differs from HEAD, in the index, the working tree
// or both.
type Change struct {
	Path      string
	OldPath   string // where a renamed file was
	Staged    byte   // the index against HEAD: 'M', 'A', 'D', 'R', … or 0 for none
	Work      byte   // the working tree against the index, '?' untracked, 0 for none
	Conflict  bool
	Submodule bool // a submodule's folder, its commit or its working tree changed
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
				c := change(p[1], p[8])
				c.Submodule = p[2][0] == 'S'
				s.Changes = append(s.Changes, c)
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
// the working tree against the index; leaving out the whitespace changes
// asked.
func (r *Repo) Diff(ctx context.Context, path string, staged bool, ws Whitespace) (string, error) {
	args := []string{"diff", "-M", "--no-ext-diff"}
	if staged {
		args = append(args, "--cached")
	}
	return r.Git(ctx, ws.with(args, "--", path)...)
}

// ReadIndex reads a file as the index holds it, "" for one not in it.
func (r *Repo) ReadIndex(ctx context.Context, path string) string {
	out, err := r.Git(ctx, "show", ":"+path)
	if err != nil {
		return ""
	}
	return out
}

// ReadHead reads a file as HEAD has it, "" for one not in it: the one
// list shows files against HEAD, as IDEA's commit window does.
func (r *Repo) ReadHead(ctx context.Context, path string) string {
	out, err := r.Git(ctx, "show", "HEAD:"+path)
	if err != nil {
		return ""
	}
	return out
}

// DiffHead is a file's change against HEAD, in the index or the working
// tree, as the one list shows it.
func (r *Repo) DiffHead(ctx context.Context, path string, ws Whitespace) (string, error) {
	return r.Git(ctx, ws.with([]string{"diff", "-M", "--no-ext-diff", r.Base(ctx), "--"}, path)...)
}

// Base is what the one list's changes are against: HEAD, or before the
// first commit the empty tree, which git knows without its being written.
func (r *Repo) Base(ctx context.Context) string {
	if _, err := r.Git(ctx, "rev-parse", "--verify", "-q", "HEAD"); err == nil {
		return "HEAD"
	}
	if t, err := r.Git(ctx, "hash-object", "-t", "tree", "--stdin"); err == nil && strings.TrimSpace(t) != "" {
		return strings.TrimSpace(t)
	}
	return "HEAD"
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

// Remove deletes files or folders of the working tree.
func (r *Repo) Remove(ctx context.Context, paths ...string) error {
	for _, p := range paths {
		if err := r.run.RemoveAll(ctx, r.Path(p)); err != nil {
			return err
		}
	}
	return nil
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

// CommitPaths commits only the files at paths, as IntelliJ commits the
// checked ones: their working tree content, which the index need not hold,
// leaving other staged changes for another commit.
func (r *Repo) CommitPaths(ctx context.Context, message string, amend bool, paths []string) error {
	_, err := r.Git(ctx, append(append(commitArgs(message, amend), "--"), paths...)...)
	return err
}

// commitArgs are git commit's, the message's lines ending at \n however
// they were typed or pasted, as git expects them (#28).
func commitArgs(message string, amend bool) []string {
	message = strings.ReplaceAll(strings.ReplaceAll(message, "\r\n", "\n"), "\r", "\n")
	args := []string{"commit", "-m", message}
	if amend {
		args = append(args, "--amend")
	}
	return args
}

// Commit commits the index.
func (r *Repo) Commit(ctx context.Context, message string, amend bool) error {
	_, err := r.Git(ctx, commitArgs(message, amend)...)
	return err
}

// LastMessage is HEAD's message, which amending starts from.
func (r *Repo) LastMessage(ctx context.Context) string {
	return r.Message(ctx, "HEAD")
}

// RecentMessages are the messages of HEAD's last n commits, newest first
// and each once, which Up in the message box brings back.
func (r *Repo) RecentMessages(ctx context.Context, n int) []string {
	out, _ := r.Git(ctx, "log", "-"+strconv.Itoa(n), "--format=%B%x00")
	var msgs []string
	for _, m := range strings.Split(out, "\x00") {
		if m = strings.TrimSpace(m); m != "" && !slices.Contains(msgs, m) {
			msgs = append(msgs, m)
		}
	}
	return msgs
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
	return r.Restore(ctx, c.Path)
}

// Restore brings files of the working tree back to the index, throwing
// away the changes they have.
func (r *Repo) Restore(ctx context.Context, paths ...string) error {
	return inBatches(paths, func(batch []string) error {
		_, err := r.Git(ctx, append([]string{"restore", "--worktree", "--"}, batch...)...)
		return err
	})
}

// RestoreHead brings files back to HEAD, in the index and the working
// tree: what the one list discards, its changes being against HEAD.
func (r *Repo) RestoreHead(ctx context.Context, paths ...string) error {
	return inBatches(paths, func(batch []string) error {
		_, err := r.Git(ctx, append([]string{"restore", "--source=HEAD", "--staged", "--worktree", "--"}, batch...)...)
		return err
	})
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

// Stages reports which versions of a file in conflict the index holds:
// a side that deleted the file has none.
func (r *Repo) Stages(ctx context.Context, path string) (has [4]bool, err error) {
	out, err := r.Git(ctx, "ls-files", "-u", "-z", "--", path)
	if err != nil {
		return has, err
	}
	// <mode> <hash> <stage>\t<path>
	for _, entry := range strings.Split(out, "\x00") {
		if f := strings.Fields(strings.SplitN(entry, "\t", 2)[0]); len(f) == 3 {
			if n, err := strconv.Atoi(f[2]); err == nil && n > 0 && n < 4 {
				has[n] = true
			}
		}
	}
	return has, nil
}

// Resolve writes a file's resolved text and stages it, which marks its
// conflict resolved.
func (r *Repo) Resolve(ctx context.Context, path, text string) error {
	if err := r.WriteFile(ctx, path, text); err != nil {
		return err
	}
	return r.Stage(ctx, path)
}

// RestoreFrom writes a file of the working tree as rev has it, leaving the
// index as it is: IntelliJ's Get takes a file from a branch this way. The
// file has to be at rev; those not there have nothing to take.
func (r *Repo) RestoreFrom(ctx context.Context, rev, path string) error {
	_, err := r.Git(ctx, "restore", "--source="+rev, "--worktree", "--", path)
	return err
}

// TakeSide resolves a file in conflict with one side's version whole,
// deleting it when that side deleted it.
func (r *Repo) TakeSide(ctx context.Context, path string, s Stage) error {
	if has, err := r.Stages(ctx, path); err == nil && !has[s] {
		_, err := r.Git(ctx, "rm", "-q", "--", path)
		return err
	}
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

// Blobs reads files' contents at revisions, named as git cat-file names
// them ("rev:path", ":path" for the index's version), in one call of git:
// nil for those git has none of.
func (r *Repo) Blobs(ctx context.Context, specs []string) ([][]byte, error) {
	var stdout, stderr bytes.Buffer
	in := strings.NewReader(strings.Join(specs, "\n") + "\n")
	if err := r.run.Exec(ctx, r.Dir, env, in, &stdout, &stderr, args([]string{"cat-file", "--batch"})...); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, errors.New(msg)
		}
		return nil, err
	}
	return parseBlobs(stdout.Bytes(), len(specs))
}

// BlobSizes is the sizes of the objects Blobs reads, -1 for those git has
// none of, in one call of it: what to read is chosen by its size before it
// is read.
func (r *Repo) BlobSizes(ctx context.Context, specs []string) ([]int64, error) {
	var stdout, stderr bytes.Buffer
	in := strings.NewReader(strings.Join(specs, "\n") + "\n")
	if err := r.run.Exec(ctx, r.Dir, env, in, &stdout, &stderr, args([]string{"cat-file", "--batch-check"})...); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, errors.New(msg)
		}
		return nil, err
	}
	sizes := make([]int64, 0, len(specs))
	for _, line := range strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == "missing" {
			sizes = append(sizes, -1)
			continue
		}
		if len(fields) != 3 {
			return nil, fmt.Errorf("cat-file: %q", line)
		}
		size, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("cat-file: %q", line)
		}
		sizes = append(sizes, size)
	}
	if len(sizes) != len(specs) {
		return nil, fmt.Errorf("cat-file: %d sizes for %d names", len(sizes), len(specs))
	}
	return sizes, nil
}

// parseBlobs reads what cat-file --batch printed for n names: a line of the
// object's hash, kind and size, its content, then the next.
func parseBlobs(b []byte, n int) ([][]byte, error) {
	out := make([][]byte, 0, n)
	for range n {
		i := bytes.IndexByte(b, '\n')
		if i < 0 {
			return out, errors.New("cat-file: the output ended before its names did")
		}
		head := string(b[:i])
		b = b[i+1:]
		fields := strings.Fields(head)
		if len(fields) == 2 && fields[1] == "missing" {
			out = append(out, nil)
			continue
		}
		if len(fields) != 3 {
			return out, fmt.Errorf("cat-file: %q", head)
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || size < 0 || size > len(b) {
			return out, fmt.Errorf("cat-file: %q", head)
		}
		out = append(out, b[:size])
		b = b[min(size+1, len(b)):] // the newline after the content
	}
	return out, nil
}
