package main

import (
	"github.com/egoist/mygo/i18n"
	"image/png"
	"os"
	"testing"
)

// TestDumpThemes renders the window in the light and dark themes into
// MYGIT_DUMP, to look at:
//
//	MYGIT_DUMP=/tmp go test -run DumpThemes
//
// MYGIT_LOCALE renders it in another language, as en.
func TestDumpThemes(t *testing.T) {
	out := os.Getenv("MYGIT_DUMP")
	if out == "" {
		t.Skip()
	}
	if l := os.Getenv("MYGIT_LOCALE"); l != "" {
		i18n.SetLocaleFunc(func() string { return l })
		defer i18n.SetLocaleFunc(func() string { return "zh-Hans" })
	}
	for _, dark := range []bool{false, true} {
		tt := newTester(t)
		tt.SetDark(dark)
		tt.click(t, "Merge the feature")
		tt.a.errMsg = "示例错误：这是一条错误提示"
		tt.Frame()
		name := out + "/light.png"
		if dark {
			name = out + "/dark.png"
		}
		f, _ := os.Create(name)
		png.Encode(f, tt.Image())
		f.Close()
	}
}
