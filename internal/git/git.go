// Package git runs the git command line for a repository: it parses what
// git prints for reading (history, status, diffs) and lets git itself do
// every change, so the repository behaves as it does in a terminal.
//
// Git runs where the repository is, through a Runner: on this computer, or
// on another one over SSH.
package git

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path"
	"strings"

	loc "mygit/locales"
)

// Runner runs git and reaches the files of the computer a repository is
// on. Paths are that computer's, with slashes or its own separators.
type Runner interface {
	// Exec runs git with args in dir, with env added to its environment.
	Exec(ctx context.Context, dir string, env []string, stdin io.Reader, stdout, stderr io.Writer, args ...string) error
	ReadFile(ctx context.Context, path string) ([]byte, error)
	// WriteFile writes a file, keeping the mode of the one it replaces,
	// and makes the folders it goes in.
	WriteFile(ctx context.Context, path string, data []byte) error
	RemoveAll(ctx context.Context, path string) error
	Exists(ctx context.Context, path string) bool
	// Join joins a path and a slash-separated one under it.
	Join(dir, rel string) string
	// Host is the computer, "" for this one.
	Host() string
}

// Repo is a working tree and its repository.
type Repo struct {
	Dir    string // the top of the working tree
	GitDir string // the repository, .git or a worktree's
	run    Runner
}

// UnsafeError says git won't work in a repository another user owns, as
// on a shared or external disk, until it is trusted (safe.directory).
type UnsafeError struct{ Dir string }

func (e *UnsafeError) Error() string {
	return loc.GitUnsafe(e.Dir)
}

// Open finds the repository dir is in, on this computer.
func Open(dir string) (*Repo, error) { return OpenOn(Local, dir) }

// OpenOn finds the repository dir is in, on the computer of run.
func OpenOn(run Runner, dir string) (*Repo, error) {
	out, err := git(context.Background(), run, dir, "", nil, "rev-parse", "--show-toplevel", "--absolute-git-dir")
	if err != nil && strings.Contains(err.Error(), "dubious ownership") {
		return nil, &UnsafeError{Dir: dir}
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if err != nil || len(lines) != 2 {
		return nil, errors.New(loc.GitNotRepo(dir))
	}
	return &Repo{Dir: run.Join(strings.TrimSpace(lines[0]), ""), GitDir: run.Join(strings.TrimSpace(lines[1]), ""), run: run}, nil
}

// Trust lets git work in a repository another user owns, for the user,
// as git's own message says to.
func Trust(dir string) error { return Local.trust(dir) }

// Host is the computer the repository is on, "" for this one.
func (r *Repo) Host() string { return r.run.Host() }

// Location names the repository among those of every computer.
func (r *Repo) Location() string { return Location(r.Host(), r.Dir) }

// Location names a folder on a computer, host:dir, or dir on this one.
func Location(host, dir string) string {
	if host == "" {
		return dir
	}
	return host + ":" + dir
}

// Name is the working tree's folder name.
func (r *Repo) Name() string { return path.Base(strings.ReplaceAll(r.Dir, `\`, "/")) }

// Git runs git in the working tree and returns what it prints.
func (r *Repo) Git(ctx context.Context, args ...string) (string, error) {
	return git(ctx, r.run, r.Dir, "", nil, args...)
}

// Path is the path of a file of the working tree, named with slashes, on
// the repository's computer.
func (r *Repo) Path(rel string) string { return r.run.Join(r.Dir, rel) }

// args are those every call of git shares: output that parses the same
// everywhere.
func args(a []string) []string {
	return append([]string{"-c", "core.quotePath=false", "-c", "color.ui=false"}, a...)
}

// env is what every call of git shares: no prompts and no editors, which
// a window can't answer (continuing a merge or a rebase takes the message
// git proposes), and messages in English, which the app reads.
var env = []string{"GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=true", "LC_ALL=C"}

// git runs git with input on its standard input and more added to its
// environment, and returns what it prints.
func git(ctx context.Context, run Runner, dir, input string, more []string, a ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	var stdin io.Reader
	if input != "" {
		stdin = strings.NewReader(input)
	}
	if err := run.Exec(ctx, dir, append(env[:len(env):len(env)], more...), stdin, &stdout, &stderr, args(a)...); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return stdout.String(), errors.New(msg)
		}
		return stdout.String(), err
	}
	return stdout.String(), nil
}

// stream runs git, writing what it prints to stdout and stderr as it does.
func stream(ctx context.Context, run Runner, dir string, stdout, stderr io.Writer, a ...string) error {
	return run.Exec(ctx, dir, env, nil, stdout, stderr, args(a)...)
}
