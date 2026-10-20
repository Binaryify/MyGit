package git

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// Local runs git on this computer.
var Local = &local{}

// Env adds to the environment of git run in a working tree, as the app's
// askpass for its prompts.
var Env func(dir string) []string

type local struct{}

func (*local) Exec(ctx context.Context, dir string, env []string, stdin io.Reader, stdout, stderr io.Writer, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	if Env != nil {
		cmd.Env = append(cmd.Env, Env(dir)...)
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	hideWindow(cmd)
	return cmd.Run()
}

func (*local) ReadFile(_ context.Context, path string) ([]byte, error) { return os.ReadFile(path) }

func (*local) WriteFile(_ context.Context, path string, data []byte) error {
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode()
	} else if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, mode)
}

func (*local) RemoveAll(_ context.Context, path string) error { return os.RemoveAll(path) }

func (*local) Exists(_ context.Context, path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (*local) Join(dir, rel string) string { return filepath.Join(dir, filepath.FromSlash(rel)) }

func (*local) Host() string { return "" }

func (l *local) trust(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	_, err = git(context.Background(), l, abs, "", nil, "config", "--global", "--add", "safe.directory", filepath.ToSlash(abs))
	return err
}
