package main

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mygit/internal/git"
	"mygit/internal/patch"
)

const rowHeight = 26

func (a *app) view(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().FontSize(12).Children(func() {
		a.toolbar(c)
		if a.errMsg != "" {
			ui.Row(c).Padding(6, 12).Gap(8).Background(t.Danger.Alpha(0.12)).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.errMsg).TextColor(t.Danger).Wrap().Grow(1)
				if ui.Button(c, "关闭").Clicked() {
					a.errMsg = ""
				}
			})
		}
		a.operationBar(c)
		ui.Split(c, &a.sidebarW, func() { a.leftPanel(c) }, func() {
			ui.SplitVertical(c, &a.logH, func() { a.logView(c) }, func() { a.detailView(c) }).Fill()
		}).Grow(1)
	})
	a.promptView(c)
	a.confirmView(c)
	c.OnShortcut(ui.Cmd, ui.KeyR, a.refresh)
}

func (a *app) toolbar(c *ui.Context) {
	t := c.Theme()
	st := a.status
	ui.Row(c).Gap(8).Padding(8, 12).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		ui.Text(c, a.repo.Name()).FontSize(14).Bold()
		branch := st.Branch
		if branch == "" {
			branch = "（游离 HEAD）"
		}
		ui.Button(c, "⎇ "+branch+" ▾").Label("分支").Menu(a.branchMenu)
		if st.Upstream != "" {
			ui.Text(c, fmt.Sprintf("%s  ↑%d ↓%d", st.Upstream, st.Ahead, st.Behind)).TextColor(t.TextMuted)
		}
		ui.Box(c).Grow(1)
		if a.busy != "" {
			ui.Spinner(c).Size(14, 14)
			ui.Text(c, a.busy+"…").TextColor(t.TextMuted)
		}
		idle := a.busy == ""
		if ui.Button(c, "获取").Tooltip("git fetch --all --prune").Disabled(!idle).Clicked() {
			a.run("获取", a.repo.Fetch, nil)
		}
		if ui.Button(c, "拉取").Tooltip("git pull --ff-only").Disabled(!idle || st.Upstream == "").Clicked() {
			a.run("拉取", a.repo.Pull, nil)
		}
		if ui.Button(c, "推送").Tooltip("git push").Disabled(!idle || st.Branch == "").Clicked() {
			a.run("推送", func(ctx context.Context) error { return a.repo.Push(ctx, st) }, nil)
		}
		if ui.Button(c, "刷新").Tooltip("⌘R").Clicked() {
			a.refresh()
		}
	})
}

// changesPanel is the working tree's changes, staged and not, and the
// commit message under them.
func (a *app) changesPanel(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).Basis(0).Children(func() {
		a.changeSection(c, "暂存的更改", true)
		a.changeSection(c, "更改", false)
		ui.Column(c).Gap(8).Padding(10).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
			ui.TextArea(c, &a.message).Placeholder("提交说明").Label("提交说明").Lines(3, 8)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if ui.Checkbox(c, &a.amend, "修改上次提交").Changed() {
					a.toggleAmend()
				}
				ui.Box(c).Grow(1)
				label := "提交"
				if n := len(a.staged); n > 0 {
					label = fmt.Sprintf("提交 %d 个文件", n)
				}
				if ui.PrimaryButton(c, label).Disabled(!a.canCommit()).Tooltip("⌘↩").Clicked() {
					a.commit()
				}
			})
		})
	})
	c.OnShortcut(ui.Cmd, ui.KeyEnter, a.commit)
}

func (a *app) changeSection(c *ui.Context, title string, staged bool) {
	t := c.Theme()
	changes := a.changes(staged)
	list, sel := &a.unstagedList, &a.unstagedSel
	if staged {
		list, sel = &a.stagedList, &a.stagedSel
	}
	ui.Row(c).Padding(8, 10, 4, 10).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).Bold().TextColor(t.TextMuted)
		ui.Badge(c, fmt.Sprint(len(changes)))
		ui.Box(c).Grow(1)
		if len(changes) > 0 {
			label, act := "全部暂存", a.stageAll
			if staged {
				label, act = "全部取消", a.unstageAll
			}
			if ui.Button(c, label).Disabled(a.busy != "").Clicked() {
				act()
			}
		}
	})
	l := ui.List(c.Key(title), list, len(changes), func(i int) {
		ch := changes[i]
		row := ui.Row(c).Height(24).Padding(0, 10).Gap(6).AlignItems(ui.Center)
		row.Children(func() {
			code := ch.Work
			if staged {
				code = ch.Staged
			}
			ui.Text(c, string(code)).Font("monospace").Bold().TextColor(changeColor(t, code, i == *sel)).Width(12)
			name, dir := path.Base(ch.Path), path.Dir(ch.Path)
			ui.Text(c, name).SingleLine()
			if dir != "." {
				ui.Text(c, dir).TextColor(t.TextMuted).SingleLine().Grow(1)
			} else {
				ui.Box(c).Grow(1)
			}
			if row.Hovered() {
				label := "+"
				if staged {
					label = "−"
				}
				if ui.Button(c, label).Size(22, 20).Tooltip(map[bool]string{true: "取消暂存", false: "暂存"}[staged]).Clicked() {
					a.toggleStage(ch, staged)
				}
			}
		})
		row.ContextMenu(func(m *ui.Menu) {
			if m.Item(map[bool]string{true: "取消暂存", false: "暂存"}[staged]).Chosen() {
				a.toggleStage(ch, staged)
			}
			if !staged && !ch.Conflict && m.Item("丢弃更改…").Chosen() {
				a.discard(ch)
			}
		})
	}).Grow(1).Basis(0)
	if l.Changed() && *sel >= 0 && *sel < len(changes) {
		a.showChange(changes[*sel], staged)
		// One change is chosen at a time, of both lists.
		if staged {
			a.unstagedSel = -1
		} else {
			a.stagedSel = -1
		}
	}
	if l.Submitted() && *sel >= 0 && *sel < len(changes) {
		a.toggleStage(changes[*sel], staged)
	}
}

