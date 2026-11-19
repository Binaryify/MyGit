package main

import (
	"context"
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mygit/internal/git"
	loc "mygit/locales"
)

// allChanges are a list's changes shown one after another in one scroll,
// as VS Code's multi-diff editor, rather than a file at a time (#44).
type allChanges struct {
	staged bool
	raw    string // the patches as git printed them, to keep the files as the list refreshes unchanged
	files  []allFile
}

// allFile is a file of allChanges: its diff, or why it shows none.
type allFile struct {
	change git.Change
	diff   *editor.DiffState // nil for a file shown by its header alone
	note   string            // why: binary, generated or too long
}

// showAll shows the changes of a list, the staged one or the other, one
// after another.
func (a *app) showAll(staged bool) {
	if a.detail.all == nil && a.leaving(func() { a.showAll(staged) }) {
		return
	}
	if a.detail.all == nil || a.detail.all.staged != staged {
		// Shown at once, loading, its files to come.
		a.detail = detail{gen: a.detail.gen + 1, loading: true, all: &allChanges{staged: staged, raw: "\x00"}}
		if staged {
			a.stagedSel = -1
		} else {
			a.unstagedSel = -1
		}
	}
	gen, changes, one, ws := a.detail.gen, append([]git.Change(nil), a.changes(staged)...), a.oneList(staged), prefs.whitespace()
	cur := a.detail.all
	marked, prev := a.genMarked, allChanges{staged: cur.staged, raw: cur.raw, files: make([]allFile, len(cur.files))}
	work(a, func(ctx context.Context) (allChanges, error) {
		var raw string
		var err error
		if one {
			raw, err = a.repo.DiffHead(ctx, ".", ws)
		} else {
			raw, err = a.repo.Diff(ctx, ".", staged, ws)
		}
		if err != nil {
			return allChanges{}, err
		}
		if prev.staged == staged && prev.raw == raw && len(prev.files) == len(changes) {
			return prev, nil // unchanged: the diffs shown stay, scrolled as they are
		}
		patches, _ := editor.ParsePatch(raw)
		byName := make(map[string]editor.FilePatch, len(patches))
		for _, p := range patches {
			byName[fileName(p)] = p
		}
		all := allChanges{staged: staged, raw: raw}
		for _, c := range changes {
			f := allFile{change: c}
			p, ok := byName[c.Path]
			switch {
			case c.Conflict:
				f.note = loc.AllConflict()
			case ok && p.Binary:
				f.note = loc.BinaryFile()
			case ok && isGenerated(c.Path, marked, &p, nil):
				f.note = loc.AllGenerated()
			case ok:
				f.diff = patchDiff(p)
			case c.Work == '?' && !staged: // untracked: git diff leaves it out
				text, err := a.repo.ReadFile(ctx, c.Path)
				switch {
				case err != nil:
					f.note = err.Error()
				case isBinary(text):
					f.note = loc.BinaryFile()
				case isGenerated(c.Path, marked, nil, firstLines(text, 5)):
					f.note = loc.AllGenerated()
				default:
					f.diff = textDiff(c.Path, "", text)
				}
			default:
				f.note = loc.NoDiff()
			}
			if f.diff != nil {
				f.diff.FitContent = true
			}
			all.files = append(all.files, f)
		}
		return all, nil
	}, func(all allChanges, err error) {
		if a.detail.gen != gen {
			return // another file chosen meanwhile
		}
		a.detail.loading = false
		if err != nil {
			a.errMsg = err.Error()
			return
		}
		if cur := a.detail.all; cur == nil || cur.raw != all.raw || len(cur.files) != len(all.files) {
			a.detail.all = &all
		}
	})
}

// allView is the changes one after another, each under its name, which
// shows it alone when clicked.
func (a *app) allView(c *ui.Context, all *allChanges) {
	t := c.Theme()
	if len(all.files) == 0 {
		ui.Column(c).Fill().Center().Children(func() { ui.Text(c, loc.NoDiff()).TextColor(t.TextMuted) })
		return
	}
	ui.Scroll(c.Key(fmt.Sprint("all", a.detail.gen))).Fill().Children(func() {
		for i, f := range all.files {
			ui.Column(c.Key(f.change.Path)).Children(func() {
				head := ui.ButtonBase(c.Key("head")).Height(30).Padding(0, 10).Gap(8).AlignItems(ui.Center).Justify(ui.Start).
					Background(t.Surface).BorderWidth(boolPx(i > 0), 0, 1, 0).BorderColor(t.Border).
					Cursor(ui.CursorPointer).Tooltip(loc.AllOpenFile())
				head.Children(func() {
					ui.Text(c, f.change.Path).Bold().SingleLine().Shrink(1)
					if f.diff != nil {
						added, deleted := f.diff.Stats()
						if added > 0 {
							ui.Text(c, fmt.Sprintf("+%d", added)).FontSize(11).TextColor(t.Success)
						}
						if deleted > 0 {
							ui.Text(c, fmt.Sprintf("-%d", deleted)).FontSize(11).TextColor(t.Danger)
						}
					}
				})
				if head.Clicked() {
					path := f.change.Path
					a.chooseChange(indexOf(a.changes(all.staged), path), all.staged)
				}
				if f.diff == nil {
					ui.Text(c, f.note).TextColor(t.TextMuted).Padding(8, 12)
					return
				}
				editor.DiffView(c.Key("diff"), f.diff).AlignSelf(ui.Stretch)
			})
		}
	})
}

// boolPx is a border's width: 1 where it shows, 0 where not.
func boolPx(on bool) float32 {
	if on {
		return 1
	}
	return 0
}
