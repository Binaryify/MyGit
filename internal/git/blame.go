package git

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// BlameLine is a line of a file and the commit that last changed it.
type BlameLine struct {
	Hash    string
	Author  string
	Time    time.Time
	Summary string
	Text    string
}

// Blame says which commit last changed each line of a file at rev, HEAD
// when empty, or the working tree's file when rev is "WORKTREE".
func (r *Repo) Blame(ctx context.Context, rev, path string) ([]BlameLine, error) {
	args := []string{"blame", "--porcelain"}
	if rev != "" && rev != "WORKTREE" {
		args = append(args, rev)
	}
	out, err := r.Git(ctx, append(args, "--", path)...)
	if err != nil {
		return nil, err
	}
	return parseBlame(out), nil
}

// parseBlame reads git blame --porcelain: a header per line, "hash orig
// final [count]", the commit's fields the first time it comes, and the
// line after a tab.
func parseBlame(out string) []BlameLine {
	type info struct {
		author, summary string
		time            time.Time
	}
	commits := map[string]*info{}
	var lines []BlameLine
	var cur string
	for _, l := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(l, "\t"):
			in := commits[cur]
			lines = append(lines, BlameLine{Hash: cur, Author: in.author, Time: in.time, Summary: in.summary, Text: l[1:]})
		case len(l) >= 40 && isHex(l[:40]) && (len(l) == 40 || l[40] == ' '):
			cur = l[:40]
			if commits[cur] == nil {
				commits[cur] = &info{}
			}
		case strings.HasPrefix(l, "author "):
			commits[cur].author = l[7:]
		case strings.HasPrefix(l, "author-time "):
			sec, _ := strconv.ParseInt(l[12:], 10, 64)
			commits[cur].time = time.Unix(sec, 0)
		case strings.HasPrefix(l, "summary "):
			commits[cur].summary = l[8:]
		}
	}
	return lines
}

func isHex(s string) bool { return strings.Trim(s, "0123456789abcdef") == "" }

// NotCommitted tells whether a blamed line isn't committed yet.
func (l BlameLine) NotCommitted() bool { return strings.Trim(l.Hash, "0") == "" }
