package main

import (
	"image/png"
	"mygit/internal/sysproxy"
	loc "mygit/locales"
	"net/http"
	"os"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// The settings list the shortcuts, each with what it does.
func TestSettingsListShortcuts(t *testing.T) {
	tt := newTester(t)
	tt.a.settings.open = true
	tt.Frame()
	for _, s := range []string{"快捷键", shortcut("K"), "命令栏", shortcut("↩"), "提交", "空格", shortcut("点击"), "比较两个提交"} {
		if !tt.HasText(s) {
			t.Fatalf("no %q in %q", s, tt.Texts())
		}
	}
	if out := os.Getenv("MYGIT_DUMP"); out != "" {
		tt.SetDark(true)
		tt.Frame()
		f, _ := os.Create(out + "/settings.png")
		png.Encode(f, tt.Image())
		f.Close()
	}
}

// The proxy written in the settings is taken once it is one, and said
// wrong until then.
func TestProxySetting(t *testing.T) {
	old := prefs.Proxy
	defer func() { prefs.Proxy = old; sysproxy.Set(old) }()
	prefs.Proxy = ""
	tt := newTester(t)
	tt.a.settings.open = true
	tt.Frame()
	if !tt.HasText("代理") || !tt.HasText(loc.ProxyHint()) {
		t.Fatalf("texts %q", tt.Texts())
	}
	if err := tt.Click("代理"); err != nil {
		t.Fatal(err)
	}
	tt.Type("ftp://nope")
	tt.Frame()
	if !tt.HasText(loc.ProxyInvalid()) {
		t.Errorf("no complaint: %q", tt.Texts())
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("127.0.0.1:7890")
	tt.Frame()
	req, _ := http.NewRequest("GET", "https://github.com/", nil)
	if u, _ := sysproxy.Func(req); u == nil || u.String() != "http://127.0.0.1:7890" || tt.HasText(loc.ProxyInvalid()) {
		t.Errorf("proxy %v, prefs %q", u, prefs.Proxy)
	}
}

// With no repository open, the welcome window offers the update and the
// settings, without the shortcuts of a repository's window.
func TestWelcomeUpdateAndSettings(t *testing.T) {
	old := updates.available
	defer func() { updates.available = old }()
	updates.available = &mygo.Update{Version: "9.9.9"}
	w := &welcome{}
	tt := ui.NewTester(w.view, 520, 640)
	tt.Frame()
	if !tt.HasText(loc.UpdateAvailable("9.9.9")) || !tt.HasText(loc.UpdateInstall()) {
		t.Errorf("no update offered: %q", tt.Texts())
	}
	w.settings.open = true
	tt.Frame()
	if !tt.HasText(loc.NetworkSection()) || tt.HasText(loc.Shortcuts()) {
		t.Errorf("settings %q", tt.Texts())
	}
	if err := tt.Click(loc.Done()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if w.settings.open || tt.HasText(loc.NetworkSection()) {
		t.Error("the settings didn't close")
	}
}

// With no menu bar, as on Windows and Linux, the welcome window carries
// the app's menu and opens the settings with Ctrl+,.
func TestWelcomeAppMenu(t *testing.T) {
	old := menuBar
	defer func() { menuBar = old }()
	menuBar = false
	w := &welcome{}
	tt := ui.NewTester(w.view, 520, 640)
	tt.SetTitleBar(ui.TitleBar{Right: 138, Height: 32})
	tt.Frame()
	if err := tt.Click("MyGit"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	if err := tt.ChooseMenuItem(loc.SettingsMenu()); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	tt.Frame()
	if !w.settings.open {
		t.Fatal("the settings didn't open")
	}
	if out := os.Getenv("ZZ_OUT"); out != "" {
		w.settings.open = false
		tt.SetDark(true)
		tt.SetScale(2)
		tt.Frame()
		tt.Click("MyGit")
		tt.Frame()
		f, _ := os.Create(out + "/welcome-windows.png")
		png.Encode(f, tt.Image())
		f.Close()
		tt.CloseMenu()
	}
	w.settings.open = false
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyComma)
	tt.Frame()
	if !w.settings.open {
		t.Error("Ctrl+, didn't open the settings")
	}
	menuBar = true
	w2 := &welcome{}
	tt2 := ui.NewTester(w2.view, 520, 640)
	tt2.Frame()
	if tt2.HasText("MyGit") && tt2.Click("MyGit") == nil && len(tt2.Menu()) > 0 {
		t.Error("the welcome window carries a menu beside macOS' menu bar")
	}
}

// The appearance chosen in the settings applies at once and is kept;
// following the system again keeps nothing.
func TestThemeSetting(t *testing.T) {
	old, oldLang, oldApply := prefs.Theme, prefs.Language, applyTheme
	defer func() { prefs.Theme, prefs.Language, applyTheme = old, oldLang, oldApply }()
	prefs.Language = "en" // its select shows English, not 跟随系统 as the appearance's can
	var applied []mygo.ThemeSource
	applyTheme = func(s mygo.ThemeSource) { applied = append(applied, s) }
	prefs.Theme = mygo.ThemeDark
	tt := newTester(t)
	tt.a.settings.open = true
	tt.Frame()
	if !tt.HasText("外观") {
		t.Fatalf("no appearance in %q", tt.Texts())
	}
	choose := func(from, to string) {
		t.Helper()
		if err := tt.Click(from); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if err := tt.ChooseMenuItem(to); err != nil {
			if err := tt.Click(to); err != nil {
				t.Fatalf("choosing %q: %v; menu %q, texts %q", to, err, tt.Menu(), tt.Texts())
			}
		}
		tt.Frame()
	}
	choose("深色", "浅色")
	if prefs.Theme != mygo.ThemeLight || len(applied) != 1 || applied[0] != mygo.ThemeLight {
		t.Fatalf("after light: theme %q, applied %v", prefs.Theme, applied)
	}
	choose("浅色", "跟随系统")
	if prefs.Theme != "" || len(applied) != 2 || applied[1] != "" {
		t.Fatalf("after the system's: theme %q, applied %v", prefs.Theme, applied)
	}
}
