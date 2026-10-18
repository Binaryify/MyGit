package main

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mygit/internal/git"
	"mygit/internal/patch"
)

const rowHeight = 26

func (a *app) view(c *ui.Context) {
	t := c.Theme()
	root := ui.Column(c).Fill().FontSize(12)
	root.Children(func() {
		a.toolbar(c)
		if a.errMsg != "" {
			ui.Row(c).Padding(6, 12).Gap(8).Background(t.Danger.Alpha(0.12)).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.errMsg).TextColor(t.Danger).Wrap().Grow(1)
				if a.trustDir != "" && ui.Button(c, "信任并打开").Tooltip("写入 git 的 safe.directory 设置").Clicked() {
					if err := git.Trust(a.trustDir); err != nil {
						a.errMsg = err.Error()
					} else {
						a.openDropped([]string{a.trustDir})
					}
				}
				if ui.Button(c, "关闭").Clicked() {
					a.errMsg, a.trustDir = "", ""
				}
			})
		}
		a.operationBar(c)
		ui.Split(c, &a.sidebarW, func() { a.leftPanel(c) }, func() {
			ui.SplitVertical(c, &a.logH, func() { a.logView(c) }, func() { a.detailView(c) }).Fill()
		}).Grow(1)
		if root.FileDragOver() {
			dropHint(c)
		}
	})
	if paths := root.DroppedFiles(); len(paths) > 0 {
		a.openDropped(paths)
	}
	a.rebaseView(c)
	a.pushView(c)
	a.promptView(c)
	a.remotesView(c)
	a.settingsView(c)
	a.confirmView(c)
	a.cred.view(c)
	c.OnShortcut(ui.Cmd, ui.KeyR, a.refresh)
}

func (a *app) toolbar(c *ui.Context) {
	t := c.Theme()
	st := a.status
	titleBar(c).Background(t.Surface).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		ui.Image(c, icon).Size(22, 22)
		ui.Text(c, a.repo.Name()).FontSize(13).Bold().Tooltip(a.repo.Dir)
		branch := st.Branch
		if branch == "" {
			branch = "（游离 HEAD）"
		}
		a.branchesButton(c, "⎇ "+branch+" ▾")
		if st.Upstream != "" {
			ui.Text(c, fmt.Sprintf("%s  ↑%d ↓%d", st.Upstream, st.Ahead, st.Behind)).TextColor(t.TextMuted)
		}
		ui.Box(c).Grow(1)
		if a.busy != "" {
			ui.Spinner(c).Size(14, 14)
			ui.Text(c, a.busy+"…").TextColor(t.TextMuted)
		}
		idle := a.busy == ""
		ui.Button(c, "获取 ▾").Label("获取").Disabled(!idle).Menu(func(m *ui.Menu) {
			if m.Item("获取全部远程").Chosen() {
				a.run("获取", a.repo.Fetch, nil)
			}
			if names := a.remoteNames(); len(names) > 1 {
				m.Separator()
				for _, name := range names {
					if m.Item("获取 " + name).Chosen() {
						a.run("获取 "+name, func(ctx context.Context) error { return a.repo.FetchRemote(ctx, name) }, nil)
					}
				}
			}
		})
		ui.Button(c, "拉取 ▾").Label("拉取").Disabled(!idle || st.Upstream == "").Menu(func(m *ui.Menu) {
			pull := func(mode git.PullMode) {
				a.run("拉取", func(ctx context.Context) error { return a.repo.Pull(ctx, mode) }, nil)
			}
			if m.Item("拉取（仅快进）").Chosen() {
				pull(git.PullFastForward)
			}
			if m.Item("拉取并变基").Chosen() {
				pull(git.PullRebase)
			}
			if m.Item("拉取并合并").Chosen() {
				pull(git.PullMerge)
			}
		})
		if ui.Button(c, "推送…").Tooltip("查看要推送的提交").Disabled(!idle || st.Branch == "").Clicked() {
			a.openPush()
		}
		if ui.Button(c, "刷新").Tooltip("⌘R").Clicked() {
			a.refresh()
		}
		ui.Button(c, "⋯").Label("仓库").Menu(a.repoMenu)
	})
}

