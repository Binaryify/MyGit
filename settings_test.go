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
