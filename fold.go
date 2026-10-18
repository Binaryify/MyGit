package main

import (
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mygit/internal/generated"
	loc "mygit/locales"
)

// isGenerated tells whether a file is one people don't write: by its name,
// .gitattributes (marked, from git check-attr) or its first lines, those
// of the patch when it starts at the top, else head's.
func isGenerated(path string, marked map[string]bool, p *editor.FilePatch, head []string) bool {
	if generated.Name(path) || marked[path] {
		return true
	}
	if p != nil && len(p.Hunks) > 0 && p.Hunks[0].NewStart <= 1 {
		head = nil
		for _, l := range p.Hunks[0].Lines {
			if l != "" && l[0] != '-' && l[0] != '\\' {
				head = append(head, l[1:])
			}
			if len(head) == 5 {
				break
			}
		}
	}
	return generated.Header(head)
}

// firstLines are a text's first n lines.
func firstLines(text string, n int) []string {
	lines := strings.SplitN(text, "\n", n+1)
	return lines[:min(n, len(lines))]
}

// foldKey names a file whose diff was asked for though it is generated,
// in the commit (rev) shown, or among the changes when rev is "".
func foldKey(rev, path string) string { return rev + "\x00" + path }

// foldedView stands in for the diff of a generated file until it is
// asked for.
func (a *app) foldedView(c *ui.Context, p *editor.FilePatch, show func()) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Gap(8).Children(func() {
		ui.Text(c, loc.GeneratedFolded()).TextColor(t.TextMuted)
		if p != nil {
			added, deleted := patchStats(*p)
			ui.Text(c, loc.GeneratedLines(added, deleted)).FontSize(11).TextColor(t.TextMuted)
		}
		if ui.Button(c, loc.ShowDiff()).Clicked() {
			show()
		}
	})
}

// patchStats counts a patch's added and deleted lines.
func patchStats(p editor.FilePatch) (added, deleted int) {
	for _, h := range p.Hunks {
		for _, l := range h.Lines {
			switch {
			case strings.HasPrefix(l, "+"):
				added++
			case strings.HasPrefix(l, "-"):
				deleted++
			}
		}
	}
	return added, deleted
}

// viewKey names a file as reviewed in a review's scope (a commit, a
// comparison or a pull request): with what its patch is, so that a file
// changed since it was viewed shows again, as GitHub's do.
func viewKey(scope string, p editor.FilePatch) string {
	h := fnv.New64a()
	for _, hunk := range p.Hunks {
		fmt.Fprintf(h, "@%d,%d\n", hunk.OldStart, hunk.NewStart)
		for _, l := range hunk.Lines {
			h.Write([]byte(l))
			h.Write([]byte{'\n'})
		}
	}
	return fmt.Sprintf("%s\x00%s\x00%s\x00%x", scope, p.OldName, p.NewName, h.Sum64())
}

// reviewScope is what the files shown are viewed in.
func (a *app) reviewScope() string {
	if a.detail.commit != nil {
		return a.detail.commit.Hash
	}
	return ""
}

func (a *app) isViewed(i int) bool {
	return a.viewed[viewKey(a.reviewScope(), a.detail.files[i])]
}

// toggleViewed marks a file of the files shown viewed, or not, and goes on
// to the next one not viewed once it is.
func (a *app) toggleViewed(i int) {
	if i < 0 || i >= len(a.detail.files) {
		return
	}
	key := viewKey(a.reviewScope(), a.detail.files[i])
	if a.viewed[key] {
		delete(a.viewed, key)
		return
	}
	a.viewed[key] = true
	if i != a.detail.fileSel {
		return
	}
	n := len(a.detail.files)
	for j := 1; j < n; j++ {
		if k := (i + j) % n; !a.isViewed(k) {
			a.showFile(k)
			return
		}
	}
}

// viewedCount counts the files shown that are viewed.
func (a *app) viewedCount() int {
	n := 0
	for i := range a.detail.files {
		if a.isViewed(i) {
			n++
		}
	}
	return n
}
