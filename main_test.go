package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mygit/internal/git"
)

// testRepo makes a repository with a merged branch and a change in its
// working tree.
func testRepo(t *testing.T) *git.Repo {
	t.Helper()
	dir := t.TempDir()
	sh := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Ada", "GIT_AUTHOR_EMAIL=a@example.com",
			"GIT_COMMITTER_NAME=Ada", "GIT_COMMITTER_EMAIL=a@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	file := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sh("init", "-q", "-b", "main")
	sh("config", "commit.gpgsign", "false")
	file("main.go", "package main\n")
	sh("add", ".")
	sh("commit", "-q", "-m", "Start")
	sh("switch", "-q", "-c", "feature")
	file("feature.go", "package main\n\nfunc feature() {}\n")
	sh("add", ".")
	sh("commit", "-q", "-m", "Add the feature")
	sh("switch", "-q", "main")
	file("main.go", "package main\n\nfunc main() {}\n")
	sh("commit", "-q", "-am", "Write main")
	sh("merge", "-q", "--no-ff", "-m", "Merge the feature", "feature")
	file("notes.txt", "remember\n")
	repo, err := git.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

// tester shows the app without a window, running what its workers hand
// the main thread as the window would.
type tester struct {
	*ui.Tester
	a     *app
	queue chan func()
}

func newTester(t *testing.T) *tester {
	a := newApp(testRepo(t))
	tt := &tester{a: a, queue: make(chan func(), 64)}
	a.update = func(fn func()) { tt.queue <- fn }
	tt.Tester = ui.NewTester(a.view, 1200, 800)
	a.start()
	tt.settle()
	return tt
}

// settle runs what the workers send until they are quiet.
func (tt *tester) settle() {
	for {
		select {
		case fn := <-tt.queue:
			fn()
			tt.Frame()
		case <-time.After(300 * time.Millisecond):
			tt.Frame()
			return
		}
	}
}

func (tt *tester) click(t *testing.T, s string) {
	t.Helper()
	if err := tt.Click(s); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.settle()
}

func TestHistory(t *testing.T) {
	tt := newTester(t)
	for _, s := range []string{"Merge the feature", "Write main", "Add the feature", "Start", "4 个提交", "main", "feature", "notes.txt"} {
		if !tt.HasText(s) {
			t.Fatalf("no %q in %q", s, tt.Texts())
		}
	}
	// The merge's graph: its second parent in a lane of its own.
	if tt.a.rows[0].Width != 2 || tt.a.rows[0].Lane != 0 {
		t.Errorf("merge row %+v", tt.a.rows[0])
	}
	tt.click(t, "Add the feature")
	if d := tt.a.detail; d.commit == nil || d.commit.Subject != "Add the feature" || len(d.files) != 1 || d.diff == nil {
		t.Fatalf("detail %+v", d)
	}
	if !tt.HasText("feature.go") {
		t.Errorf("no feature.go in %q", tt.Texts())
	}
}

func TestStageAndCommit(t *testing.T) {
	tt := newTester(t)
	tt.click(t, "notes.txt")
	if d := tt.a.detail; d.change == nil || d.change.Path != "notes.txt" || d.diff == nil {
		t.Fatalf("untracked file's detail %+v", d)
	}
	tt.click(t, "全部暂存")
	if len(tt.a.staged) != 1 || len(tt.a.unstaged) != 0 {
		t.Fatalf("staged %v, unstaged %v", tt.a.staged, tt.a.unstaged)
	}
	tt.click(t, "提交说明")
	tt.Type("Take notes")
	tt.click(t, "提交 1 个文件")
	if tt.a.errMsg != "" {
		t.Fatal(tt.a.errMsg)
	}
	if len(tt.a.commits) != 5 || tt.a.commits[0].Subject != "Take notes" || len(tt.a.staged) != 0 || tt.a.message != "" {
		t.Fatalf("after committing: %d commits, first %q, staged %v, message %q",
			len(tt.a.commits), tt.a.commits[0].Subject, tt.a.staged, tt.a.message)
	}
}

func TestAmendStartsFromLastMessage(t *testing.T) {
	tt := newTester(t)
	tt.click(t, "修改上次提交")
	if !tt.a.amend || !strings.HasPrefix(tt.a.message, "Merge the feature") {
		t.Fatalf("amend %v, message %q", tt.a.amend, tt.a.message)
	}
	tt.click(t, "修改上次提交")
	if tt.a.amend || tt.a.message != "" {
		t.Fatalf("amend %v, message %q", tt.a.amend, tt.a.message)
	}
}

func TestStageSelectedLinesAndHunks(t *testing.T) {
	tt := newTester(t)
	dir := tt.a.repo.Dir
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\n// A\nfunc main() {}\n\n// B\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tt.a.refresh()
	tt.settle()
	tt.click(t, "main.go")
	d := tt.a.detail.diff
	if tt.a.detail.patch == nil || d == nil || !d.LineSelection {
		t.Fatalf("detail %+v", tt.a.detail)
	}
	// Select "// A", the new side's third line, and stage it alone.
	d.SetSelectedLines(editor.DiffLineRange{Side: editor.DiffNew, Start: 2, End: 2}, true)
	tt.Frame()
	tt.click(t, "暂存选中行")
	index, err := tt.a.repo.Git(context.Background(), "show", ":main.go")
	if err != nil || index != "package main\n\n// A\nfunc main() {}\n" {
		t.Fatalf("index %q, %v", index, err)
	}
	if len(tt.a.staged) != 1 || len(tt.a.unstaged) != 2 { // main.go and notes.txt
		t.Fatalf("staged %v, unstaged %v", tt.a.staged, tt.a.unstaged)
	}
	// Discard what is left, "// B", through its change's button.
	if tt.a.detail.staged || tt.a.detail.patch == nil {
		t.Fatalf("detail after staging %+v", tt.a.detail)
	}
	tt.a.applyLines(tt.a.hunkLines(tt.a.detail.diff.Hunks()[0]), lineDiscard)
	tt.Frame()
	tt.click(t, "丢弃")
	b, _ := os.ReadFile(filepath.Join(dir, "main.go"))
	if string(b) != index {
		t.Fatalf("working tree %q", b)
	}
}

func TestSearchAndScope(t *testing.T) {
	tt := newTester(t)
	tt.click(t, "搜索历史")
	tt.Type("FEATURE")
	tt.settle()
	if n := tt.a.logCount(); n != 2 || !tt.HasText("2 / 4 个提交") || tt.HasText("Write main") {
		t.Fatalf("%d matches, texts %q", n, tt.Texts())
	}
	tt.click(t, "Add the feature")
	if d := tt.a.detail; d.commit == nil || d.commit.Subject != "Add the feature" {
		t.Fatalf("chose %+v", d.commit)
	}
	tt.a.search = ""
	tt.a.setScope(git.Scope{Path: "feature.go"})
	tt.settle()
	if len(tt.a.commits) != 1 || !tt.HasText("只看改动了 feature.go 的提交") {
		t.Fatalf("history of feature.go: %d commits", len(tt.a.commits))
	}
	tt.click(t, "显示全部")
	if len(tt.a.commits) != 4 {
		t.Fatalf("all of it again: %d commits", len(tt.a.commits))
	}
}

func TestBranchFromCommitAndDelete(t *testing.T) {
	tt := newTester(t)
	if err := tt.RightClick("Write main"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("从此提交新建分支…"); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	tt.settle()
	tt.Type("hotfix")
	tt.click(t, "创建并切换")
	if tt.a.status.Branch != "hotfix" || tt.a.errMsg != "" {
		t.Fatalf("branch %q, error %q", tt.a.status.Branch, tt.a.errMsg)
	}
	// main has commits hotfix doesn't: deleting it asks first.
	tt.a.deleteBranch("main")
	tt.settle()
	if !tt.a.confirm.open {
		t.Fatal("deleted an unmerged branch without asking")
	}
	tt.click(t, "仍然删除")
	if b, _ := tt.a.repo.Branches(context.Background()); len(b) != 2 || b[0] != "feature" || b[1] != "hotfix" {
		t.Fatalf("branches %v", b)
	}
	// Merged into hotfix, feature goes at once.
	tt.a.run("合并", func(ctx context.Context) error { return tt.a.repo.Merge(ctx, "feature") }, nil)
	tt.settle()
	tt.a.deleteBranch("feature")
	tt.settle()
	if b, _ := tt.a.repo.Branches(context.Background()); len(b) != 1 || tt.a.confirm.open {
		t.Fatalf("branches %v, asking %v", b, tt.a.confirm.open)
	}
}

func TestResetHardAsks(t *testing.T) {
	tt := newTester(t)
	if err := tt.RightClick("Start"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("将 main 重置到此", "Hard：丢弃所有更改…"); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	tt.settle()
	tt.click(t, "硬重置")
	main, _ := tt.a.repo.Git(context.Background(), "log", "-1", "--format=%s", "main")
	if strings.TrimSpace(main) != "Start" || len(tt.a.commits) != 2 { // Start, and feature's commit
		t.Fatalf("main at %q, %d commits", main, len(tt.a.commits))
	}
}
