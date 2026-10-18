package main

import (
	"context"
	"fmt"

	"github.com/egoist/mygo/ui"

	"mygit/internal/git"
)

// blame is a file's lines, each with the commit that last changed it.
type blame struct {
	path, rev string
	lines     []git.BlameLine
	list      ui.ListState
	sel       int
}

// showBlame shows who changed each line of a file at rev, last.
func (a *app) showBlame(rev, file string) {
	keep := a.detail
	a.detail.gen++
	gen := a.detail.gen
	a.detail.loading = true
	work(a, func(ctx context.Context) ([]git.BlameLine, error) { return a.repo.Blame(ctx, rev, file) },
		func(lines []git.BlameLine, err error) {
			if gen != a.detail.gen {
				return
			}
			a.detail.loading = false
			if err != nil {
				a.errMsg = err.Error()
				return
			}
			keep.gen, keep.loading = gen, false
			keep.blame = &blame{path: file, rev: rev, lines: lines, sel: -1}
			keep.blame.list.Selected = &keep.blame.sel
			a.detail = keep
		})
}

func (a *app) blameView(c *ui.Context, b *blame) {
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Padding(6, 12).Gap(8).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
			ui.Text(c, "Blame").Bold()
			ui.Text(c, b.path).SingleLine().Grow(1)
			ui.Text(c, "双击一行，在历史里定位它的提交").TextColor(t.TextMuted)
			if ui.Button(c, "关闭").Clicked() {
				a.detail.blame = nil
			}
		})
		l := ui.List(c.Key("blame"), &b.list, len(b.lines), func(i int) {
			ln := b.lines[i]
			ui.Row(c).Height(20).Gap(8).PaddingX(8).AlignItems(ui.Center).Children(func() {
				who, color := ln.Hash[:8], laneColor(int(ln.Hash[0])+int(ln.Hash[1]))
				if ln.NotCommitted() {
					who, color = "未提交", t.TextMuted
				}
				// A commit's lines show its hash on their first line only.
				first := i == 0 || b.lines[i-1].Hash != ln.Hash
				ui.Box(c).Size(3, 20).Background(color)
				ui.Text(c, map[bool]string{true: who, false: ""}[first]).Font("monospace").FontSize(11).TextColor(color).Width(64)
				ui.Text(c, map[bool]string{true: ln.Author, false: ""}[first]).FontSize(11).TextColor(t.TextMuted).SingleLine().Width(110)
				ui.Text(c, map[bool]string{true: ln.Time.Format("2006-01-02"), false: ""}[first]).FontSize(11).TextColor(t.TextMuted).Width(72)
				ui.Text(c, fmt.Sprint(i+1)).Font("monospace").FontSize(11).TextColor(t.TextMuted).Width(40).TextAlign(ui.End)
				ui.Text(c, ln.Text).Font("monospace").FontSize(12).SingleLine().Grow(1).Basis(0)
			}).Tooltip(ln.Summary)
		}).Grow(1).Basis(0)
		if l.Submitted() && b.sel >= 0 && b.sel < len(b.lines) {
			a.revealCommit(b.lines[b.sel].Hash)
		}
	})
}

// revealCommit chooses a commit in the history and scrolls to it.
func (a *app) revealCommit(hash string) {
	a.search = ""
	for i := range a.commits {
		if a.commits[i].Hash == hash {
			a.logSel = i
			a.logList.ScrollTo(i, ui.Center)
			a.showCommit(i)
			return
		}
	}
	a.errMsg = "历史里找不到 " + hash[:8] + "（可能被当前的分支筛选隐藏了）"
}

// pushPlan is the push dialog: the commits a push would send.
type pushPlan struct {
	open    bool
	loading bool
	commits []git.Commit
	force   bool
}

func (a *app) openPush() {
	st := a.status
	a.push = pushPlan{open: true, loading: true}
	work(a, func(ctx context.Context) ([]git.Commit, error) { return a.repo.Outgoing(ctx, st) },
		func(commits []git.Commit, err error) {
			a.push.loading = false
			a.push.commits = commits
			if err != nil {
				a.errMsg = err.Error()
			}
		})
}

func (a *app) pushView(c *ui.Context) {
	p := &a.push
	if !p.open {
		return
	}
	t := c.Theme()
	st := a.status
	ui.Modal(c, &p.open, func() {
		ui.Column(c).Gap(10).Width(560).Children(func() {
			target := st.Upstream
			if target == "" {
				target = "origin/" + st.Branch + "（新建）"
			}
			ui.Text(c, "推送 "+st.Branch+" → "+target).Bold().FontSize(15)
			switch {
			case p.loading:
				ui.Spinner(c).Size(16, 16)
			case len(p.commits) == 0:
				ui.Text(c, "没有要推送的提交").TextColor(t.TextMuted)
			default:
				ui.Text(c, fmt.Sprintf("%d 个提交", len(p.commits))).TextColor(t.TextMuted)
				ui.List(c.Key("outgoing"), nil, len(p.commits), func(i int) {
					cm := p.commits[i]
					ui.Row(c).Gap(8).Padding(4, 0).Children(func() {
						ui.Text(c, cm.Short()).Font("monospace").TextColor(t.Accent)
						ui.Text(c, cm.Subject).SingleLine().Grow(1).Basis(0)
						ui.Text(c, cm.Author).TextColor(t.TextMuted)
					})
				}).MaxHeight(280)
			}
			ui.Checkbox(c, &p.force, "强制推送（--force-with-lease，远程分支没被别人更新时才覆盖）")
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "推送标签").Tooltip("git push origin --tags").Clicked() {
					p.open = false
					a.run("推送标签", a.repo.PushTags, nil)
				}
				ui.Box(c).Grow(1)
				if ui.Button(c, "取消").Clicked() {
					p.open = false
				}
				label := "推送"
				if p.force {
					label = "强制推送"
				}
				if ui.PrimaryButton(c, label).Disabled(p.loading || a.busy != "").Clicked() {
					p.open = false
					force := p.force
					a.run(label, func(ctx context.Context) error { return a.repo.Push(ctx, st, force) }, nil)
				}
			})
		})
	})
}
