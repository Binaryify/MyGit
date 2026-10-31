package main

import (
	"context"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"mygit/internal/git"
	loc "mygit/locales"
)

// These walk through what people do, a step at a time, looking at what
// the window shows after each.

// write writes files of the tester's working tree, and refreshes.
func (tt *tester) write(t *testing.T, files map[string]string) {
	t.Helper()
	for name, text := range files {
		p := filepath.Join(tt.a.repo.Dir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tt.a.refresh()
	tt.settle()
}

func (tt *tester) shown() string {
	if ch := tt.a.detail.change; ch != nil {
		return ch.Path
	}
	return ""
}

func TestScenarioUnsavedEditThenAnotherFile(t *testing.T) {
	setup := func(t *testing.T) *tester {
		tt := newTester(t)
		tt.commitFiles(t, "Two", map[string]string{"a.txt": "a\n", "b.txt": "b\n"})
		tt.write(t, map[string]string{"a.txt": "a changed\n", "b.txt": "b changed\n"})
		tt.click(t, "a.txt")
		tt.a.detail.diff.New.Set("a edited\n")
		tt.Frame()
		tt.click(t, "b.txt")
		if !tt.a.confirm.open || tt.shown() != "a.txt" {
			t.Fatalf("went on without asking: shown %q", tt.shown())
		}
		return tt
	}
	disk := func(tt *tester, name string) string {
		b, _ := os.ReadFile(filepath.Join(tt.a.repo.Dir, name))
		return string(b)
	}
	t.Run("cancel", func(t *testing.T) {
		tt := setup(t)
		tt.click(t, "取消")
		if tt.shown() != "a.txt" || !tt.a.unsaved() || tt.a.unstaged[tt.a.unstagedSel].Path != "a.txt" {
			t.Fatalf("shown %q, unsaved %v, chosen %d", tt.shown(), tt.a.unsaved(), tt.a.unstagedSel)
		}
		// Clicking the file edited again asks nothing.
		tt.click(t, "a.txt")
		if tt.a.confirm.open {
			t.Fatal("asked choosing the file edited")
		}
	})
	t.Run("don't save", func(t *testing.T) {
		tt := setup(t)
		tt.click(t, "不保存")
		if tt.shown() != "b.txt" || disk(tt, "a.txt") != "a changed\n" {
			t.Fatalf("shown %q, a.txt %q", tt.shown(), disk(tt, "a.txt"))
		}
	})
	t.Run("save", func(t *testing.T) {
		tt := setup(t)
		tt.Key(0, ui.KeyEnter) // Save, the default
		tt.settle()
		if tt.shown() != "b.txt" || disk(tt, "a.txt") != "a edited\n" {
			t.Fatalf("shown %q, a.txt %q", tt.shown(), disk(tt, "a.txt"))
		}
	})
	t.Run("a commit", func(t *testing.T) {
		tt := setup(t)
		tt.click(t, "取消")
		tt.click(t, "Write main")
		if !tt.a.confirm.open || tt.a.detail.commit != nil {
			t.Fatal("showed a commit without asking")
		}
		tt.click(t, "不保存")
		if c := tt.a.detail.commit; c == nil || c.Subject != "Write main" {
			t.Fatalf("commit shown %+v", c)
		}
	})
}

func TestScenarioDiscardTheChangeShown(t *testing.T) {
	tt := newTester(t)
	tt.commitFiles(t, "Two", map[string]string{"a.txt": "a\n", "b.txt": "b\n"})
	tt.write(t, map[string]string{"a.txt": "a changed\n", "b.txt": "b changed\n"})
	tt.click(t, "b.txt")
	tt.a.discard(*tt.a.detail.change)
	tt.Frame()
	tt.Key(0, ui.KeyEnter) // Discard, the default
	tt.settle()
	if _, ok := find(tt.a.unstaged, "b.txt"); ok || tt.shown() != "" {
		t.Fatalf("after discarding: %v, shown %q", tt.a.unstaged, tt.shown())
	}
	tt.click(t, "a.txt")
	if tt.shown() != "a.txt" || tt.a.detail.diff == nil {
		t.Fatalf("a.txt not shown: %q", tt.shown())
	}
}

func TestScenarioDiscardAllChanges(t *testing.T) {
	tt := newTester(t)
	old := trash
	trash = os.Remove // what the trash does, without the trash
	t.Cleanup(func() { trash = old })
	tt.write(t, map[string]string{"main.go": "package main\n\n// changed\n", "new.txt": "new\n"})
	tt.click(t, "丢弃全部更改")
	tt.Frame()
	tt.Key(0, ui.KeyEnter) // Discard, the default
	tt.settle()
	if len(tt.a.unstaged) != 0 || tt.a.errMsg != "" {
		t.Fatalf("after discarding all: %v, %q", tt.a.unstaged, tt.a.errMsg)
	}
	b, err := os.ReadFile(filepath.Join(tt.a.repo.Dir, "main.go"))
	if err != nil || string(b) != "package main\n\nfunc main() {}\n" {
		t.Fatalf("main.go %q, %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(tt.a.repo.Dir, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("new.txt still there: %v", err)
	}
}

func TestScenarioCommitEverything(t *testing.T) {
	tt := newTester(t)
	tt.click(t, "notes.txt")
	tt.a.stageAll()
	tt.settle()
	tt.a.message = "Add the notes"
	tt.a.commit()
	tt.settle()
	if len(tt.a.staged)+len(tt.a.unstaged) != 0 || tt.a.message != "" || tt.shown() != "" {
		t.Fatalf("after the commit: staged %v, unstaged %v, message %q, shown %q", tt.a.staged, tt.a.unstaged, tt.a.message, tt.shown())
	}
	if tt.a.commits[0].Subject != "Add the notes" {
		t.Fatalf("history starts with %q", tt.a.commits[0].Subject)
	}
	// A new change, chosen, shows.
	tt.write(t, map[string]string{"notes.txt": "remember more\n"})
	tt.click(t, "notes.txt")
	if tt.shown() != "notes.txt" {
		t.Fatalf("the new change not shown: %q", tt.shown())
	}
}

func TestScenarioPartlyStaged(t *testing.T) {
	tt := newTester(t)
	tt.commitFiles(t, "Lines", map[string]string{"f.txt": "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"})
	tt.write(t, map[string]string{"f.txt": "one\n2\n3\n4\n5\n6\n7\n8\n9\nten\n"})
	tt.click(t, "f.txt")
	d := tt.a.detail
	if len(d.diff.Hunks()) != 2 {
		t.Fatalf("hunks %d", len(d.diff.Hunks()))
	}
	// Stage the first change alone: the file is in both lists, and the
	// worktree's side still shows, with the change left.
	tt.a.applyLines(tt.a.hunkLines(d.diff.Hunks()[0]), lineStage)
	tt.settle()
	if _, ok := find(tt.a.staged, "f.txt"); !ok {
		t.Fatal("not staged")
	}
	if _, ok := find(tt.a.unstaged, "f.txt"); !ok {
		t.Fatal("not left unstaged")
	}
	if tt.shown() != "f.txt" || tt.a.detail.staged || len(tt.a.detail.diff.Hunks()) != 1 {
		t.Fatalf("shown %q staged %v hunks %d", tt.shown(), tt.a.detail.staged, len(tt.a.detail.diff.Hunks()))
	}
	// Its staged side, chosen, shows the first change.
	tt.a.chooseChange(indexOf(tt.a.staged, "f.txt"), true)
	tt.settle()
	if !tt.a.detail.staged || tt.a.detail.diff == nil || !strings.Contains(tt.a.detail.raw, "+one") || strings.Contains(tt.a.detail.raw, "+ten") {
		t.Fatalf("staged side %q", tt.a.detail.raw)
	}
}

func TestScenarioRename(t *testing.T) {
	tt := newTester(t)
	tt.commitFiles(t, "Old", map[string]string{"old name.txt": "same\nlines\nhere\n"})
	cmd := exec.Command("git", "mv", "old name.txt", "新的 名字.txt")
	cmd.Dir = tt.a.repo.Dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	tt.a.refresh()
	tt.settle()
	ch, ok := find(tt.a.staged, "新的 名字.txt")
	if !ok || ch.Staged != 'R' || ch.OldPath != "old name.txt" {
		t.Fatalf("staged %+v", tt.a.staged)
	}
	tt.a.chooseChange(indexOf(tt.a.staged, ch.Path), true)
	tt.settle()
	if tt.shown() != "新的 名字.txt" || tt.a.errMsg != "" {
		t.Fatalf("shown %q, error %q", tt.shown(), tt.a.errMsg)
	}
	tt.a.unstage(ch)
	tt.settle()
	if len(tt.a.staged) != 0 {
		t.Fatalf("still staged: %v", tt.a.staged)
	}
}

func TestScenarioUntrackedAndBinary(t *testing.T) {
	tt := newTester(t)
	tt.write(t, map[string]string{"data.bin": "\x00\x01\x02binary\x00", "new.go": "package main\n"})
	tt.click(t, "new.go")
	if tt.a.detail.diff == nil || tt.a.detail.diff.New.String() != "package main\n" {
		t.Fatalf("untracked file: %+v", tt.a.detail)
	}
	tt.click(t, "data.bin")
	tt.Frame()
	if tt.shown() != "data.bin" || tt.a.errMsg != "" || !tt.HasText("二进制文件") || tt.a.detail.diff != nil {
		t.Fatalf("binary untracked: shown %q error %q: %q", tt.shown(), tt.a.errMsg, tt.Texts())
	}
	// Committed, then changed: a binary change says so.
	tt.commitFiles(t, "Data", nil)
	tt.write(t, map[string]string{"data.bin": "\x00\x09other\x00"})
	tt.click(t, "data.bin")
	tt.Frame()
	// It says so, and can't be edited as text, which would break it.
	if !tt.HasText("二进制文件") || tt.a.detail.whole || tt.a.detail.diff != nil {
		t.Errorf("a binary change: whole %v, texts %q", tt.a.detail.whole, tt.Texts())
	}
}

func TestScenarioDeletedPicture(t *testing.T) {
	tt := newTester(t)
	tt.commitFiles(t, "Logo", map[string]string{"logo.png": pngOf(t, 10, 10, color.White)})
	os.Remove(filepath.Join(tt.a.repo.Dir, "logo.png"))
	tt.a.refresh()
	tt.settle()
	tt.click(t, "logo.png")
	tt.settle()
	im := tt.a.detail.images
	if im == nil || im.err != "" || im.old == nil || im.new != nil {
		t.Fatalf("deleted picture: %+v", im)
	}
}

func TestScenarioEmptyRepository(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "-q", "-b", "main")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	os.WriteFile(filepath.Join(dir, "first.txt"), []byte("hello\n"), 0o644)
	repo, err := git.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(repo)
	tt := &tester{a: a, queue: make(chan func(), 64)}
	a.update = func(fn func()) { tt.queue <- fn }
	tt.Tester = ui.NewTester(a.view, 1200, 800)
	a.start()
	tt.settle()
	if a.errMsg != "" {
		t.Fatalf("an empty repository says %q", a.errMsg)
	}
	tt.click(t, "first.txt")
	if tt.shown() != "first.txt" {
		t.Fatalf("shown %q", tt.shown())
	}
	a.stageAll()
	tt.settle()
	tt.click(t, "first.txt")
	if tt.shown() != "first.txt" || !a.detail.staged || a.errMsg != "" {
		t.Fatalf("staged side of the first file: shown %q, error %q", tt.shown(), a.errMsg)
	}
	a.message = "First"
	a.commit()
	tt.settle()
	if a.errMsg != "" || len(a.commits) != 1 {
		t.Fatalf("first commit: %q, %d commits", a.errMsg, len(a.commits))
	}
	tt.Key(ui.Cmd, ui.KeyK)
	tt.Frame()
	if !tt.a.palette.open {
		t.Fatal("⌘K didn't open")
	}
}

func TestScenarioDetachedHead(t *testing.T) {
	tt := newTester(t)
	tt.click(t, "Write main")
	tt.a.run("checkout", func(ctx context.Context) error {
		return tt.a.repo.CheckoutDetached(ctx, tt.a.commits[tt.a.logSel].Hash)
	}, nil)
	tt.settle()
	if tt.a.status.Branch != "" || tt.a.errMsg != "" || !tt.HasText("⎇ （游离 HEAD） ▾") {
		t.Fatalf("branch %q, error %q: %q", tt.a.status.Branch, tt.a.errMsg, tt.Texts())
	}
	// Committing on it, then switching back to main, works.
	tt.write(t, map[string]string{"main.go": "package main\n\nfunc main() { detached() }\n"})
	tt.a.stageAll()
	tt.settle()
	tt.a.message = "On no branch"
	tt.a.commit()
	tt.settle()
	if tt.a.errMsg != "" || tt.a.commits[0].Subject != "On no branch" {
		t.Fatalf("commit on a detached HEAD: %q, %q", tt.a.errMsg, tt.a.commits[0].Subject)
	}
	tt.a.switchTo("main")
	tt.settle()
	if tt.a.status.Branch != "main" || tt.a.errMsg != "" {
		t.Fatalf("back to main: %q, %q", tt.a.status.Branch, tt.a.errMsg)
	}
}

func TestScenarioPaletteKeys(t *testing.T) {
	tt := newTester(t)
	tt.commitFiles(t, "Three", map[string]string{"x/a.go": "a\n", "x/b.go": "b\n", "x/c.go": "c\n"})
	tt.click(t, "Three")
	tt.Key(ui.Cmd, ui.KeyP)
	tt.Frame()
	tt.Type("x/")
	tt.Frame()
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.settle()
	d := tt.a.detail
	if tt.a.palette.open || d.fileSel < 0 || fileName(d.files[d.fileSel]) != "x/b.go" {
		t.Fatalf("open %v, chosen %d", tt.a.palette.open, d.fileSel)
	}
	if tt.a.fileList.Selected == nil || *tt.a.fileList.Selected != d.fileSel {
		t.Errorf("the files list doesn't show the file chosen")
	}
}

func TestScenarioFileChangedOnDiskWhileEdited(t *testing.T) {
	tt := newTester(t)
	tt.commitFiles(t, "One", map[string]string{"a.txt": "a\n"})
	tt.write(t, map[string]string{"a.txt": "a changed\n"})
	tt.click(t, "a.txt")
	tt.a.detail.diff.New.Set("my edit\n")
	tt.Frame()
	// Another editor, or git, writes the file meanwhile.
	tt.write(t, map[string]string{"a.txt": "theirs\n"})
	if !tt.a.unsaved() || tt.a.detail.diff.New.String() != "my edit\n" {
		t.Fatalf("the edit was lost as the file changed: %q", tt.a.detail.diff.New.String())
	}
	disk := func() string {
		b, _ := os.ReadFile(filepath.Join(tt.a.repo.Dir, "a.txt"))
		return string(b)
	}
	tt.a.saveFile()
	tt.Frame()
	if !tt.a.confirm.open || disk() != "theirs\n" {
		t.Fatalf("saved over the change on disk without asking: %q", disk())
	}
	tt.click(t, "取消")
	if disk() != "theirs\n" || !tt.a.unsaved() {
		t.Fatalf("after Cancel: %q, unsaved %v", disk(), tt.a.unsaved())
	}
	tt.a.saveFile()
	tt.Frame()
	tt.click(t, "仍然保存")
	if disk() != "my edit\n" || tt.a.unsaved() {
		t.Fatalf("after Save Anyway: %q, unsaved %v", disk(), tt.a.unsaved())
	}
	// Saved, the next edit saves without asking.
	tt.a.detail.diff.New.Set("my edit again\n")
	tt.a.saveFile()
	tt.settle()
	if tt.a.confirm.open || disk() != "my edit again\n" {
		t.Fatalf("asked saving over one's own save: %q", disk())
	}
}

func TestScenarioCommitWithAnUnsavedEdit(t *testing.T) {
	setup := func(t *testing.T) *tester {
		tt := newTester(t)
		tt.commitFiles(t, "One", map[string]string{"a.txt": "a\n"})
		tt.write(t, map[string]string{"a.txt": "a changed\n"})
		tt.click(t, "a.txt")
		tt.a.detail.diff.New.Set("a edited\n")
		tt.Frame()
		return tt
	}
	staged := func(tt *tester) string {
		out, _ := tt.a.repo.Git(context.Background(), "show", ":a.txt")
		return out
	}
	t.Run("stage all, saving", func(t *testing.T) {
		tt := setup(t)
		tt.a.stageAll()
		tt.Frame()
		if !tt.a.confirm.open {
			t.Fatal("staged without asking to save")
		}
		tt.click(t, "保存并继续")
		tt.settle()
		if staged(tt) != "a edited\n" || tt.a.unsaved() {
			t.Fatalf("staged %q", staged(tt))
		}
	})
	t.Run("stage the file, not saving", func(t *testing.T) {
		tt := setup(t)
		ch, _ := find(tt.a.unstaged, "a.txt")
		tt.a.stage(ch)
		tt.Frame()
		tt.click(t, "不保存，继续")
		tt.settle()
		if staged(tt) != "a changed\n" || tt.a.confirm.open {
			t.Fatalf("staged %q, asking %v", staged(tt), tt.a.confirm.open)
		}
	})
	t.Run("another file asks nothing", func(t *testing.T) {
		tt := setup(t)
		ch, _ := find(tt.a.unstaged, "notes.txt")
		tt.a.stage(ch)
		tt.settle()
		if tt.a.confirm.open {
			t.Fatal("asked staging another file")
		}
	})
	t.Run("commit, saving", func(t *testing.T) {
		tt := setup(t)
		// The file staged as it was, then edited again: committing asks,
		// and saving first commits what is staged, the edit left a change.
		tt.a.run("stage", func(ctx context.Context) error { return tt.a.repo.Stage(ctx, "a.txt") }, nil)
		tt.settle()
		tt.a.message = "Change a"
		tt.a.commit()
		tt.Frame()
		if !tt.a.confirm.open {
			t.Fatal("committed without asking to save")
		}
		tt.click(t, "保存并继续")
		tt.settle()
		if tt.a.errMsg != "" || tt.a.commits[0].Subject != "Change a" {
			t.Fatalf("commit: %q, %q", tt.a.errMsg, tt.a.commits[0].Subject)
		}
		b, _ := os.ReadFile(filepath.Join(tt.a.repo.Dir, "a.txt"))
		if string(b) != "a edited\n" {
			t.Fatalf("the edit wasn't saved: %q", b)
		}
	})
}

func TestScenarioChangeGoneWhileEdited(t *testing.T) {
	tt := newTester(t)
	tt.commitFiles(t, "One", map[string]string{"a.txt": "a\n"})
	tt.write(t, map[string]string{"a.txt": "a changed\n"})
	tt.click(t, "a.txt")
	tt.a.detail.diff.New.Set("a edited\n")
	tt.Frame()
	// git add in a terminal: the working tree's change is gone.
	if _, err := tt.a.repo.Git(context.Background(), "add", "a.txt"); err != nil {
		t.Fatal(err)
	}
	tt.a.refresh()
	tt.settle()
	if !tt.a.unsaved() || tt.a.detail.diff.New.String() != "a edited\n" {
		t.Fatalf("the edit went with the change: %+v", tt.a.detail)
	}
	tt.a.saveFile()
	tt.settle()
	b, _ := os.ReadFile(filepath.Join(tt.a.repo.Dir, "a.txt"))
	if string(b) != "a edited\n" || tt.shown() != "a.txt" {
		t.Fatalf("saved %q, shown %q", b, tt.shown())
	}
}

// F7 past a file's last change says so, and pressed again goes on to the
// next file at its first change, a commit's or along the staged changes
// and then the others; ⇧F7 comes back to the last change of the one before.
func TestScenarioStepAcrossFiles(t *testing.T) {
	long := func(changed ...int) string {
		var b strings.Builder
		for i := range 40 {
			if len(changed) > 0 && (i == changed[0] || i == changed[len(changed)-1]) {
				b.WriteString("changed ")
			}
			b.WriteString("line " + strconv.Itoa(i) + "\n")
		}
		return b.String()
	}
	tt := newTester(t)
	tt.commitFiles(t, "Long files", map[string]string{"a.txt": long(), "b.txt": long()})
	tt.commitFiles(t, "Change both", map[string]string{"a.txt": long(5, 30), "b.txt": long(5, 30)})
	tt.write(t, map[string]string{"a.txt": long(), "b.txt": long()})
	gitIn(t, tt.a.repo.Dir, "add", "a.txt")
	tt.a.refresh()
	tt.settle()

	type place struct {
		file  string
		hunk  int // -1 out of the changes
		armed int
	}
	at := func() place {
		d := tt.a.detail
		file := tt.shown()
		if d.commit != nil {
			file = fileName(d.files[d.fileSel])
		}
		h, inside := d.diff.CurrentHunk()
		if !inside {
			h = -1
		}
		return place{file, h, tt.a.armed()}
	}
	walk := func(steps []bool, want []place) {
		t.Helper()
		for i, back := range steps {
			tt.a.stepChange(back)
			tt.settle()
			if got := at(); got != want[i] {
				t.Fatalf("step %d (back %v): at %+v, want %+v", i, back, got, want[i])
			}
		}
	}
	steps := []bool{false, false, false, false, true, true}
	want := []place{{"a.txt", 0, 0}, {"a.txt", 1, 0}, {"a.txt", 1, 1}, {"b.txt", 0, 0}, {"b.txt", 0, -1}, {"a.txt", 1, 0}}

	tt.a.chooseChange(0, true) // a.txt, staged
	tt.settle()
	walk(steps, want)
	if !tt.a.detail.staged {
		t.Fatal("came back to a.txt's working tree, not its staged change")
	}
	// The last file has nothing after it to say.
	tt.a.chooseChange(indexOf(tt.a.unstaged, "b.txt"), false)
	tt.settle()
	walk([]bool{false, false, false}, []place{{"b.txt", 1, 0}, {"b.txt", 1, 0}, {"b.txt", 1, 0}})

	tt.click(t, "Change both")
	walk(steps, want)
}

// The filter shows the changes whose paths have its text, counts them
// out of all, chooses the one clicked, and staging all stages only those.
func TestScenarioFilterChanges(t *testing.T) {
	tt := newTester(t)
	tt.write(t, map[string]string{"src/a.go": "a\n", "src/b.go": "b\n", "docs/c.md": "c\n"})
	if len(tt.a.unstaged) != 4 { // notes.txt too
		t.Fatalf("changes %v", tt.a.unstaged)
	}
	tt.click(t, "按路径筛选更改")
	tt.Type("SRC/")
	tt.Frame()
	if !tt.HasText("2/4") || !tt.HasText("a.go") || tt.HasText("c.md") || tt.HasText("notes.txt") {
		t.Fatalf("filtered: %q", tt.Texts())
	}
	tt.click(t, "b.go")
	if tt.shown() != "src/b.go" {
		t.Fatalf("shown %q, chosen %d", tt.shown(), tt.a.unstagedSel)
	}
	tt.a.stageAll()
	tt.settle()
	var staged []string
	for _, c := range tt.a.staged {
		staged = append(staged, c.Path)
	}
	if strings.Join(staged, " ") != "src/a.go src/b.go" || len(tt.a.unstaged) != 2 {
		t.Fatalf("staged %v, unstaged %v", staged, tt.a.unstaged)
	}
	// The filter keeps to the new lists, and clearing it shows them all.
	if !tt.HasText("2/2") || !tt.HasText("0/2") {
		t.Fatalf("after staging: %q", tt.Texts())
	}
	tt.click(t, "清除搜索")
	if tt.a.filter.query != "" || !tt.HasText("c.md") || !tt.HasText("notes.txt") {
		t.Fatalf("cleared: %q", tt.Texts())
	}
	// The line between the staged changes and the others drags.
	head, ok := tt.Find(loc.StageAll()) // the button in the changes' header
	if !ok {
		t.Fatalf("no header of the changes: %q", tt.Texts())
	}
	before := tt.a.stagedPct
	y := head.Y - 6 // the split's line, just above the header
	tt.Press(100, y)
	tt.Move(100, y+120)
	tt.Release(100, y+120)
	tt.Frame()
	if tt.a.stagedPct <= before {
		t.Fatalf("dragged down, the staged changes take %v%%, from %v%%", tt.a.stagedPct, before)
	}
}
