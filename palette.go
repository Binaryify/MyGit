package main

import (
	"context"
	"sort"
	"strings"
	"unicode"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mygit/internal/git"
	loc "mygit/locales"
)

// paletteItem is a row of the palette: what it says, a hint beside it (a
// folder, a shortcut), and what choosing it does.
type paletteItem struct {
	label, hint string
	run         func()
}

// palette is a list to search as one types and choose from with the
// keys, as VS Code's quick open and command palette: of the files shown
// (⌘P), or of what the window does (⌘K).
type palette struct {
	open        bool
	placeholder string
	query       string
	items       []paletteItem
	sel         int
}

func (a *app) openPalette(placeholder string, items []paletteItem) {
	a.palette = palette{open: true, placeholder: placeholder, items: items}
}

// matches are the items the query matches, the best first.
func (p *palette) matches() []paletteItem {
	q := strings.TrimSpace(p.query)
	if q == "" {
		return p.items
	}
	type scored struct {
		item  paletteItem
		score int
	}
	var found []scored
	for _, it := range p.items {
		if s, ok := fuzzy(q, it.label); ok {
			found = append(found, scored{it, s})
		}
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].score < found[j].score })
	out := make([]paletteItem, len(found))
	for i, f := range found {
		out[i] = f.item
	}
	return out
}

// fuzzy tells whether the query's letters are in s in order, ignoring
// case, and how well: lower is better, letters found together, at the
// start of words and of s's last part (a file's name) counting less.
func fuzzy(query, s string) (int, bool) {
	q, r := []rune(strings.ToLower(query)), []rune(s)
	lower := []rune(strings.ToLower(s))
	base := strings.LastIndex(s, "/") + 1
	score, last, qi := 0, -1, 0
	for i := 0; i < len(lower) && qi < len(q); i++ {
		if lower[i] != q[qi] {
			continue
		}
		switch {
		case last >= 0 && i == last+1:
			// together with the letter before: free
		case i == 0 || !unicode.IsLetter(r[i-1]) || unicode.IsUpper(r[i]) && unicode.IsLower(r[i-1]):
			score++ // a word's start
		default:
			score += 3 + min(i-last, 10)
		}
		if i < base {
			score++ // in the folders rather than the name
		}
		last = i
		qi++
	}
	if qi < len(q) {
		return 0, false
	}
	return score*100 + len(r), true
}

func (a *app) paletteView(c *ui.Context) {
	p := &a.palette
	if !p.open {
		return
	}
	t := c.Theme()
	found := p.matches()
	p.sel = max(0, min(p.sel, len(found)-1))
	choose := func(it paletteItem) {
		p.open = false
		it.run()
	}
	ui.Modal(c, &p.open, func() {
		ui.Column(c).Width(560).Gap(8).Children(func() {
			in := ui.TextInput(c, &p.query).Placeholder(p.placeholder).Label(p.placeholder).AutoFocus()
			if in.Changed() {
				p.sel = 0
			}
			in.OnShortcut(0, ui.KeyDown, func() { p.sel = min(p.sel+1, len(found)-1) })
			in.OnShortcut(0, ui.KeyUp, func() { p.sel = max(p.sel-1, 0) })
			if in.Submitted() && len(found) > 0 {
				choose(found[p.sel])
			}
			if len(found) == 0 {
				ui.Text(c, loc.PaletteEmpty()).TextColor(t.TextMuted).Padding(8)
				return
			}
			ui.Scroll(c).MaxHeight(380).Gap(1).Children(func() {
				for i, it := range found {
					row := ui.ButtonBase(c.Key(i)).Height(28).Padding(0, 10).Radius(6).Cursor(ui.CursorPointer)
					chosen := i == p.sel
					if chosen {
						row.Background(t.Accent).TextColor(t.AccentText)
					} else if row.Hovered() {
						row.Background(t.Text.Alpha(0.08))
					}
					if chosen {
						row.ScrollIntoView()
					}
					row.Children(func() {
						ui.Row(c).Fill().Gap(10).AlignItems(ui.Center).Children(func() {
							ui.Text(c, it.label).SingleLine().Shrink(0)
							muted := t.TextMuted
							if chosen {
								muted = t.AccentText.Alpha(0.75)
							}
							ui.Text(c, it.hint).FontSize(11).TextColor(muted).SingleLine().Grow(1).Basis(0).TextAlign(ui.End)
						})
					})
					if row.Clicked() {
						choose(it)
					}
				}
			})
		})
	})
}

