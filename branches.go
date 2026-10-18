package main

import (
	"context"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mygit/internal/git"
)

// prompt asks for a name, of a branch or a tag, in a dialog.
type prompt struct {
	open                  bool
	title, button, value  string
	note, message, msgVal string // a second, optional field: a tag's message
	ok                    func(value, message string)
}

func (a *app) ask1(title, button, value string, ok func(string)) {
	a.prompt = prompt{open: true, title: title, button: button, value: value,
		ok: func(v, _ string) { ok(v) }}
}

func (a *app) promptView(c *ui.Context) {
	p := &a.prompt
	ui.Modal(c, &p.open, func() {
		ui.Column(c).Gap(12).Width(360).Children(func() {
			ui.Text(c, p.title).Bold().FontSize(14)
			submitted := ui.TextInput(c, &p.value).Label(p.title).AutoFocus().Submitted()
			if p.message != "" {
				ui.TextInput(c, &p.msgVal).Placeholder(p.message).Label(p.message)
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					p.open = false
				}
				ok := strings.TrimSpace(p.value) != ""
				if (ui.PrimaryButton(c, p.button).Disabled(!ok).Clicked() || submitted) && ok {
					p.open = false
					p.ok(strings.TrimSpace(p.value), strings.TrimSpace(p.msgVal))
				}
			})
		})
	})
}

// branchMenu is the toolbar's branch button: a branch to switch to, and
// what to do with each, as IntelliJ's branches popup.
func (a *app) branchMenu(m *ui.Menu) {
	cur := a.status.Branch
	if m.Item("新建分支…").Chosen() {
		a.newBranch("")
	}
	m.Separator()
	for _, b := range a.branches {
		label := b
		if b == cur {
			label = "✓ " + b
		}
		m.Submenu(label, func(m *ui.Menu) {
			if b != cur && m.Item("切换到此分支").Chosen() {
				a.run("切换到 "+b, func(ctx context.Context) error { return a.repo.Switch(ctx, b) }, nil)
			}
			if b != cur {
				a.mergeItems(m, b)
			}
			if m.Item("从此分支新建分支…").Chosen() {
				a.newBranch(b)
			}
			if m.Item("显示此分支的历史").Chosen() {
				a.setScope(git.Scope{Ref: b})
			}
			m.Separator()
			if m.Item("重命名…").Chosen() {
				a.ask1("重命名分支 "+b, "重命名", b, func(to string) {
					a.run("重命名分支", func(ctx context.Context) error { return a.repo.RenameBranch(ctx, b, to) }, nil)
				})
			}
			if b != cur && m.Item("删除").Chosen() {
				a.deleteBranch(b)
			}
		})
	}
	if len(a.remotes) == 0 {
		return
	}
	m.Separator()
	m.Submenu("远程分支", func(m *ui.Menu) {
		for _, rb := range a.remotes {
			m.Submenu(rb, func(m *ui.Menu) {
				if m.Item("检出为本地分支").Chosen() {
					a.run("检出 "+rb, func(ctx context.Context) error { return a.repo.CheckoutRemote(ctx, rb) }, nil)
				}
				a.mergeItems(m, rb)
				if m.Item("从此分支新建分支…").Chosen() {
					a.newBranch(rb)
				}
				m.Separator()
				if m.Item("删除远程分支…").Chosen() {
					a.ask("删除远程分支 "+rb+"？", "会从远程仓库删除这个分支。", "删除", func() {
						a.run("删除远程分支", func(ctx context.Context) error { return a.repo.DeleteRemoteBranch(ctx, rb) }, nil)
					})
				}
			})
		}
	})
}

// mergeItems merge a branch into the current one, or rebase the current
// one onto it.
func (a *app) mergeItems(m *ui.Menu, rev string) {
	cur := a.status.Branch
	if cur == "" {
		cur = "HEAD"
	}
	if m.Item("合并到 " + cur).Chosen() {
		a.run("合并 "+rev, func(ctx context.Context) error { return a.repo.Merge(ctx, rev) }, nil)
	}
	if m.Item("将 " + cur + " 变基到此").Chosen() {
		a.run("变基到 "+rev, func(ctx context.Context) error { return a.repo.Rebase(ctx, rev) }, nil)
	}
}

