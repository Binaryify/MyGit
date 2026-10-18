package main

import (
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// credPrompt is git's or ssh's question for a username, a password, a
// token or a passphrase, which a window asks for the command waiting.
type credPrompt struct {
	open   bool
	prompt string
	value  string
	done   chan credAnswer
}

type credAnswer struct {
	value string
	ok    bool
}

// ask shows a prompt in win, giving an earlier one up.
func (p *credPrompt) ask(win *mygo.Window, prompt string, done chan credAnswer) {
	if p.open {
		p.done <- credAnswer{}
	}
	*p = credPrompt{open: true, prompt: strings.TrimSpace(prompt), done: done}
	if win != nil {
		win.Show()
		win.Focus()
	}
}

func (a *app) askCredential(prompt string, done chan credAnswer) { a.cred.ask(a.win, prompt, done) }

// secret tells whether a prompt asks for what shouldn't show as typed.
func secret(prompt string) bool {
	p := strings.ToLower(prompt)
	return strings.Contains(p, "password") || strings.Contains(p, "passphrase") || strings.Contains(p, "token")
}

func (p *credPrompt) view(c *ui.Context) {
	if !p.open {
		return
	}
	t := c.Theme()
	answer := func(ok bool) {
		p.open = false
		p.done <- credAnswer{p.value, ok}
		p.value = ""
	}
	if ui.Modal(c, &p.open, func() {
		ui.Column(c).Gap(12).Width(420).Children(func() {
			ui.Text(c, "git 需要验证").Bold().FontSize(15)
			ui.Text(c, p.prompt).TextColor(t.TextMuted).Wrap()
			in := ui.TextInput(c, &p.value).Label(p.prompt).AutoFocus()
			if secret(p.prompt) {
				in.Password()
				ui.Text(c, "GitHub 等网站要填个人访问令牌（token），不是登录密码。验证成功后由 git 的凭据管理器记住。").
					FontSize(11).TextColor(t.TextMuted).Wrap()
			}
			submitted := in.Submitted()
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					answer(false)
				}
				if ui.PrimaryButton(c, "确定").Clicked() || submitted {
					answer(true)
				}
			})
		})
	}).Dismissed() {
		p.open = true // Escape: answer as cancel
		answer(false)
	}
}