// changesPanel is the working tree's changes, staged and not, and the
// commit message under them.
func (a *app) changesPanel(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).Basis(0).Children(func() {
		// The message on top, as VS Code has it, where it shows however
		// many files changed.
		ui.Column(c).Gap(8).Padding(10).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
			ui.TextArea(c, &a.message).Placeholder("提交说明（⌘↩ 提交）").Label("提交说明").Lines(2, 8)
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
		a.changeSection(c, "暂存的更改", true)
		a.changeSection(c, "更改", false)
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
	// A fixed height, so the header doesn't move as its button comes and
	// goes; the button stays while git runs, doing nothing then.
	ui.Row(c).Height(32).Padding(4, 8, 0, 10).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).Bold().TextColor(t.TextMuted)
		ui.Badge(c, fmt.Sprint(len(changes)))
		ui.Box(c).Grow(1)
		if len(changes) > 0 {
			idle := a.busy == ""
			if staged {
				if iconButton(c, "minus", "全部取消暂存") && idle {
					a.unstageAll()
				}
			} else if iconButton(c, "plus", "全部暂存") && idle {
				a.stageAll()
			}
		}
	})
	l := ui.List(c.Key(title), list, len(changes), func(i int) {
		ch := changes[i]
		chosen := i == *sel
		// As VS Code's: the name, its folder, the actions while the
		// pointer is over the row, and the change's letter at the end.
		row := ui.Row(c).Height(24).Padding(0, 8, 0, 10).Gap(6).AlignItems(ui.Center)
		if chosen {
			row.TextColor(t.AccentText)
		}
		row.Children(func() {
			code := ch.Work
			if staged {
				code = ch.Staged
			}
			name, dir := path.Base(ch.Path), path.Dir(ch.Path)
			ui.Text(c, name).SingleLine().Shrink(0)
			muted := t.TextMuted
			if chosen {
				muted = t.AccentText.Alpha(0.75)
			}
			if dir != "." {
				ui.Text(c, dir).FontSize(11).TextColor(muted).SingleLine().Grow(1).Basis(0)
			} else {
				ui.Box(c).Grow(1)
			}
			if row.Hovered() || chosen {
				if iconButton(c, "open", "打开文件") {
					a.openInEditor(filepath.Join(a.repo.Dir, filepath.FromSlash(ch.Path)), 0)
				}
				if !staged && !ch.Conflict && iconButton(c, "undo", "丢弃更改") {
					a.discard(ch)
				}
				icon, label := "plus", "暂存"
				if staged {
					icon, label = "minus", "取消暂存"
				}
				if iconButton(c, icon, label) {
					a.toggleStage(ch, staged)
				}
			}
			letter := string(code)
			if code == '?' {
				letter = "U" // untracked, as VS Code says it
			}
			ui.Text(c, letter).Font("monospace").FontSize(11).Bold().TextColor(changeColor(t, code, chosen)).
				Width(14).TextAlign(ui.Center).Tooltip(changeName(code))
		})
		row.ContextMenu(func(m *ui.Menu) {
			if m.Item(map[bool]string{true: "取消暂存", false: "暂存"}[staged]).Chosen() {
				a.toggleStage(ch, staged)
			}
			if !staged && !ch.Conflict && m.Item("丢弃更改…").Chosen() {
				a.discard(ch)
			}
			m.Separator()
			if ch.Work != '?' {
				a.fileMenu(m, ch.Path, "WORKTREE")
			} else {
				a.ignoreItems(m, ch.Path)
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
	// The keys of VS Code's and Finder's lists: space stages or unstages
	// the change chosen, delete discards it, asking first.
	chosen := func(fn func(git.Change)) func() {
		return func() {
			if *sel >= 0 && *sel < len(changes) && a.busy == "" {
				fn(changes[*sel])
			}
		}
	}
	list.Handle.OnShortcut(c, 0, ui.KeySpace, chosen(func(ch git.Change) { a.toggleStage(ch, staged) }))
	if !staged {
		discard := chosen(func(ch git.Change) {
			if !ch.Conflict {
				a.discard(ch)
			}
		})
		list.Handle.OnShortcut(c, 0, ui.KeyBackspace, discard)
		list.Handle.OnShortcut(c, 0, ui.KeyDelete, discard)
	}
}

func (a *app) toggleStage(ch git.Change, staged bool) {
	if staged {
		a.unstage(ch)
	} else {
		a.stage(ch)
	}
}

// changeColor colors a change's letter as VS Code does, white on a
// chosen row's accent.
func changeColor(t *ui.Theme, code byte, chosen bool) ui.Color {
	if chosen {
		return t.AccentText
	}
	// VS Code's colors of its dark themes, and of its light ones.
	pick := func(dark, light ui.Color) ui.Color {
		if t.Dark {
			return dark
		}
		return light
	}
	switch code {
	case 'A', '?', 'R', 'C':
		return pick(ui.RGB(115, 201, 145), ui.RGB(88, 124, 12))
	case 'D':
		return pick(ui.RGB(199, 78, 57), ui.RGB(173, 7, 7))
	case 'U':
		return pick(ui.RGB(228, 103, 107), ui.RGB(173, 7, 7))
	}
	return pick(ui.RGB(226, 192, 141), ui.RGB(137, 85, 3)) // modified
}

func changeName(code byte) string {
	return map[byte]string{'M': "已修改", 'A': "新增", 'D': "已删除", 'R': "已重命名", 'C': "已复制", '?': "未跟踪", 'U': "冲突", 'T': "类型改变"}[code]
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
		case d.blame != nil:
			a.blameView(c, d.blame)
		case d.merge != nil:
			a.mergeView(c, d.merge)
		case d.change != nil && d.change.Conflict:
			ui.Column(c).Fill().Center().Children(func() { ui.Spinner(c).Size(18, 18) })
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
	d := &a.detail
	ui.Column(c).Padding(8, 12).Gap(3).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		ui.Text(c, cm.Subject).Bold().SingleLine()
		if comparing(cm) {
			ui.Text(c, fmt.Sprintf("%d 个文件有差异", len(d.files))).TextColor(t.TextMuted)
			return
		}
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, cm.Short()).Font("monospace").TextColor(t.Accent).Tooltip(cm.Hash)
			ui.Text(c, cm.Author).TextColor(t.TextMuted)
			ui.Text(c, cm.Time.Format("2006-01-02 15:04")).TextColor(t.TextMuted)
			ui.Box(c).Grow(1)
			if n := len(cm.Parents); n > 1 {
				labels := make([]string, n)
				for i, p := range cm.Parents {
					labels[i] = fmt.Sprintf("对比父提交 %d（%s）", i+1, p[:8])
				}
				label := labels[d.parent]
				if ui.Select(c.Key("parent"), &label, labels).Label("对比的父提交").Changed() {
					a.showParent(slices.Index(labels, label))
				}
			}
		})
		if body := strings.TrimSpace(strings.TrimPrefix(d.message, cm.Subject)); body != "" {
			ui.Text(c, body).TextColor(t.TextMuted).Wrap().MaxLines(6)
		}
	})
}

