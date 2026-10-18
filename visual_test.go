package main

import (
	"image/color"
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/i18n"
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

// TestDumpFeatures renders the views of the review features into
// MYGIT_DUMP, as TestDumpThemes does: a generated file folded, a picture's
// change, and the files of a commit some of them viewed.
func TestDumpFeatures(t *testing.T) {
	out := os.Getenv("MYGIT_DUMP")
	if out == "" {
		t.Skip()
	}
	save := func(tt *tester, name string) {
		tt.Frame()
		f, _ := os.Create(out + "/" + name + ".png")
		png.Encode(f, tt.Image())
		f.Close()
	}
	tt := newTester(t)
	tt.SetDark(true)
	tt.commitFiles(t, "Bump the deps and the logo", map[string]string{
		"go.sum":   "example.com/a v1.0.0 h1:abc=\nexample.com/b v1.2.0 h1:def=\n",
		"main.go":  "package main\n\nfunc main() { run() }\n",
		"logo.png": pngOf(t, 320, 200, color.RGBA{61, 123, 255, 255}),
	})
	tt.click(t, "Bump the deps and the logo")
	tt.a.toggleViewed(tt.a.detail.fileSel)
	for i, f := range tt.a.detail.files {
		if fileName(f) == "go.sum" {
			tt.a.showFile(i)
		}
	}
	save(tt, "folded")
	for i, f := range tt.a.detail.files {
		if fileName(f) == "logo.png" {
			tt.a.showFile(i)
		}
	}
	tt.settle()
	save(tt, "image")
}
