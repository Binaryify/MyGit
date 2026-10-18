package main

import (
	"fmt"

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

// logView is the history: a row per commit, with its graph.
func (a *app) logView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Padding(6, 12).Gap(8).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
			ui.Text(c, "历史").Bold().TextColor(t.TextMuted)
			ui.Text(c, fmt.Sprintf("%d 个提交", len(a.commits))).TextColor(t.TextMuted)
			if a.loading {
				ui.Spinner(c).Size(12, 12)
			}
		})
		lanes := a.visibleLanes()
		l := ui.List(c.Key("log"), &a.logList, len(a.commits), func(i int) {
			a.logRow(c, &a.commits[i], &a.rows[i], lanes)
		}).Grow(1).Basis(0)
		if l.Changed() && a.logSel >= 0 && a.logSel < len(a.commits) {
			a.showCommit(a.logSel)
			a.stagedSel, a.unstagedSel = -1, -1
		}
	})
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
	ui.Row(c).Height(rowHeight).PaddingX(4).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(float32(lanes*laneWidth+4), rowHeight).Draw(func(p *ui.Painter, r ui.Rect) {
			drawRow(p, r, row)
		})
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