// newBranch asks for a branch's name, made at start (HEAD when empty) and
// checked out.
func (a *app) newBranch(start string) {
	title := "新建分支"
	if start != "" {
		title += "（基于 " + shortRev(start) + "）"
	}
	a.ask1(title, "创建并切换", "", func(name string) {
		a.run("新建分支 "+name, func(ctx context.Context) error {
			return a.repo.CreateBranch(ctx, name, start, true)
		}, nil)
	})
}

// deleteBranch deletes a merged branch at once, and asks before deleting
// one whose commits would be lost.
func (a *app) deleteBranch(b string) {
	a.run("删除分支 "+b, func(ctx context.Context) error {
		err := a.repo.DeleteBranch(ctx, b, false)
		if git.NotMerged(err) {
			a.update(func() {
				a.ask("分支 "+b+" 还没有合并", "删除后，只在这个分支上的提交会丢失。", "仍然删除", func() {
					a.run("删除分支 "+b, func(ctx context.Context) error { return a.repo.DeleteBranch(ctx, b, true) }, nil)
				})
			})
			return nil
		}
		return err
	}, nil)
}

// commitMenu is a commit's context menu in the history.
func (a *app) commitMenu(m *ui.Menu, cm git.Commit) {
	if m.Item("复制哈希").Chosen() {
		mygo.Clipboard.WriteText(cm.Hash)
	}
	if m.Item("复制说明").Chosen() {
		mygo.Clipboard.WriteText(cm.Subject)
	}
	m.Separator()
	if m.Item("从此提交新建分支…").Chosen() {
		a.newBranch(cm.Hash)
	}
	if m.Item("新建标签…").Chosen() {
		a.prompt = prompt{open: true, title: "新建标签（" + cm.Short() + "）", button: "创建", message: "说明（可选，填写则为附注标签）",
			ok: func(name, msg string) {
				a.run("新建标签 "+name, func(ctx context.Context) error { return a.repo.Tag(ctx, name, cm.Hash, msg) }, nil)
			}}
	}
	if m.Item("检出此提交").Chosen() {
		a.run("检出 "+cm.Short(), func(ctx context.Context) error { return a.repo.CheckoutDetached(ctx, cm.Hash) }, nil)
	}
	m.Separator()
	if m.Item("Cherry-pick").Chosen() {
		a.run("Cherry-pick "+cm.Short(), func(ctx context.Context) error { return a.repo.CherryPick(ctx, cm) }, nil)
	}
	if m.Item("撤销提交（revert）").Chosen() {
		a.run("撤销 "+cm.Short(), func(ctx context.Context) error { return a.repo.Revert(ctx, cm) }, nil)
	}
	a.mergeItems(m, cm.Hash)
	if a.status.Branch != "" {
		m.Submenu("将 "+a.status.Branch+" 重置到此", func(m *ui.Menu) {
			reset := func(mode git.ResetMode) {
				a.run("重置", func(ctx context.Context) error { return a.repo.Reset(ctx, cm.Hash, mode) }, nil)
			}
			if m.Item("Soft：保留更改，并暂存").Chosen() {
				reset(git.ResetSoft)
			}
			if m.Item("Mixed：保留更改，不暂存").Chosen() {
				reset(git.ResetMixed)
			}
			if m.Item("Hard：丢弃所有更改…").Chosen() {
				a.ask("硬重置到 "+cm.Short()+"？", "工作区和暂存区的所有更改都会丢失，无法撤销。", "硬重置", func() { reset(git.ResetHard) })
			}
		})
	}
	for _, ref := range cm.Refs {
		if tag, ok := strings.CutPrefix(ref, "tag: "); ok && m.Item("删除标签 "+tag).Chosen() {
			a.run("删除标签 "+tag, func(ctx context.Context) error { return a.repo.DeleteTag(ctx, tag) }, nil)
		}
	}
}

// shortRev shortens a hash, and leaves a branch's name as it is.
func shortRev(rev string) string {
	if len(rev) == 40 && strings.Trim(rev, "0123456789abcdef") == "" {
		return rev[:8]
	}
	return rev
}
