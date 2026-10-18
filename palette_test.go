package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestFuzzy(t *testing.T) {
	if _, ok := fuzzy("mgo", "main.go"); !ok {
		t.Error("mgo doesn't match main.go")
	}
	if _, ok := fuzzy("xyz", "main.go"); ok {
		t.Error("xyz matches main.go")
	}
	// A file's name before a folder's, and letters together before apart.
	name, _ := fuzzy("view", "src/view.go")
	folder, _ := fuzzy("view", "view/src/main.go")
	if name >= folder {
		t.Errorf("name %d not before folder %d", name, folder)
	}
	together, _ := fuzzy("stage", "Stage All")
	apart, _ := fuzzy("stage", "Show the agent")
	if together >= apart {
		t.Errorf("together %d not before apart %d", together, apart)
	}
}

func TestPaletteFiles(t *testing.T) {
	tt := newTester(t)
	tt.commitFiles(t, "Three files", map[string]string{"web/app.ts": "x\n", "web/view.ts": "y\n", "main.go": "package main\n\nfunc main() { run() }\n"})
	tt.click(t, "Three files")
	tt.Key(ui.Cmd, ui.KeyP)
	tt.Frame()
	if !tt.a.palette.open || !tt.HasText("web/view.ts") {
		t.Fatalf("⌘P didn't list the files: %q", tt.Texts())
	}
	tt.a.palette.query = "vie"
	tt.Frame()
	if found := tt.a.palette.matches(); len(found) != 1 || found[0].label != "web/view.ts" {
		t.Fatalf("matches %v", found)
	}
	tt.click(t, "web/view.ts")
	if d := tt.a.detail; tt.a.palette.open || fileName(d.files[d.fileSel]) != "web/view.ts" {
		t.Fatalf("palette open %v, file %q", tt.a.palette.open, fileName(d.files[d.fileSel]))
	}
}

func TestPaletteCommands(t *testing.T) {
	tt := newTester(t)
	tt.Key(ui.Cmd, ui.KeyK)
	tt.Frame()
	if !tt.HasText("切换到 feature") || !tt.HasText("全部暂存") {
		t.Fatalf("⌘K's commands: %q", tt.Texts())
	}
	tt.Type("全部暂存")
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.settle()
	if len(tt.a.staged) != 1 || len(tt.a.unstaged) != 0 {
		t.Fatalf("staged %v, unstaged %v", tt.a.staged, tt.a.unstaged)
	}
}
