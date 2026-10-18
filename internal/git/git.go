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
	Dir string // the top of the working tree
}

// Open finds the repository dir is in.
func Open(dir string) (*Repo, error) {
	out, err := run(context.Background(), dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("%s 不是 git 仓库", dir)
	}
	return &Repo{Dir: filepath.Clean(strings.TrimSpace(out))}, nil
}

// Name is the working tree's folder name.
func (r *Repo) Name() string { return filepath.Base(r.Dir) }

// Git runs git in the working tree and returns what it prints.
func (r *Repo) Git(ctx context.Context, args ...string) (string, error) {
	return run(ctx, r.Dir, args...)
}

// command is git with the settings every call shares: no prompts, which
// a window can't answer, and output that parses the same everywhere.
func command(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "core.quotePath=false", "-c", "color.ui=false"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
	hideWindow(cmd)
	return cmd
}

func run(ctx context.Context, dir string, args ...string) (string, error) {
	return runInput(ctx, dir, "", args...)
}

// runInput runs git with input on its standard input.
func runInput(ctx context.Context, dir, input string, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := command(ctx, dir, args...)
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
