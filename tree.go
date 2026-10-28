package main

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	loc "mygit/locales"
)

// treeRow is a row of files shown as a tree: a folder, or a file.
type treeRow struct {
	dir   string // a folder's path, "" for a file's row
	name  string // a folder's name, with those of its lone subfolders, or a file's
	depth int
	item  int // a file's index, -1 for a folder's row
}

// treeNode is a folder of the tree being laid out.
type treeNode struct {
	dirs  map[string]*treeNode
	files []int // the indices, of the items, of its files
}

// treeRows lays out items, at the paths path gives, as a tree, as VS
// Code's: in each folder its folders first, then its files, by name; a
// folder with nothing but one folder shows as one row with it; the
// contents of folders folded don't show.
func treeRows(items []int, path func(item int) string, folded func(dir string) bool) []treeRow {
	root := &treeNode{}
	for _, it := range items {
		n := root
		parts := strings.Split(path(it), "/")
		for _, p := range parts[:len(parts)-1] {
			if n.dirs == nil {
				n.dirs = map[string]*treeNode{}
			}
			if n.dirs[p] == nil {
				n.dirs[p] = &treeNode{}
			}
			n = n.dirs[p]
		}
		n.files = append(n.files, it)
	}
	var rows []treeRow
	var walk func(n *treeNode, dir string, depth int)
	walk = func(n *treeNode, dir string, depth int) {
		names := make([]string, 0, len(n.dirs))
		for name := range n.dirs {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			child, full := n.dirs[name], dir+name
			for len(child.dirs) == 1 && len(child.files) == 0 {
				for sub, grandchild := range child.dirs {
					name, full, child = name+"/"+sub, full+"/"+sub, grandchild
				}
			}
			rows = append(rows, treeRow{dir: full, name: name, depth: depth, item: -1})
			if !folded(full) {
				walk(child, full+"/", depth+1)
			}
		}
		files := slices.Clone(n.files)
		slices.SortFunc(files, func(a, b int) int { return strings.Compare(path(a), path(b)) })
		for _, it := range files {
			p := path(it)
			rows = append(rows, treeRow{name: p[strings.LastIndex(p, "/")+1:], depth: depth, item: it})
		}
	}
	walk(root, "", 0)
	return rows
}

// treeRowOf is the row of item, -1 when none shows it.
func treeRowOf(rows []treeRow, item int) int {
	if item < 0 {
		return -1
	}
	return slices.IndexFunc(rows, func(r treeRow) bool { return r.item == item })
}

// listRows are the rows a list of files shows: the items, at the paths
// path gives, as they are or as a tree when asked. It gives the tree's
// rows, nil for a list, how many rows show, and the item of each, -1 for
// a folder's.
func (a *app) listRows(list string, items []int, path func(int) string) (tree []treeRow, n int, item func(r int) int) {
	if !prefs.TreeView {
		return nil, len(items), func(r int) int { return items[r] }
	}
	tree = treeRows(items, path, func(dir string) bool { return a.folded(list, dir) })
	return tree, len(tree), func(r int) int { return tree[r].item }
}

// folded tells whether a folder of a list shows folded.
func (a *app) folded(list, dir string) bool { return a.foldedDirs[list+"\x00"+dir] }

// toggleFolder folds or unfolds a folder of a list.
func (a *app) toggleFolder(list, dir string) {
	if a.foldedDirs == nil {
		a.foldedDirs = map[string]bool{}
	}
	key := list + "\x00" + dir
	a.foldedDirs[key] = !a.foldedDirs[key]
}

// folderRow is a folder's row of a list shown as a tree: a chevron that
// says whether it is folded, and its name.
func (a *app) folderRow(c *ui.Context, list string, r treeRow) {
	t := c.Theme()
	icon := map[bool]string{true: "chevron-right", false: "chevron-down"}[a.folded(list, r.dir)]
	ui.Row(c).Height(24).Padding(0, 8, 0, 6+float32(r.depth)*treeIndent).Gap(4).AlignItems(ui.Center).
		Tooltip(r.dir).Children(func() {
		ui.Icon(c, icons[icon]).Size(12, 12).TextColor(t.TextMuted)
		ui.Icon(c, icons["folder"]).Size(13, 13).TextColor(t.TextMuted)
		ui.Text(c, r.name).SingleLine().Grow(1).Basis(0)
	})
}

// treeIndent is how far a tree's row moves in for each folder it is in.
const treeIndent = 12

// treeToggle switches the lists of files between a list and a tree.
func treeToggle(c *ui.Context) {
	icon, label := "list-tree", loc.ViewAsTree()
	if prefs.TreeView {
		icon, label = "list", loc.ViewAsList()
	}
	if iconButton(c, icon, label) {
		prefs.TreeView = !prefs.TreeView
		prefs.save()
	}
}