func (a *app) toggleStage(ch git.Change, staged bool) {
	if staged {
		a.unstage(ch)
	} else {
		a.stage(ch)
	}
}

// changeColor colors a change's letter, white on a chosen row's accent.
func changeColor(t *ui.Theme, code byte, chosen bool) ui.Color {
	if chosen {
		return t.AccentText
	}
	switch code {
	case 'A', '?':
		return ui.RGB(52, 168, 83)
	case 'D':
		return t.Danger
	case 'U':
		return ui.RGB(230, 126, 34)
	}
	return t.Accent
}

// detailView is the diff of what was chosen last: a commit, with its files
// beside, or a change.
func (a *app) detailView(c *ui.Context) {
	t := c.Theme()
	d := &a.detail
	ui.Column(c).Fill().BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
		switch {
		case d.commit != nil:
			a.commitHeader(c, d.commit)
			ui.Split(c.Key("files"), &a.filesW, func() { a.filesPane(c) }, func() { a.diffView(c) }).Grow(1)
		case d.change != nil:
			ui.Row(c).Padding(8, 12).Gap(8).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
				ui.Text(c, d.change.Path).Bold().SingleLine()
				side := "工作区"
				if d.staged {
					side = "已暂存"
				}
				ui.Badge(c, side)
				ui.Box(c).Grow(1)
				a.lineButtons(c)
			})
			a.diffView(c)
		default:
			ui.Column(c).Fill().Center().Children(func() {
				ui.Text(c, "选择一个提交或更改查看差异").TextColor(t.TextMuted)
			})
		}
	})
}

func (a *app) commitHeader(c *ui.Context, cm *git.Commit) {
	t := c.Theme()
	ui.Column(c).Padding(8, 12).Gap(3).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		ui.Text(c, cm.Subject).Bold().SingleLine()
		ui.Row(c).Gap(10).Children(func() {
			ui.Text(c, cm.Short()).Font("monospace").TextColor(t.Accent).Tooltip(cm.Hash)
			ui.Text(c, cm.Author).TextColor(t.TextMuted)
			ui.Text(c, cm.Time.Format("2006-01-02 15:04")).TextColor(t.TextMuted)
		})
	})
}

func (a *app) filesPane(c *ui.Context) {
	t := c.Theme()
	files := a.detail.files
	l := ui.List(c, &a.fileList, len(files), func(i int) {
		f := files[i]
		code, name := byte('M'), f.NewName
		switch {
		case f.OldName == "":
			code = 'A'
		case f.NewName == "":
			code, name = 'D', f.OldName
		case f.OldName != f.NewName:
			code = 'R'
		}
		ui.Row(c).Height(24).Padding(0, 10).Gap(6).AlignItems(ui.Center).Tooltip(name).Children(func() {
			ui.Text(c, string(code)).Font("monospace").Bold().TextColor(changeColor(t, code, i == a.detail.fileSel)).Width(12)
			ui.Text(c, path.Base(name)).SingleLine()
			if dir := path.Dir(name); dir != "." {
				ui.Text(c, dir).TextColor(t.TextMuted).SingleLine()
			}
		})
	}).Fill()
	if l.Changed() && a.detail.fileSel >= 0 && a.detail.fileSel < len(files) {
		a.showFile(a.detail.fileSel)
	}
}

func (a *app) diffView(c *ui.Context) {
	t := c.Theme()
	d := &a.detail
	switch {
	case d.loading:
		ui.Column(c).Fill().Center().Children(func() { ui.Spinner(c).Size(18, 18) })
	case d.commit != nil && d.fileSel >= 0 && d.fileSel < len(d.files) && d.files[d.fileSel].Binary:
		ui.Column(c).Fill().Center().Children(func() { ui.Text(c, "二进制文件").TextColor(t.TextMuted) })
	case d.diff != nil:
		editor.DiffView(c.Key(d.gen*1000+d.fileSel), d.diff).Fill()
	default:
		ui.Column(c).Fill().Center().Children(func() { ui.Text(c, "没有差异").TextColor(t.TextMuted) })
	}
}

// refLabel is how a ref shows: a tag without its "tag: ".
func refLabel(ref string) (string, bool) {
	if tag, ok := strings.CutPrefix(ref, "tag: "); ok {
		return tag, true
	}
	return ref, false
}

// lineButtons act on the lines selected in the diff of a change.
func (a *app) lineButtons(c *ui.Context) {
	if a.detail.patch == nil || !patch.Partial(*a.detail.patch) {
		return
	}
	chosen := a.selectedLines()
	none := chosen == nil || a.busy != ""
	if a.detail.staged {
		if ui.Button(c, "取消暂存选中行").Disabled(none).Clicked() {
			a.applyLines(chosen, lineUnstage)
		}
		return
	}
	if ui.Button(c, "丢弃选中行").Disabled(none).Clicked() {
		a.applyLines(chosen, lineDiscard)
	}
	if ui.Button(c, "暂存选中行").Disabled(none).Tooltip("点击或拖动行号选中行").Clicked() {
		a.applyLines(chosen, lineStage)
	}
}
