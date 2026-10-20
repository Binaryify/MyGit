package main

import (
	"image/png"
	"mygit/internal/sysproxy"
	loc "mygit/locales"
	"net/http"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
)

// The settings list the shortcuts, each with what it does.
func TestSettingsListShortcuts(t *testing.T) {
	tt := newTester(t)
	tt.a.settingsOpen = true
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
	tt.a.settingsOpen = true
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
