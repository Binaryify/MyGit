package main

import (
	"context"
	"fmt"

	"github.com/egoist/mygo/ui"

	"mygit/internal/git"
)

// leftPanel is the working tree's changes, or the stash.
func (a *app) leftPanel(c *ui.Context) {
	ui.Column(c).Fill().Children(func() {
		ui.Box(c).Padding(8, 10, 0, 10).Children(func() {
			ui.Segmented(c, &a.leftTab, "更改", fmt.Sprintf("Stash (%d)", len(a.stashes))).FillWidth()
		})
		if a.leftTab == 1 {
			a.stashPanel(c)
		} else {
			a.changesPanel(c)
		}
	})
}

func (a *app) stashPanel(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).Basis(0).Children(func() {
		ui.Row(c).Padding(8, 10).Children(func() {
			changed := len(a.staged)+len(a.unstaged) > 0
			if ui.Button(c, "Stash 当前更改…").Disabled(!changed || a.busy != "").Grow(1).Clicked() {
				a.prompt = prompt{open: true, title: "Stash 当前更改", button: "Stash", check: "包括未跟踪的文件", checked: true,
					ok: func(msg, _ string) {
						untracked := a.prompt.checked
						a.run("Stash", func(ctx context.Context) error { return a.repo.StashPush(ctx, msg, untracked) }, nil)
					}}
				a.prompt.value = "WIP"
			}
		})
		stashes := a.stashes
		l := ui.List(c.Key("stash"), &a.stashList, len(stashes), func(i int) {
			s := stashes[i]
			row := ui.Column(c).Padding(6, 10).Gap(2)
			row.Children(func() {
				ui.Text(c, s.Subject).SingleLine()
				ui.Text(c, s.Ref+" · "+s.Time.Format("2006-01-02 15:04")).FontSize(11).TextColor(t.TextMuted)
			})
			row.ContextMenu(func(m *ui.Menu) { a.stashMenu(m, s) })
		}).Grow(1).Basis(0).Children(func() {
			if len(stashes) == 0 {
				ui.Text(c, "没有 stash").TextColor(t.TextMuted).Padding(12)
			}
		})
		if l.Changed() && a.stashSel >= 0 && a.stashSel < len(stashes) {
			a.showStash(stashes[a.stashSel])
			a.logSel = -1
		}
		if l.Submitted() && a.stashSel >= 0 && a.stashSel < len(stashes) {
			s := stashes[a.stashSel]
			a.run("应用 "+s.Ref, func(ctx context.Context) error { return a.repo.StashApply(ctx, s.Ref, false) }, nil)
		}
	})
}

func (a *app) stashMenu(m *ui.Menu, s git.Stash) {
	if m.Item("应用").Chosen() {
		a.run("应用 "+s.Ref, func(ctx context.Context) error { return a.repo.StashApply(ctx, s.Ref, false) }, nil)
	}
	if m.Item("弹出（应用并删除）").Chosen() {
		a.run("弹出 "+s.Ref, func(ctx context.Context) error { return a.repo.StashApply(ctx, s.Ref, true) }, nil)
	}
	if m.Item("从此新建分支…").Chosen() {
		a.ask1("从 "+s.Ref+" 新建分支", "创建", "", func(name string) {
			a.run("新建分支 "+name, func(ctx context.Context) error { return a.repo.StashBranch(ctx, name, s.Ref) }, nil)
		})
	}
	m.Separator()
	if m.Item("删除…").Chosen() {
		a.ask("删除 "+s.Ref+"？", s.Subject+"\n删除后无法恢复。", "删除", func() {
			a.run("删除 "+s.Ref, func(ctx context.Context) error { return a.repo.StashDrop(ctx, s.Ref) }, nil)
		})
	}
}

var operationNames = map[git.Operation]string{
	git.Merging: "合并", git.Rebasing: "变基", git.CherryPick: "cherry-pick", git.Reverting: "撤销提交",
}

// operationBar says the repository is in the middle of a merge, a rebase,
// a cherry-pick or a revert, and goes on with it or gives it up.
func (a *app) operationBar(c *ui.Context) {
	if a.op == git.NoOperation {
		return
	}
	op, name := a.op, operationNames[a.op]
	conflicts := 0
	for _, ch := range a.unstaged {
		if ch.Conflict {
			conflicts++
		}
	}
	ui.Row(c).Padding(6, 12).Gap(8).AlignItems(ui.Center).Background(ui.RGB(230, 126, 34).Alpha(0.15)).Children(func() {
		msg := "正在" + name
		if conflicts > 0 {
			msg += fmt.Sprintf("，有 %d 个冲突文件：解决后暂存，再点继续", conflicts)
		} else {
			msg += "：冲突已解决，可以继续"
		}
		ui.Text(c, msg).Grow(1).Wrap()
		idle := a.busy == ""
		if ui.Button(c, "中止…").Disabled(!idle).Clicked() {
			a.ask("中止"+name+"？", "回到"+name+"开始之前的状态，期间解决的冲突会丢失。", "中止", func() {
				a.run("中止"+name, func(ctx context.Context) error { return a.repo.Abort(ctx, op) }, nil)
			})
		}
		if op == git.Rebasing || op == git.CherryPick {
			if ui.Button(c, "跳过此提交").Disabled(!idle).Clicked() {
				a.run("跳过", func(ctx context.Context) error { return a.repo.Skip(ctx, op) }, nil)
			}
		}
		if ui.PrimaryButton(c, "继续").Disabled(!idle || conflicts > 0).Clicked() {
			a.run("继续"+name, func(ctx context.Context) error { return a.repo.Continue(ctx, op) }, nil)
		}
	})
}
