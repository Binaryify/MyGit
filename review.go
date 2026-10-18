package main

import (
	"fmt"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mygit/internal/patch"
	loc "mygit/locales"
)

// comment is a remark on lines of a file of a diff: kept to hand to a
// colleague or an agent as Markdown, or sent with a pull request's review.
type comment struct {
	Path       string
	Rev        string // the commit's, or "" for the working tree's changes
	Side       editor.DiffSide
	Start, End int      // the lines, of the file's side, from 1
	Code       []string // the lines, as the diff shows them
	Body       string
}

// lines says a comment's lines, as "12" or "12-14".
func (c comment) lines() string {
	if c.Start == c.End {
		return fmt.Sprint(c.Start)
	}
	return fmt.Sprintf("%d-%d", c.Start, c.End)
}

// composer is the dialog a comment is written in.
type composer struct {
	open    bool
	draft   comment
	editing int // the comment edited, or -1 for a new one
}

// diffFile is the file of the diff shown and its revision: a commit's file,
// or a change's ("" for the working tree, the index when staged).
func (a *app) diffFile() (name, rev string, ok bool) {
	d := a.detail
	switch {
	case d.commit != nil && d.fileSel >= 0 && d.fileSel < len(d.files):
		return fileName(d.files[d.fileSel]), d.commit.Hash, true
	case d.change != nil:
		return d.change.Path, "", true
	}
	return "", "", false
}

// diffPatch is the patch the diff shown is of, when its lines are a
// patch's rather than whole files'.
func (a *app) diffPatch() *editor.FilePatch {
	d := a.detail
	switch {
	case d.commit != nil && d.fileSel >= 0 && d.fileSel < len(d.files):
		return &d.files[d.fileSel]
	case d.change != nil && !d.whole:
		return d.patch
	}
	return nil
}

// fileLine is the line of the file (from 1) a line of the diff (from 0)
// is, and toDiff the other way.
func (a *app) fileLine(side editor.DiffSide, i int) int {
	p := a.diffPatch()
	if p == nil {
		return i + 1
	}
	old, new := patch.Lines(*p)
	lines := new
	if side == editor.DiffOld {
		lines = old
	}
	if i < 0 || i >= len(lines) {
		return i + 1
	}
	return patch.Number(*p, lines[i], side == editor.DiffOld)
}

func (a *app) toDiff(side editor.DiffSide, n int) (int, bool) {
	p := a.diffPatch()
	if p == nil {
		return n - 1, n >= 1
	}
	old, new := patch.Lines(*p)
	lines := new
	if side == editor.DiffOld {
		lines = old
	}
	for i, l := range lines {
		if patch.Number(*p, l, side == editor.DiffOld) == n {
			return i, true
		}
	}
	return 0, false
}

// startComment opens the composer on lines of the diff shown: those
// selected when the line is among them, else the line.
func (a *app) startComment(r editor.DiffLineRange) {
	d := a.detail.diff
	name, rev, ok := a.diffFile()
	if d == nil || !ok {
		return
	}
	if sel, ok := d.SelectedLines(); ok && sel.Side == r.Side && sel.Start <= r.Start && r.End <= sel.End {
		r = sel
	}
	buf := d.New
	if r.Side == editor.DiffOld {
		buf = d.Old
	}
	text := strings.Split(buf.String(), "\n")
	var code []string
	for i := r.Start; i <= r.End && i < len(text); i++ {
		code = append(code, text[i])
	}
	a.composer = composer{open: true, editing: -1, draft: comment{
		Path: name, Rev: rev, Side: r.Side, Code: code,
		Start: a.fileLine(r.Side, r.Start), End: a.fileLine(r.Side, r.End),
	}}
}

func (a *app) composerView(c *ui.Context) {
	p := &a.composer
	ui.Modal(c, &p.open, func() {
		ui.Column(c).Gap(12).Width(480).Children(func() {
			ui.Text(c, loc.CommentTitle(path.Base(p.draft.Path), p.draft.lines())).Bold().FontSize(14)
			ui.TextArea(c, &p.draft.Body).Placeholder(loc.CommentPlaceholder()).Label(loc.CommentPlaceholder()).Lines(4, 12).AutoFocus()
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, loc.Cancel()).Clicked() {
					p.open = false
				}
				ok := strings.TrimSpace(p.draft.Body) != ""
				label := loc.AddComment()
				if p.editing >= 0 {
					label = loc.Save()
				}
				if ui.PrimaryButton(c, label).Disabled(!ok).Tooltip(shortcut("↩")).Clicked() && ok {
					a.saveComment()
				}
			})
		})
		c.OnShortcut(ui.Cmd, ui.KeyEnter, func() {
			if strings.TrimSpace(p.draft.Body) != "" {
				a.saveComment()
			}
		})
	})
}

