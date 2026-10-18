package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"mygit/internal/git"
	"mygit/internal/graph"
)

const (
	laneWidth = 14
	maxLanes  = 24 // wider graphs are cut, so the subjects keep their room
)

// laneColors color the graph's lanes in turn.
var laneColors = []ui.Color{
	ui.RGB(66, 133, 244), ui.RGB(52, 168, 83), ui.RGB(234, 67, 53), ui.RGB(251, 188, 4),
	ui.RGB(171, 71, 188), ui.RGB(0, 172, 193), ui.RGB(255, 112, 67), ui.RGB(124, 179, 66),
}

func laneColor(i int) ui.Color { return laneColors[i%len(laneColors)] }

// logView is the history: a row per commit, with its graph, and above it
// the search and the scope.
func (a *app) logView(c *ui.Context) {
	t := c.Theme()
	a.updateMatches()
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Padding(6, 12).Gap(8).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
			ui.Text(c, "历史").Bold().TextColor(t.TextMuted)
			count := fmt.Sprintf("%d 个提交", len(a.commits))
			if a.searching() {
				count = fmt.Sprintf("%d / %d 个提交", len(a.matches), len(a.commits))
			}
			ui.Text(c, count).TextColor(t.TextMuted)
			if a.loading {
				ui.Spinner(c).Size(12, 12)
			}
			ui.Box(c).Grow(1)
			a.scopeButton(c)
			ui.SearchField(c, &a.search).Placeholder("搜索说明、作者、哈希 ⌘F").Label("搜索历史").Width(260).Bind(&a.searchBox)
		})
		if a.scope.Path != "" {
			ui.Row(c).Padding(4, 12).Gap(8).AlignItems(ui.Center).Background(t.Accent.Alpha(0.1)).Children(func() {
				ui.Text(c, "只看改动了 "+a.scope.Path+" 的提交").Grow(1)
				if ui.Button(c, "显示全部").Clicked() {
					a.setScope(git.Scope{Ref: a.scope.Ref})
				}
			})
		}
		lanes := 0
		if !a.searching() {
			lanes = a.visibleLanes()
		}
		l := ui.List(c.Key("log"), &a.logList, a.logCount(), func(i int) {
			ci := a.commitAt(i)
			a.logRow(c, &a.commits[ci], &a.rows[ci], lanes)
		}).Grow(1).Basis(0)
		if l.Changed() && a.logSel >= 0 && a.logSel < a.logCount() {
			a.stagedSel, a.unstagedSel = -1, -1
			if !a.compareChosen() {
				a.showCommit(a.commitAt(a.logSel))
			}
		}
	})
	c.OnShortcut(ui.Cmd, ui.KeyF, func() { a.searchBox.Focus() })
}

// scopeButton picks the refs the history lists.
func (a *app) scopeButton(c *ui.Context) {
	label := "全部分支"
	switch a.scope.Ref {
	case "":
	case "HEAD":
		label = "当前分支"
	default:
		label = a.scope.Ref
	}
	ui.Button(c, label+" ▾").Label("显示的分支").Menu(func(m *ui.Menu) {
		if m.Item("全部分支").Checked(a.scope.Ref == "").Chosen() {
			a.setScope(git.Scope{Path: a.scope.Path})
		}
		if m.Item("当前分支").Checked(a.scope.Ref == "HEAD").Chosen() {
			a.setScope(git.Scope{Ref: "HEAD", Path: a.scope.Path})
		}
		m.Separator()
		for _, b := range a.branches {
			if m.Item(b).Checked(a.scope.Ref == b).Chosen() {
				a.setScope(git.Scope{Ref: b, Path: a.scope.Path})
			}
		}
	})
}

func (a *app) setScope(s git.Scope) {
	if s != a.scope {
		a.scope = s
		a.loadLog()
	}
}

func (a *app) searching() bool { return strings.TrimSpace(a.search) != "" }

// logCount and commitAt map the list's rows to commits: all of them, or
// those matching the search.
func (a *app) logCount() int {
	if a.searching() {
		return len(a.matches)
	}
	return len(a.commits)
}

func (a *app) commitAt(row int) int {
	if a.searching() {
		return a.matches[row]
	}
	return row
}

