package main

import (
	"context"
	"slices"
	"sync"

	"github.com/egoist/mygo/ui"

	"mygit/internal/git"
	loc "mygit/locales"
)

// withSubmodules are repos, each followed by its submodules checked out,
// theirs too, as VS Code's source control lists them (#54); each opened
// once, its submodules read alongside the others', a round trip each over
// SSH.
func withSubmodules(ctx context.Context, repos []*git.Repo) []*git.Repo {
	subs := make([][]*git.Repo, len(repos))
	var wg sync.WaitGroup
	for i, r := range repos {
		wg.Add(1)
		go func() {
			defer wg.Done()
			subs[i] = submodulesOf(ctx, r)
		}()
	}
	wg.Wait()
	var all []*git.Repo
	seen := map[string]bool{}
	for i, r := range repos {
		for _, s := range append([]*git.Repo{r}, subs[i]...) {
			if !seen[s.Dir] {
				seen[s.Dir] = true
				all = append(all, s)
			}
		}
	}
	return all
}

// submodulesOf are the repository's submodules checked out, opened.
func submodulesOf(ctx context.Context, r *git.Repo) []*git.Repo {
	list, err := r.Submodules(ctx)
	if err != nil {
		return nil
	}
	var subs []*git.Repo
	for _, s := range list {
		if !s.Initialized {
			continue
		}
		// A folder that isn't a working tree after all opens its parent.
		if sub, err := r.OpenSubmodule(s.Path); err == nil && sub.Dir != r.Dir {
			subs = append(subs, sub)
		}
	}
	return subs
}

// uninitialized is how many of the repository's submodules are left
// empty, as a clone without them leaves them.
func (a *app) uninitialized() int {
	n := 0
	for _, s := range a.submodules {
		if !s.Initialized {
			n++
		}
	}
	return n
}

// submoduleItems are the repository menu's: adding a submodule, and with
// some, checking them out as recorded.
func (a *app) submoduleItems(m *ui.Menu) {
	if m.Item(loc.AddSubmoduleMenu()).Chosen() {
		a.addSubmodule()
	}
	if len(a.submodules) > 0 && m.Item(loc.UpdateSubmodules()).Chosen() {
		a.updateSubmodules()
	}
}

// submodulesRow says, over the changes, how many submodules are left
// empty, with the button that checks them out.
func (a *app) submodulesRow(c *ui.Context) {
	n := a.uninitialized()
	if n == 0 {
		return
	}
	t := c.Theme()
	ui.Row(c).Padding(6, 10).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, loc.SubmodulesEmpty(n)).FontSize(12).TextColor(t.TextMuted).Wrap().Grow(1).Basis(0)
		if ui.Button(c, loc.InitSubmodules()).Disabled(a.busy != "").Clicked() {
			a.updateSubmodules()
		}
	})
}

// updateSubmodules initializes the submodules and checks them out at the
// commits recorded, then shows those checked out beside the repository.
func (a *app) updateSubmodules() {
	a.run(loc.OpUpdateSubmodules(), a.repo.UpdateSubmodules, a.regroup)
}

// addSubmodule asks for a repository's URL and the folder to clone it
// into, then adds it as a submodule.
func (a *app) addSubmodule() {
	a.prompt = prompt{open: true, title: loc.AddSubmoduleTitle(), button: loc.Add(), note: loc.AddSubmoduleNote(),
		message: loc.SubmodulePath(), ok: func(url, dir string) {
			a.run(loc.OpAddSubmodule(), func(ctx context.Context) error { return a.repo.AddSubmodule(ctx, url, dir) }, a.regroup)
		}}
}

// regroup shows the submodules checked out beside the repository, once
// they are added or initialized: a workspace takes the new ones in, a
// window of one repository opens again as a workspace of them.
func (a *app) regroup() {
	work(a, func(ctx context.Context) ([]*git.Repo, error) {
		return withSubmodules(ctx, []*git.Repo{a.repo}), nil
	}, func(repos []*git.Repo, _ error) {
		if a.ws != nil {
			a.ws.add(repos)
			return
		}
		if len(repos) < 2 || a.unsaved() {
			return // nothing to show beside it, or an edit to keep first
		}
		old := a.win
		delete(apps, a.repo.Location())
		openWorkspace(a.repo.Host(), a.repo.Dir, repos)
		old.Close()
	})
}

// add takes in the repositories it doesn't have yet, after the last.
func (ws *workspace) add(repos []*git.Repo) {
	for _, r := range repos {
		if slices.ContainsFunc(ws.apps, func(a *app) bool { return a.repo.Dir == r.Dir }) {
			continue
		}
		a := newApp(r)
		a.ws, a.win = ws, ws.win
		a.update = ws.apps[0].update // the window's, as start sets it
		ws.apps = append(ws.apps, a)
		a.start()
	}
}