func (a *app) saveComment() {
	p := &a.composer
	p.open = false
	p.draft.Body = strings.TrimSpace(p.draft.Body)
	if p.editing >= 0 && p.editing < len(a.comments) {
		a.comments[p.editing].Body = p.draft.Body
		return
	}
	a.comments = append(a.comments, p.draft)
}

// commentKey names a comment's room under its line in the diff.
type commentKey int

// reviewDiff lets the diff shown be commented on, with a + by the lines'
// numbers, and shows its comments under their lines.
func (a *app) reviewDiff(c *ui.Context, d *editor.DiffState) {
	name, rev, ok := a.diffFile()
	if !ok {
		return
	}
	d.LineSelection = true
	d.OnGutterButton = a.startComment
	d.Annotations = d.Annotations[:0]
	for i, cm := range a.comments {
		if cm.Path != name || cm.Rev != rev {
			continue
		}
		if line, ok := a.toDiff(cm.Side, cm.End); ok {
			d.Annotations = append(d.Annotations, editor.DiffAnnotation{Side: cm.Side, Line: line, Height: noteHeight(cm.Body, noteCols(d)), Key: commentKey(i)})
		}
	}
	d.RenderAnnotation = func(c *ui.Context, an editor.DiffAnnotation) {
		if k, ok := an.Key.(commentKey); ok && int(k) < len(a.comments) {
			a.commentCard(c, int(k))
		}
	}
}

// noteHeight is about how high a note of body shows under a line: its
// lines wrapped at cols columns, a wide letter as two, with the note's
// header and padding.
func noteHeight(body string, cols int) float32 {
	lines := 0
	for _, l := range strings.Split(body, "\n") {
		n := 0
		for _, r := range l {
			n++
			if utf8.RuneLen(r) > 2 {
				n++
			}
		}
		lines += 1 + n/cols
	}
	return float32(56 + 18*lines)
}

// noteCols is about how many columns a note's text has under a line: a
// side's half of the diff, or all of it shown one over the other.
func noteCols(d *editor.DiffState) int {
	if d.Layout == editor.DiffUnified {
		return 90
	}
	return 44
}

// commentCard is a comment as it shows under its line.
func (a *app) commentCard(c *ui.Context, i int) {
	t := c.Theme()
	cm := a.comments[i]
	ui.Column(c).Fill().Padding(6, 12).Children(func() {
		ui.Column(c).Grow(1).Padding(8, 10).Gap(4).Radius(8).Background(t.Surface).BorderWidth(1).BorderColor(t.Accent.Alpha(0.5)).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, loc.CommentYours()).Bold().FontSize(11)
				ui.Text(c, loc.CommentLines(cm.lines())).FontSize(11).TextColor(t.TextMuted)
				ui.Box(c).Grow(1)
				if iconButton(c.Key(i), "edit", loc.Edit()) {
					a.composer = composer{open: true, draft: cm, editing: i}
				}
				if iconButton(c.Key(i), "x", loc.Delete()) {
					a.comments = append(a.comments[:i:i], a.comments[i+1:]...)
				}
			})
			ui.Text(c, cm.Body).Wrap()
		})
	})
}

// commentsMarkdown is the comments as Markdown, each with the lines it is
// on, for an agent or a colleague to address.
func commentsMarkdown(comments []comment) string {
	var b strings.Builder
	b.WriteString("# Address these Review Comments\n")
	for i, cm := range comments {
		fmt.Fprintf(&b, "\n## %d. %s:%s", i+1, cm.Path, cm.lines())
		if cm.Rev != "" && !strings.Contains(cm.Rev, "..") {
			fmt.Fprintf(&b, " (commit %s)", cm.Rev[:min(8, len(cm.Rev))])
		}
		if cm.Side == editor.DiffOld {
			b.WriteString(" (removed lines)")
		}
		lang := strings.TrimPrefix(path.Ext(cm.Path), ".")
		fmt.Fprintf(&b, "\n\n```%s\n%s\n```\n\n%s\n", lang, strings.Join(cm.Code, "\n"), cm.Body)
	}
	return b.String()
}

// copyComments puts the comments on the clipboard as Markdown.
func (a *app) copyComments() {
	if len(a.comments) == 0 {
		return
	}
	mygo.Clipboard.WriteText(commentsMarkdown(a.comments))
	a.notice = loc.CommentsCopied(len(a.comments))
}

// commentsButton is the toolbar's, while there are comments: to copy them
// or throw them away.
func (a *app) commentsButton(c *ui.Context) {
	if len(a.comments) == 0 {
		return
	}
	ui.Button(c, loc.CommentsMenu(len(a.comments))+" ▾").Label(loc.CommentsMenu(len(a.comments))).Menu(func(m *ui.Menu) {
		if m.Item(loc.CopyComments()).Chosen() {
			a.copyComments()
		}
		if m.Item(loc.ClearComments()).Chosen() {
			a.ask(loc.ClearCommentsTitle(), loc.ClearCommentsMessage(len(a.comments)), loc.Delete(), func() { a.comments = nil })
		}
	})
}
