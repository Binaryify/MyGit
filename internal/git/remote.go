package git

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"path"
	"strings"
)

// Remote is a remote repository and where it is.
type Remote struct {
	Name, URL string
}

// Remotes lists the remotes, with their fetch URLs.
func (r *Repo) Remotes(ctx context.Context) ([]Remote, error) {
	out, err := r.Git(ctx, "remote", "-v")
	if err != nil {
		return nil, err
	}
	var list []Remote
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) == 3 && f[2] == "(fetch)" {
			list = append(list, Remote{f[0], f[1]})
		}
	}
	return list, nil
}

func (r *Repo) AddRemote(ctx context.Context, name, url string) error {
	_, err := r.Git(ctx, "remote", "add", name, url)
	return err
}

func (r *Repo) RemoveRemote(ctx context.Context, name string) error {
	_, err := r.Git(ctx, "remote", "remove", name)
	return err
}

func (r *Repo) RenameRemote(ctx context.Context, from, to string) error {
	_, err := r.Git(ctx, "remote", "rename", from, to)
	return err
}

func (r *Repo) SetRemoteURL(ctx context.Context, name, url string) error {
	_, err := r.Git(ctx, "remote", "set-url", name, url)
	return err
}

// FetchRemote updates one remote's branches.
func (r *Repo) FetchRemote(ctx context.Context, name string) error {
	_, err := r.Git(ctx, "fetch", "--prune", name)
	return err
}

// RepoName is the folder a clone of url makes, as git names it.
func RepoName(url string) string {
	url = strings.TrimSuffix(strings.TrimRight(url, "/"), ".git")
	if i := strings.LastIndexAny(url, "/:"); i >= 0 {
		url = url[i+1:]
	}
	return path.Base(url)
}

// Clone clones url into dir, a folder in parent, telling progress the
// lines git reports as it goes.
func Clone(ctx context.Context, parent, url, dir string, progress func(string)) error {
	pipe, w := io.Pipe()
	done := make(chan error, 1)
	go func() {
		err := stream(ctx, Local, parent, io.Discard, w, "clone", "--progress", url, dir)
		w.Close()
		done <- err
	}()
	var stderr bytes.Buffer
	sc := bufio.NewScanner(pipe)
	sc.Split(scanProgress)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		stderr.WriteString(line + "\n")
		if line != "" && progress != nil {
			progress(line)
		}
	}
	_, _ = io.Copy(io.Discard, pipe)
	if err := <-done; err != nil {
		return lastError(stderr.String(), err)
	}
	return nil
}

// scanProgress splits git's progress at \r as well as \n, as it rewrites
// a line in place.
func scanProgress(data []byte, atEOF bool) (int, []byte, error) {
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// lastError is the fatal line of git's output, or err.
func lastError(out string, err error) error {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(lines[i], "fatal:") || strings.HasPrefix(lines[i], "error:") {
			return textErr(lines[i])
		}
	}
	return err
}

type textErr string

func (e textErr) Error() string { return string(e) }

// Init makes a repository in dir, with main as its first branch.
func Init(ctx context.Context, dir string) error {
	_, err := git(ctx, Local, dir, "", nil, "init", "-b", "main")
	return err
}
