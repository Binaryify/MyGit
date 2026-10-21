package main

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"mygit/internal/git"
	loc "mygit/locales"
)

// changeFilter narrows the lists of changes to the paths with its text in
// them, as a repository of thousands of changes needs.
type changeFilter struct {
	query string
	// The rows of each list the filter shows, by the changes' indices,
	// nil for all of them.
	staged, unstaged []int
}

// refilter works out the rows shown again, as the text or the changes
// change.
func (a *app) refilter() {
	f := &a.filter
	f.staged, f.unstaged = matching(a.staged, f.query), matching(a.unstaged, f.query)
}

// matching are the indices of the changes whose paths have query in them,
// whatever their case, or nil for no query.
func matching(changes []git.Change, query string) []int {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	rows := []int{}
	for i, c := range changes {
		if strings.Contains(strings.ToLower(c.Path), q) {
			rows = append(rows, i)
		}
	}
	return rows
}

// shownRows are the rows of the staged changes or the others the filter
// shows, nil for all.
func (a *app) shownRows(staged bool) []int {
	if staged {
		return a.filter.staged
	}
	return a.filter.unstaged
}

// shownPaths are what staging or unstaging all of a list acts on: the
// whole tree, or with a filter the changes it shows.
func (a *app) shownPaths(staged bool) []string {
	rows := a.shownRows(staged)
	if rows == nil {
		return []string{"."}
	}
	changes := a.changes(staged)
	paths := make([]string, len(rows))
	for i, r := range rows {
		paths[i] = changes[r].Path
	}
	return paths
}

// rowOf is the row a change shows in, of rows (nil for all), or -1 when
// it isn't chosen or the filter hides it.
func rowOf(rows []int, i int) int {
	if i < 0 || rows == nil {
		return i
	}
	if r, ok := slices.BinarySearch(rows, i); ok {
		return r
	}
	return -1
}

// filterField is the changes' filter: an input with a magnifier, as the
// history's search, and a button clearing it.
func (a *app) filterField(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Padding(6, 8).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		ui.Row(c).Height(26).Grow(1).Gap(6).PaddingX(8).Radius(6).AlignItems(ui.Center).
			Background(t.Surface).Border(1, t.Border).Children(func() {
			ui.Icon(c, icons["search"]).Size(13, 13).TextColor(t.TextMuted)
			if ui.TextInputBase(c, &a.filter.query).Placeholder(loc.FilterChanges()).Label(loc.FilterChanges()).
				Grow(1).Changed() {
				a.refilter()
			}
			if a.filter.query != "" && iconButton(c, "x", loc.ClearSearch()) {
				a.filter.query = ""
				a.refilter()
			}
		})
	})
}
