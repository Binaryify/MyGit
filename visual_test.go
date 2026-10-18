package main

import (
	"image/color"
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui/editor"
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
	tt.a.openCommands()
	tt.a.palette.query = "分支"
	save(tt, "commands")
	tt.a.palette.open = false
	for i, f := range tt.a.detail.files {
		if fileName(f) == "main.go" {
			tt.a.showFile(i)
		}
	}
	tt.Frame()
	tt.a.startComment(editor.DiffLineRange{Side: editor.DiffNew, Start: 2, End: 2})
	tt.a.composer.draft.Body = "run 还没定义，先补上再调用；另外这里最好处理一下返回的错误。"
	tt.a.saveComment()
	save(tt, "comment")
}

// TestDumpPull renders a pull request's review into MYGIT_DUMP: its
// header, a thread under its line and a comment of the review pending.
func TestDumpPull(t *testing.T) {
	out := os.Getenv("MYGIT_DUMP")
	if out == "" {
		t.Skip()
	}
	tt, _ := pullTester(t)
	tt.SetDark(true)
	tt.a.openPull(7)
	tt.settle()
	line, _ := tt.a.toDiff(editor.DiffNew, 3)
	tt.a.detail.diff.OnGutterButton(editor.DiffLineRange{Side: editor.DiffNew, Start: line, End: line})
	tt.a.composer.draft.Body = "导出它，别的包要用。"
	tt.a.saveComment()
	tt.Frame()
	f, _ := os.Create(out + "/pull.png")
	png.Encode(f, tt.Image())
	f.Close()
	tt.a.openPulls()
	tt.settle()
	f, _ = os.Create(out + "/pulls.png")
	png.Encode(f, tt.Image())
	f.Close()
}