// updateMatches finds the commits matching the search: their subjects or
// authors containing it, or their hashes starting with it, ignoring case.
// Commits loaded since the last search are searched on their own.
func (a *app) updateMatches() {
	q := strings.ToLower(strings.TrimSpace(a.search))
	if q != a.searchFor || a.searched > len(a.commits) {
		a.searchFor, a.searched, a.matches = q, 0, a.matches[:0]
	}
	if q == "" {
		return
	}
	for i := a.searched; i < len(a.commits); i++ {
		cm := &a.commits[i]
		if strings.HasPrefix(cm.Hash, q) || strings.Contains(strings.ToLower(cm.Subject), q) ||
			strings.Contains(strings.ToLower(cm.Author), q) {
			a.matches = append(a.matches, i)
		}
	}
	a.searched = len(a.commits)
}

// visibleLanes is the widest graph of the rows in view, which the graph
// column takes, so the subjects line up without the room a branch far
// away would take.
func (a *app) visibleLanes() int {
	first, last := a.logList.Visible()
	if last < first {
		first, last = 0, 50 // nothing shown yet
	}
	lanes := 1
	for i := max(first, 0); i <= min(last, len(a.rows)-1); i++ {
		lanes = max(lanes, a.rows[i].Width)
	}
	return min(lanes, maxLanes)
}

func (a *app) logRow(c *ui.Context, cm *git.Commit, row *graph.Row, lanes int) {
	t := c.Theme()
	head := false
	ui.Row(c).Height(rowHeight).PaddingX(4).Gap(6).AlignItems(ui.Center).ContextMenu(func(m *ui.Menu) {
		a.commitMenu(m, *cm)
	}).Children(func() {
		if lanes > 0 {
			ui.Box(c).Size(float32(lanes*laneWidth+4), rowHeight).Draw(func(p *ui.Painter, r ui.Rect) {
				drawRow(p, r, row)
			})
		}
		for _, ref := range cm.Refs {
			if ref == "HEAD" {
				head = true
				continue
			}
			name, tag := refLabel(ref)
			b := ui.Badge(c, name)
			if tag {
				b.Background(ui.RGB(251, 188, 4).Alpha(0.25))
			}
		}
		subject := ui.Text(c, cm.Subject).SingleLine().Grow(1).Basis(0)
		if head {
			subject.Bold()
		}
		ui.Text(c, cm.Author).TextColor(t.TextMuted).SingleLine().Width(120)
		ui.Text(c, cm.Time.Format("2006-01-02 15:04")).TextColor(t.TextMuted).Font("monospace").FontSize(11).Width(110)
	})
}

// drawRow paints a row of the graph: its lines, then the commit's dot.
func drawRow(p *ui.Painter, r ui.Rect, row *graph.Row) {
	x := func(lane int) float32 { return r.X + 2 + float32(lane)*laneWidth + laneWidth/2 }
	y := func(at graph.Y) float32 { return r.Y + float32(at)*r.H/2 }
	limit := float32(maxLanes) * laneWidth
	for _, l := range row.Lines {
		if l.From.Lane >= maxLanes && l.To.Lane >= maxLanes {
			continue
		}
		x0, y0, x1, y1 := x(l.From.Lane), y(l.From.Y), x(l.To.Lane), y(l.To.Y)
		x0, x1 = min(x0, r.X+limit), min(x1, r.X+limit)
		color := laneColor(l.Color)
		if x0 == x1 {
			p.Line(x0, y0, x1, y1, 1.6, color)
			continue
		}
		var path ui.Path
		mid := (y0 + y1) / 2
		path.MoveTo(x0, y0).CubeTo(x0, mid, x1, mid, x1, y1)
		p.StrokePath(&path, 1.6, color)
	}
	if row.Lane < maxLanes {
		cx, cy := x(row.Lane), y(graph.Mid)
		p.Fill(ui.Rect{X: cx - 4, Y: cy - 4, W: 8, H: 8}, laneColor(row.Color), 4)
	}
}

// compareChosen compares the two commits chosen, ⌘-clicking, the older
// to the newer, and reports whether two are.
func (a *app) compareChosen() bool {
	if a.logChosen.Len() != 2 {
		return false
	}
	var rows []int
	for i := range a.commits {
		if a.logChosen.Has(a.commits[i].Hash) {
			rows = append(rows, i)
		}
	}
	if len(rows) != 2 {
		return false
	}
	newer, older := a.commits[rows[0]], a.commits[rows[1]] // the list is newest first
	a.showCompare(fmt.Sprintf("比较 %s → %s", older.Short(), newer.Short()), older.Hash, newer.Hash)
	return true
}