// openFiles is ⌘P: the files of the commit shown, else the changes.
func (a *app) openFiles() {
	var items []paletteItem
	if d := a.detail; d.commit != nil {
		for i, f := range d.files {
			name := fileName(f)
			items = append(items, paletteItem{name, "", func() {
				a.detail.fileSel = i
				a.showFile(i)
			}})
		}
	} else {
		for _, staged := range []bool{true, false} {
			hint := loc.Changes()
			if staged {
				hint = loc.Staged()
			}
			for i, ch := range a.changes(staged) {
				items = append(items, paletteItem{ch.Path, hint, func() { a.chooseChange(i, staged) }})
			}
		}
	}
	a.openPalette(loc.PaletteFiles(), items)
}

// chooseChange chooses a change of a list and shows it, asking first
// when the file shown is edited and not saved; the change asked for is
// found again by its path, as saving moves the lists' rows.
func (a *app) chooseChange(i int, staged bool) {
	changes := a.changes(staged)
	if i < 0 || i >= len(changes) {
		return
	}
	ch := changes[i]
	shown := a.detail.change
	again := shown != nil && shown.Path == ch.Path && a.detail.staged == staged
	if !again && a.leaving(func() { a.chooseChange(indexOf(a.changes(staged), ch.Path), staged) }) {
		return
	}
	a.leftTab = 0
	if staged {
		a.stagedSel, a.unstagedSel = i, -1
	} else {
		a.unstagedSel, a.stagedSel = i, -1
	}
	a.showChange(ch, staged)
}

// openCommands is ⌘K: what the toolbar, the menus and the lists do, and
// the branches to switch to.
func (a *app) openCommands() {
	idle := a.busy == ""
	var items []paletteItem
	add := func(ok bool, label, hint string, run func()) {
		if ok {
			items = append(items, paletteItem{label, hint, run})
		}
	}
	add(len(a.detail.files) > 0 || len(a.staged)+len(a.unstaged) > 0, loc.GoToFile(), shortcut("P"), a.openFiles)
	add(true, loc.SearchHistory(), shortcut("F"), func() { a.searchBox.Focus() })
	add(a.canCommit(), loc.Commit(), shortcut("↩"), a.commit)
	add(idle && len(a.unstaged) > 0, loc.StageAll(), "", a.stageAll)
	add(idle && len(a.staged) > 0, loc.UnstageAll(), "", a.unstageAll)
	add(idle && len(a.staged)+len(a.unstaged) > 0, loc.StashChanges(), "", a.stashChanges)
	add(idle, loc.Fetch(), "", func() { a.run(loc.OpFetch(), a.repo.Fetch, nil) })
	add(idle && a.status.Upstream != "", loc.Pull(), "", func() {
		a.run(loc.OpPull(), func(ctx context.Context) error { return a.repo.Pull(ctx, git.PullFastForward) }, nil)
	})
	add(idle && a.status.Branch != "", loc.PushMenu(), "", a.openPush)
	add(true, loc.Refresh(), shortcut("R"), a.refresh)
	add(len(a.comments) > 0, loc.CopyComments()+" · "+loc.CommentsMenu(len(a.comments)), "", a.copyComments)
	add(idle, loc.BranchNew(), "", func() { a.newBranch("") })
	for _, b := range a.branches {
		if b != a.status.Branch {
			add(idle, loc.OpSwitchTo(b), "", func() { a.switchTo(b) })
		}
	}
	add(true, loc.PullRequestsMenu(), "", a.openPulls)
	add(true, loc.RemotesMenu(), "", a.openRemotes)
	add(true, loc.OpenRepoInEditor(), "", func() { a.openInEditor(a.repo.Dir, 0) })
	add(a.local(), loc.ShowInFolder(), "", func() { mygo.Shell.ShowItemInFolder(a.repo.Dir) })
	add(true, loc.MenuOpenRepo(), shortcut("O"), showWelcome)
	add(true, loc.MenuInstallCLI(), "", func() { go installLauncher() })
	add(true, loc.MenuUninstallCLI(), "", func() { go uninstallLauncher() })
	add(true, loc.SettingsMenu(), "", func() { a.settingsOpen = true })
	a.openPalette(loc.PaletteCommands(), items)
}
