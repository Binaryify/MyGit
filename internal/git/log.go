package git

import (
	"bufio"
	"bytes"
	"context"
	"strconv"
	"strings"
	"time"
)

// Commit is a commit as the history lists it.
type Commit struct {
	Hash    string
	Parents []string
	Author  string
	Time    time.Time
	Subject string
	Refs    []string // branches and tags that point at it, "HEAD -> main" as "HEAD", "main"
}

// Short is the hash abbreviated, as the history shows it.
func (c *Commit) Short() string { return c.Hash[:min(len(c.Hash), 8)] }

const logFormat = "%H\x1f%P\x1f%an\x1f%at\x1f%s\x1f%D"

// Log streams the history of every branch, tag and remote, children before
// their parents, to batch in groups as git prints it, so the first
// commits show before a long history is read.
func (r *Repo) Log(ctx context.Context, batchSize int, batch func([]Commit)) error {
	cmd := command(ctx, r.Dir, "log", "--branches", "--tags", "--remotes", "HEAD",
		"--date-order", "-z", "--format="+logFormat)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	sc.Split(splitNUL)
	commits := make([]Commit, 0, batchSize)
	for sc.Scan() {
		if c, ok := parseCommit(sc.Text()); ok {
			commits = append(commits, c)
		}
		if len(commits) == batchSize {
			batch(commits)
			commits = make([]Commit, 0, batchSize)
		}
	}
	if len(commits) > 0 {
		batch(commits)
	}
	if err := cmd.Wait(); err != nil && ctx.Err() == nil {
		// An empty repository has no HEAD to list: no commits, no error.
		if strings.Contains(stderr.String(), "unknown revision") || strings.Contains(stderr.String(), "bad revision 'HEAD'") {
			return nil
		}
		return err
	}
	return ctx.Err()
}

func parseCommit(record string) (Commit, bool) {
	f := strings.Split(strings.TrimPrefix(record, "\n"), "\x1f")
	if len(f) != 6 {
		return Commit{}, false
	}
	sec, _ := strconv.ParseInt(f[3], 10, 64)
	c := Commit{Hash: f[0], Author: f[2], Time: time.Unix(sec, 0), Subject: f[4], Refs: parseRefs(f[5])}
	if f[1] != "" {
		c.Parents = strings.Split(f[1], " ")
	}
	return c, true
}

// parseRefs reads %D: "HEAD -> main, origin/main, tag: v1.0".
func parseRefs(d string) []string {
	if d == "" {
		return nil
	}
	var refs []string
	for _, r := range strings.Split(d, ", ") {
		if head, branch, ok := strings.Cut(r, " -> "); ok {
			refs = append(refs, head, branch)
			continue
		}
		refs = append(refs, r)
	}
	return refs
}

func splitNUL(data []byte, atEOF bool) (int, []byte, error) {
	if i := bytes.IndexByte(data, 0); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// Show is the patch a commit made to its first parent, or to nothing for
// a root commit.
func (r *Repo) Show(ctx context.Context, hash string) (string, error) {
	return r.Git(ctx, "show", "--format=", "--patch", "-M", "--diff-merges=first-parent", hash)
}