// fileMenu is a file's context menu, of a commit's files or the changes.
func (a *app) fileMenu(m *ui.Menu, path, rev string) {
	if m.Item("在编辑器中打开").Chosen() {
		a.openInEditor(filepath.Join(a.repo.Dir, filepath.FromSlash(path)), 0)
	}
	if m.Item("文件历史").Chosen() {
		a.setScope(git.Scope{Ref: a.scope.Ref, Path: path})
	}
	if m.Item("Blame（逐行追溯）").Chosen() {
		a.showBlame(rev, path)
	}
	m.Separator()
	if m.Item("复制路径").Chosen() {
		mygo.Clipboard.WriteText(path)
	}
	if m.Item("在文件夹中显示").Chosen() {
		mygo.Shell.ShowItemInFolder(filepath.Join(a.repo.Dir, filepath.FromSlash(path)))
	}
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
		rev := ""
		if a.detail.commit != nil {
			rev = a.detail.commit.Hash
		}
		ui.Row(c).Height(24).Padding(0, 8, 0, 10).Gap(6).AlignItems(ui.Center).Tooltip(name).ContextMenu(func(m *ui.Menu) {
			a.fileMenu(m, name, rev)
		}).Children(func() {
			chosen := i == a.detail.fileSel
			muted := t.TextMuted
			if chosen {
				muted = t.AccentText.Alpha(0.75)
			}
			ui.Text(c, path.Base(name)).SingleLine().Shrink(0)
			if dir := path.Dir(name); dir != "." {
				ui.Text(c, dir).FontSize(11).TextColor(muted).SingleLine().Grow(1).Basis(0)
			} else {
				ui.Box(c).Grow(1)
			}
			ui.Text(c, string(code)).Font("monospace").FontSize(11).Bold().TextColor(changeColor(t, code, chosen)).
				Width(14).TextAlign(ui.Center).Tooltip(changeName(code))
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
	if ui.Button(c, "在编辑器中打开").Clicked() {
		a.openChangeInEditor()
	}
	if a.detail.whole {
		if ui.PrimaryButton(c, "保存").Disabled(!a.unsaved()).Tooltip("⌘S").Clicked() {
			a.saveFile()
		}
		c.OnShortcut(ui.Cmd, ui.KeyS, a.saveFile)
	}
	if a.detail.patch == nil || !patch.Partial(*a.detail.patch) || prefs.IgnoreWhitespace {
		return
	}
	chosen := a.selectedLines()
	none := chosen == nil || a.busy != "" || a.unsaved()
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
