package main

import (
	"image/png"
	"os"
	"testing"
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
