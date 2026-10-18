package main

import (
	"image/png"
	"os"
	"testing"
)

// TestDumpThemes renders the window in the light and dark themes into
// MYGIT_DUMP, to look at:
//
//	MYGIT_DUMP=/tmp go test -run DumpThemes
func TestDumpThemes(t *testing.T) {
	out := os.Getenv("MYGIT_DUMP")
	if out == "" {
		t.Skip()
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
