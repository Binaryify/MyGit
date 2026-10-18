package main

import (
	"context"
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mygit/internal/conflict"
	"mygit/internal/git"
)

// merger resolves a file in conflict, as IntelliJ's merge window: our
// version on the left, theirs on the right, and between them the result,
// git's merged text with its conflicts, to edit and resolve a conflict at
// a time.
type merger struct {
	path           string
	ours, theirs   *editor.State
	result         *editor.State
	blocks         []conflict.Block
	version        uint64 // the result's, that blocks were read at
	cur            int    // the conflict shown
	split1, split2 float32
}

type mergeTexts struct{ ours, theirs, result string }

// showConflict opens the merge of a file in conflict.
func (a *app) showConflict(c git.Change) {
	a.detail = detail{change: &c, gen: a.detail.gen + 1, loading: true}
	gen := a.detail.gen
	work(a, func(ctx context.Context) (mergeTexts, error) {
		result, err := a.repo.ReadFile(c.Path)
		return mergeTexts{a.repo.ShowStage(ctx, c.Path, git.OursStage), a.repo.ShowStage(ctx, c.Path, git.TheirsStage), result}, err
	}, func(t mergeTexts, err error) {
		if gen != a.detail.gen {
			return
		}
		a.detail.loading = false
		if err != nil {
			a.errMsg = err.Error()
			return
		}
		lang := editor.LanguageFor(c.Path)
		state := func(text string, readOnly bool) *editor.State {
			b := editor.NewBuffer(text)
			if lang != nil {
				b.SetLanguage(lang)
			}
			return &editor.State{Buffer: b, ReadOnly: readOnly}
		}
		a.detail.merge = &merger{path: c.Path, ours: state(t.ours, true), theirs: state(t.theirs, true),
			result: state(t.result, false), version: ^uint64(0), split1: 30, split2: 57}
	})
}

// sync reads the result's conflicts again once it was edited.
func (m *merger) sync() {
	if v := m.result.Buffer.Version(); v != m.version {
		m.version = v
		m.blocks = conflict.Parse(m.result.Buffer.String())
		m.cur = min(m.cur, max(len(m.blocks)-1, 0))
	}
	m.result.LineBackgrounds = m.result.LineBackgrounds[:0]
	for i, b := range m.blocks {
		alpha := float32(0.12)
		if i == m.cur {
			alpha = 0.28
		}
		m.result.LineBackgrounds = append(m.result.LineBackgrounds,
			editor.LineBackground{First: b.FirstLine, Last: b.LastLine, Color: ui.RGB(230, 126, 34).Alpha(alpha)})
	}
}

func (m *merger) goTo(i int) {
	if i >= 0 && i < len(m.blocks) {
		m.cur = i
		m.result.ScrollTo(m.blocks[i].FirstLine)
		m.result.Select(m.blocks[i].Start, m.blocks[i].Start)
	}
}

// take resolves the conflict shown.
func (m *merger) take(ch conflict.Choice) {
	if m.cur < len(m.blocks) {
		b := m.blocks[m.cur]
		m.result.Buffer.Edit(editor.Edit{Start: b.Start, End: b.End, Text: b.Text(ch)})
		m.sync()
		m.goTo(m.cur)
	}
}

func (a *app) mergeView(c *ui.Context, m *merger) {
	t := c.Theme()
	m.sync()
	left, right := "我们的（HEAD）", "他们的"
	if a.op == git.Rebasing { // rebasing, HEAD is the branch rebased onto
		left, right = "上游（ours）", "你的提交（theirs）"
	}
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Padding(6, 12).Gap(6).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
			ui.Text(c, m.path).Bold().SingleLine()
			if n := len(m.blocks); n > 0 {
				ui.Text(c, fmt.Sprintf("冲突 %d / %d", m.cur+1, n)).TextColor(t.TextMuted)
				if ui.Button(c, "◀").Label("上一个冲突").Disabled(m.cur == 0).Clicked() {
					m.goTo(m.cur - 1)
				}
				if ui.Button(c, "▶").Label("下一个冲突").Disabled(m.cur >= n-1).Clicked() {
					m.goTo(m.cur + 1)
				}
				if ui.Button(c, "采用左侧").Clicked() {
					m.take(conflict.Ours)
				}
				if ui.Button(c, "采用右侧").Clicked() {
					m.take(conflict.Theirs)
				}
				if ui.Button(c, "两者都要").Clicked() {
					m.take(conflict.Both)
				}
			} else {
				ui.Text(c, "没有剩余冲突").TextColor(t.TextMuted)
			}
			ui.Box(c).Grow(1)
			ui.Button(c, "整个文件 ▾").Label("整个文件").Menu(func(menu *ui.Menu) {
				if menu.Item("全部采用左侧（" + left + "）").Chosen() {
					a.takeSide(m.path, git.OursStage)
				}
				if menu.Item("全部采用右侧（" + right + "）").Chosen() {
					a.takeSide(m.path, git.TheirsStage)
				}
			})
			if ui.PrimaryButton(c, "保存并标记为已解决").Disabled(a.busy != "").Clicked() {
				a.saveResolution(m)
			}
		})
		pane := func(title string, s *editor.State) {
			ui.Column(c).Fill().Children(func() {
				ui.Text(c, title).TextColor(t.TextMuted).Padding(4, 10)
				editor.View(c.Key(title), s).Grow(1).FillWidth()
			})
		}
		ui.SplitPercent(c.Key("merge1"), &m.split1, func() { pane(left, m.ours) }, func() {
			ui.SplitPercent(c.Key("merge2"), &m.split2, func() { pane("结果", m.result) }, func() { pane(right, m.theirs) }).Fill()
		}).Grow(1)
	})
}

func (a *app) takeSide(path string, s git.Stage) {
	a.run("解决冲突", func(ctx context.Context) error { return a.repo.TakeSide(ctx, path, s) }, func() { a.detail = detail{} })
}

// saveResolution writes the result and marks the file resolved, asking
// first while conflict markers are left in it.
func (a *app) saveResolution(m *merger) {
	save := func() {
		text := m.result.Buffer.String()
		a.run("解决冲突", func(ctx context.Context) error { return a.repo.Resolve(ctx, m.path, text) }, func() { a.detail = detail{} })
	}
	if n := len(m.blocks); n > 0 {
		a.ask("还有冲突没解决", fmt.Sprintf("结果里还有 %d 处冲突标记，仍然保存并标记为已解决？", n), "仍然保存", save)
		return
	}
	save()
}
