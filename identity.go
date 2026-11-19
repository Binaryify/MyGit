package main

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"

	"mygit/internal/git"
	loc "mygit/locales"
)

// identityDialog sets who the commits are made as, git's user.name and
// user.email, for the repository or for every one (#43).
type identityDialog struct {
	open bool
	id   git.Identity // as edited
}

// openIdentity shows the dialog with the identity git uses now.
func (a *app) openIdentity() {
	work(a, func(ctx context.Context) (git.Identity, error) { return a.repo.Identity(ctx), nil }, func(id git.Identity, _ error) {
		a.identity = identityDialog{open: true, id: id}
	})
}

func (a *app) identityView(c *ui.Context) {
	d := &a.identity
	if !d.open {
		return
	}
	t := c.Theme()
	ui.Modal(c, &d.open, func() {
		ui.Column(c).Gap(10).Width(dialogWidth(c, 420)).Children(func() {
			ui.Text(c, loc.IdentityTitle()).Bold().FontSize(15)
			ui.Text(c, loc.IdentityName()).Bold().TextColor(t.TextMuted)
			ui.TextInput(c.Key("identity-name"), &d.id.Name).Label(loc.IdentityName()).Placeholder("Jane Doe")
			ui.Text(c, loc.IdentityEmail()).Bold().TextColor(t.TextMuted)
			ui.TextInput(c.Key("identity-email"), &d.id.Email).Label(loc.IdentityEmail()).Placeholder("jane@example.com")
			ui.Checkbox(c.Key("identity-global"), &d.id.Global, loc.IdentityGlobal())
			hint := loc.IdentityLocalHint()
			if d.id.Global {
				hint = loc.IdentityGlobalHint()
			}
			ui.Text(c, hint).FontSize(11).TextColor(t.TextMuted).Wrap()
			ui.Row(c).Gap(8).Children(func() {
				ui.Box(c).Grow(1)
				if ui.Button(c, loc.Cancel()).Clicked() {
					d.open = false
				}
				id := git.Identity{Name: strings.TrimSpace(d.id.Name), Email: strings.TrimSpace(d.id.Email), Global: d.id.Global}
				if ui.PrimaryButton(c, loc.Save()).Disabled(a.busy != "" || id.Name == "" || id.Email == "").Clicked() {
					d.open = false
					a.run(loc.OpSaveIdentity(), func(ctx context.Context) error { return a.repo.SetIdentity(ctx, id) }, nil)
				}
			})
		})
	})
}
