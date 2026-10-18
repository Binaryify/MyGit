// Package git runs the git command line for a repository: it parses what
// git prints for reading (history, status, diffs) and lets git itself do
// every change, so the repository behaves as it does in a terminal.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Repo is a working tree and its repository.
type Repo struct {
	Dir    string // the top of the working tree
	GitDir string // the repository, .git or a worktree's
}

// UnsafeError says git won't work in a repository another user owns, as
// on a shared or external disk, until it is trusted (safe.directory).
type UnsafeError struct{ Dir string }

func (e *UnsafeError) Error() string {
	return e.Dir + " 的所有者不是当前用户，git 出于安全考虑拒绝在里面运行"
}

// Open finds the repository dir is in.
func Open(dir string) (*Repo, error) {
	out, err := run(context.Background(), dir, "rev-parse", "--show-toplevel", "--absolute-git-dir")
	if err != nil && strings.Contains(err.Error(), "dubious ownership") {
		return nil, &UnsafeError{Dir: dir}
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if err != nil || len(lines) != 2 {
		return nil, fmt.Errorf("%s 不是 git 仓库", dir)
	}
	return &Repo{Dir: filepath.Clean(strings.TrimSpace(lines[0])), GitDir: filepath.Clean(strings.TrimSpace(lines[1]))}, nil
}

// Trust lets git work in a repository another user owns, for the user,
// as git's own message says to.
func Trust(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	_, err = run(context.Background(), abs, "config", "--global", "--add", "safe.directory", filepath.ToSlash(abs))
	return err
}

// Name is the working tree's folder name.
func (r *Repo) Name() string { return filepath.Base(r.Dir) }

// Git runs git in the working tree and returns what it prints.
func (r *Repo) Git(ctx context.Context, args ...string) (string, error) {
	return run(ctx, r.Dir, args...)
}

// Env adds to the environment of git run in a working tree, as the app's
// askpass for its prompts.
var Env func(dir string) []string

// command is git with the settings every call shares: no prompts, which
// a window can't answer, and output that parses the same everywhere.
func command(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "core.quotePath=false", "-c", "color.ui=false"}, args...)...)
	cmd.Dir = dir
	// No prompts and no editors, which a window can't answer: continuing a
	// merge or a rebase takes the message git proposes.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=true", "LC_ALL=C")
	if Env != nil {
		cmd.Env = append(cmd.Env, Env(dir)...)
	}
	hideWindow(cmd)
	return cmd
}

func run(ctx context.Context, dir string, args ...string) (string, error) {
	return runInput(ctx, dir, "", nil, args...)
}

// runInput runs git with input on its standard input and env added to
// its environment.
func runInput(ctx context.Context, dir, input string, env []string, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := command(ctx, dir, args...)
	cmd.Env = append(cmd.Env, env...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return stdout.String(), errors.New(msg)
		}
		return stdout.String(), err
	}
	return stdout.String(), nil
}
