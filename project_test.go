package main

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	loc "mygit/locales"
)

// keepRecent restores the repositories opened lately after a test, which
// sets them.
func keepRecent(t *testing.T, recent ...string) {
	oldRemotes, oldRecent := prefs.Remotes, prefs.Recent
	t.Cleanup(func() { prefs.Remotes, prefs.Recent = oldRemotes, oldRecent })
	prefs.Remotes, prefs.Recent = slices.Clone(recent), slices.Clone(recent)
}

// The repositories opened lately are the welcome window's right column,
// each taken off it with the button its row shows as it is pointed at.
func TestWelcomeForgetsRecent(t *testing.T) {
	keepRecent(t, "devbox:/home/mia/orbit", "devbox:/home/mia/atlas")
	w := &welcome{recent: recentLocations()}
	tt := ui.NewTester(w.view, 860, 600)
	tt.Frame()
	for _, s := range []string{"最近打开", "清除最近打开", "orbit", "atlas", "把仓库文件夹拖到这里"} {
		if !tt.HasText(s) {
			t.Fatalf("no %q in %q", s, tt.Texts())
		}
	}
	if tt.HasText(loc.RemoveRecent()) {
		t.Fatal("a row's remove button shows unpointed")
	}
	r, _ := tt.Find("orbit")
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()
	if err := tt.Click(loc.RemoveRecent()); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	if slices.Contains(prefs.Recent, "devbox:/home/mia/orbit") || slices.Contains(prefs.Remotes, "devbox:/home/mia/orbit") ||
		tt.HasText("orbit") || !tt.HasText("atlas") {
		t.Fatalf("recent %q %q, texts %q", prefs.Recent, prefs.Remotes, tt.Texts())
	}
}

// With none opened lately, the column says where they will show.
func TestWelcomeNoRecent(t *testing.T) {
	keepRecent(t)
	w := &welcome{}
	tt := ui.NewTester(w.view, 860, 600)
	tt.Frame()
	if !tt.HasText(loc.NoRecent()) || tt.HasText(loc.ClearRecent()) {
		t.Fatalf("texts %q", tt.Texts())
	}
}

// The repository's name in the toolbar opens the project's menu, with
// either button: the others opened lately to switch to, and closing it.
func TestProjectMenu(t *testing.T) {
	tt := newTester(t)
	keepRecent(t, "devbox:/home/mia/orbit", tt.a.repo.Location())
	name := tt.a.repo.Name()
	for _, open := range []func(string) error{tt.Click, tt.RightClick} {
		if err := open(name); err != nil {
			t.Fatalf("%v; texts %q", err, tt.Texts())
		}
		tt.Frame()
		if menu := tt.Menu(); !slices.Contains(menu, loc.SwitchProject()) || !slices.Contains(menu, loc.CloseProject()) {
			t.Fatalf("menu %q", menu)
		}
		tt.CloseMenu()
		tt.Frame()
	}
	keepRecent(t, tt.a.repo.Location()) // only this one: nothing to switch to
	tt.Click(name)
	tt.Frame()
	if menu := tt.Menu(); slices.Contains(menu, loc.SwitchProject()) || !slices.Contains(menu, loc.CloseProject()) {
		t.Fatalf("menu %q", menu)
	}
}
