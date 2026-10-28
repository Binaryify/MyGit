package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mygit/internal/git"
	loc "mygit/locales"
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

// settle runs what the workers send until they are quiet and the app has
// nothing running, as git takes its time on Windows.
func (tt *tester) settle() {
	a := tt.a
	deadline := time.Now().Add(30 * time.Second)
	for {
		select {
		case fn := <-tt.queue:
			fn()
			tt.Frame()
		case <-time.After(300 * time.Millisecond):
			tt.Frame()
			if (a.busy == "" && !a.refreshing && !a.loading && !a.detail.loading) || time.Now().After(deadline) {
				return
			}
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

func TestUpBringsBackRecentMessages(t *testing.T) {
	tt := newTester(t)
	tt.click(t, "提交说明")
	for _, step := range []struct {
		key  ui.Key
		want string
	}{
		{ui.KeyUp, "Merge the feature"},
		{ui.KeyUp, "Write main"},
		{ui.KeyDown, "Merge the feature"},
		{ui.KeyDown, ""},
		{ui.KeyDown, ""},
		{ui.KeyUp, "Merge the feature"},
	} {
		tt.Key(0, step.key)
		if !strings.HasPrefix(tt.a.message, step.want) || step.want == "" && tt.a.message != "" {
			t.Fatalf("after %v: message %q, want %q", step.key, tt.a.message, step.want)
		}
	}
	// The caret after it, which typing goes on from; changed, the arrows
	// move the caret again.
	tt.Type("!")
	tt.Key(0, ui.KeyUp)
	if !strings.HasSuffix(tt.a.message, "!") {
		t.Fatalf("message %q", tt.a.message)
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
	if strings.ReplaceAll(string(b), "\r\n", "\n") != index { // CRLF with core.autocrlf
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
	tt.a.setScope(git.Scope{Ref: "HEAD", Path: "feature.go"})
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
	if strings.TrimSpace(main) != "Start" || len(tt.a.commits) != 1 { // main's history: Start alone
		t.Fatalf("main at %q, %d commits", main, len(tt.a.commits))
	}
}

func TestStashFlow(t *testing.T) {
	tt := newTester(t)
	tt.click(t, "Stash (0)")
	tt.click(t, "Stash 当前更改…")
	tt.click(t, "Stash")
	if len(tt.a.stashes) != 1 || len(tt.a.unstaged) != 0 {
		t.Fatalf("stashes %v, unstaged %v", tt.a.stashes, tt.a.unstaged)
	}
	tt.click(t, tt.a.stashes[0].Subject)
	if d := tt.a.detail; d.commit == nil || len(d.files) != 1 || d.files[0].NewName != "notes.txt" {
		t.Fatalf("stash detail %+v", d)
	}
	if err := tt.RightClick(tt.a.stashes[0].Subject); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("弹出（应用并删除）"); err != nil {
		t.Fatal(err)
	}
	tt.settle()
	if len(tt.a.stashes) != 0 || len(tt.a.unstaged) != 1 {
		t.Fatalf("after pop: stashes %v, unstaged %v", tt.a.stashes, tt.a.unstaged)
	}
}

func TestConflictBar(t *testing.T) {
	tt := newTester(t)
	ctx := context.Background()
	r := tt.a.repo
	for _, args := range [][]string{
		{"switch", "-q", "-c", "other", "HEAD~1"},
		{"-c", "user.name=a", "-c", "user.email=a@a", "commit", "-q", "--allow-empty", "-m", "noop"},
	} {
		if _, err := r.Git(ctx, args...); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(r.Dir, "main.go"), []byte("package other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Git(ctx, "-c", "user.name=a", "-c", "user.email=a@a", "commit", "-q", "-am", "other main"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Git(ctx, "switch", "-q", "main"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.Dir, "main.go"), []byte("package mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Git(ctx, "-c", "user.name=a", "-c", "user.email=a@a", "commit", "-q", "-am", "my main"); err != nil {
		t.Fatal(err)
	}
	tt.a.run("合并", func(ctx context.Context) error { return r.Merge(ctx, "other") }, nil)
	tt.settle()
	if tt.a.op != git.Merging || !tt.HasText("正在合并，有 1 个冲突文件：解决后暂存，再点继续") {
		t.Fatalf("op %q, texts %q", tt.a.op, tt.Texts())
	}
	// Resolve it in the merge view with their side, and go on.
	tt.click(t, "main.go")
	m := tt.a.detail.merge
	if m == nil || m.model.Unresolved() != 1 || !tt.HasText("冲突 1 / 1") {
		t.Fatalf("merge %+v, texts %q", m, tt.Texts())
	}
	ours, result, theirs := m.panes[paneOurs].Buffer, m.panes[paneResult].Buffer, m.panes[paneTheirs].Buffer
	if ours.String() != "package mine\n" || theirs.String() != "package other\n" || result.String() != "package main\n\nfunc main() {}\n" {
		t.Fatalf("panes %q, %q, %q", ours.String(), result.String(), theirs.String())
	}
	tt.click(t, "采用右侧")
	if result.String() != "package other\n" || !tt.HasText("没有剩余冲突") {
		t.Fatalf("result %q", result.String())
	}
	tt.click(t, "保存并标记为已解决")
	if tt.a.errMsg != "" || len(tt.a.staged) != 1 || tt.HasText("main.go 有冲突") {
		t.Fatalf("error %q, staged %v", tt.a.errMsg, tt.a.staged)
	}
	tt.click(t, "继续")
	if tt.a.op != git.NoOperation || tt.a.errMsg != "" {
		t.Fatalf("op after continuing %q, error %q", tt.a.op, tt.a.errMsg)
	}
	if b, _ := os.ReadFile(filepath.Join(r.Dir, "main.go")); string(b) != "package other\n" {
		t.Fatalf("main.go %q", b)
	}
}

func TestMove(t *testing.T) {
	for _, c := range []struct {
		rows []int
		to   int
		want string
	}{
		{[]int{0}, 3, "bca"}, {[]int{2}, 0, "cab"}, {[]int{0, 2}, 2, "bac"}, {[]int{1}, 1, "abc"},
	} {
		if got := string(move([]byte("abc"), c.rows, c.to)); got != c.want {
			t.Errorf("move %v to %d: %q, want %q", c.rows, c.to, got, c.want)
		}
	}
}

func TestInteractiveRebaseDialog(t *testing.T) {
	tt := newTester(t)
	if _, err := tt.a.repo.Git(context.Background(), "switch", "-q", "feature"); err != nil {
		t.Fatal(err)
	}
	if _, err := tt.a.repo.Git(context.Background(), "-c", "user.name=a", "-c", "user.email=a@a", "commit", "-q", "--allow-empty", "-m", "Polish"); err != nil {
		t.Fatal(err)
	}
	tt.a.refresh()
	tt.settle()
	if err := tt.RightClick("Add the feature"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("从此处交互式变基…"); err != nil {
		t.Fatal(err)
	}
	tt.settle()
	p := &tt.a.rebase
	if !p.open || len(p.steps) != 2 || p.steps[1].Subject != "Polish" {
		t.Fatalf("plan %+v", p)
	}
	p.steps[1].Action = git.Fixup
	tt.Frame()
	tt.click(t, "开始变基")
	out, _ := tt.a.repo.Git(context.Background(), "log", "--format=%s", "-2")
	if tt.a.errMsg != "" || out != "Add the feature\nStart\n" {
		t.Fatalf("error %q, log %q", tt.a.errMsg, out)
	}
}

func TestEditCommitMessage(t *testing.T) {
	tt := newTester(t)
	ctx := context.Background()
	if _, err := tt.a.repo.Git(ctx, "switch", "-q", "feature"); err != nil {
		t.Fatal(err)
	}
	if _, err := tt.a.repo.Git(ctx, "-c", "user.name=a", "-c", "user.email=a@a", "commit", "-q", "--allow-empty", "-m", "Polish"); err != nil {
		t.Fatal(err)
	}
	tt.a.refresh()
	tt.settle()
	if err := tt.RightClick("Add the feature"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("编辑提交消息…"); err != nil {
		t.Fatal(err)
	}
	tt.settle()
	p := &tt.a.reword
	if !p.open || p.pushed || p.message != "Add the feature" {
		t.Fatalf("plan %+v", p)
	}
	p.message = "Add the feature, reworded"
	tt.Frame()
	tt.click(t, "保存")
	tt.settle()
	out, _ := tt.a.repo.Git(ctx, "log", "--format=%s", "-3")
	if tt.a.errMsg != "" || out != "Polish\nAdd the feature, reworded\nStart\n" {
		t.Fatalf("error %q, log %q", tt.a.errMsg, out)
	}
}

func TestSquashCommits(t *testing.T) {
	tt := newTester(t)
	ctx := context.Background()
	if _, err := tt.a.repo.Git(ctx, "switch", "-q", "feature"); err != nil {
		t.Fatal(err)
	}
	for _, msg := range []string{"Polish", "Tidy"} {
		if _, err := tt.a.repo.Git(ctx, "-c", "user.name=a", "-c", "user.email=a@a", "commit", "-q", "--allow-empty", "-m", msg); err != nil {
			t.Fatal(err)
		}
	}
	tt.a.refresh()
	tt.settle()
	tt.click(t, "Add the feature")
	if err := tt.ClickWith(ui.Cmd, "Tidy"); err != nil {
		t.Fatal(err)
	}
	tt.settle()
	if err := tt.RightClick("Tidy"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("压缩 2 个提交…"); err != nil {
		t.Fatal(err)
	}
	tt.settle()
	if p := tt.a.reword; !p.open || p.message != "Add the feature\n\nTidy" {
		t.Fatalf("plan %+v", p)
	}
	tt.click(t, "压缩")
	tt.settle()
	out, _ := tt.a.repo.Git(ctx, "log", "--format=%s", "-3")
	if tt.a.errMsg != "" || out != "Polish\nAdd the feature\nStart\n" || tt.a.logChosen.Len() != 0 {
		t.Fatalf("error %q, log %q", tt.a.errMsg, out)
	}
	if body := tt.a.repo.Message(ctx, "HEAD~1"); body != "Add the feature\n\nTidy" {
		t.Fatalf("message %q", body)
	}
}

func TestBlameAndParents(t *testing.T) {
	tt := newTester(t)
	tt.click(t, "Merge the feature")
	d := tt.a.detail
	if d.commit == nil || len(d.files) != 1 || d.files[0].NewName != "feature.go" || !tt.HasText("对比父提交 1（"+d.commit.Parents[0][:8]+"）") {
		t.Fatalf("merge against its first parent: %+v", d.files)
	}
	tt.a.showParent(1)
	tt.settle()
	if d := tt.a.detail; len(d.files) != 1 || d.files[0].NewName != "main.go" {
		t.Fatalf("merge against its second parent: %+v", d.files)
	}
	if err := tt.RightClick("main.go"); err != nil { // the merge's file
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Blame（逐行追溯）"); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	tt.settle()
	b := tt.a.detail.blame
	if b == nil || len(b.lines) != 3 || b.lines[2].Summary != "Write main" {
		t.Fatalf("blame %+v", b)
	}
	tt.a.revealCommit(b.lines[0].Hash)
	tt.settle()
	if d := tt.a.detail; d.commit == nil || d.commit.Subject != "Start" {
		t.Fatalf("revealed %+v", d.commit)
	}
}

func TestPushDialog(t *testing.T) {
	tt := newTester(t)
	origin := t.TempDir()
	ctx := context.Background()
	if out, err := exec.Command("git", "init", "-q", "--bare", origin).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if _, err := tt.a.repo.Git(ctx, "remote", "add", "origin", origin); err != nil {
		t.Fatal(err)
	}
	tt.a.refresh()
	tt.settle()
	tt.click(t, "推送…")
	if !tt.HasText("推送 main → origin/main（新建）") || !tt.HasText("4 个提交") {
		t.Fatalf("texts %q", tt.Texts())
	}
	tt.click(t, "推送")
	if tt.a.errMsg != "" || tt.a.status.Upstream != "origin/main" {
		t.Fatalf("error %q, upstream %q", tt.a.errMsg, tt.a.status.Upstream)
	}
}

func TestPullStashesUntrackedInTheWay(t *testing.T) {
	tt := newTester(t)
	ctx := context.Background()
	origin, other := t.TempDir(), filepath.Join(t.TempDir(), "other")
	sh := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	sh(origin, "init", "-q", "--bare", "-b", "main")
	sh(tt.a.repo.Dir, "remote", "add", "origin", origin)
	sh(tt.a.repo.Dir, "push", "-q", "-u", "origin", "main")
	sh(origin, "clone", "-q", origin, other)
	for _, name := range []string{"local.cfg", "配置.txt"} {
		if err := os.WriteFile(filepath.Join(other, name), []byte("theirs"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sh(other, "add", ".")
	sh(other, "-c", "user.name=b", "-c", "user.email=b@b", "commit", "-qm", "Add the config")
	sh(other, "push", "-q")
	sh(tt.a.repo.Dir, "fetch", "-q")
	for _, name := range []string{"local.cfg", "配置.txt", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(tt.a.repo.Dir, name), []byte("mine"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tt.a.refresh()
	tt.settle()
	tt.a.runPull("拉取", func(ctx context.Context) error { return tt.a.repo.Pull(ctx, git.PullFastForward) })
	tt.settle()
	if q := tt.a.confirm; !q.open || q.message != loc.UntrackedInTheWayMessage("local.cfg\n配置.txt") || tt.a.errMsg != "" {
		t.Fatalf("asking %+v, error %q", q, tt.a.errMsg)
	}
	tt.click(t, "存入 stash 并拉取")
	tt.settle()
	if out, _ := tt.a.repo.Git(ctx, "log", "-1", "--format=%s"); tt.a.errMsg != "" || out != "Add the config\n" {
		t.Fatalf("error %q, HEAD %q", tt.a.errMsg, out)
	}
	for name, want := range map[string]string{"local.cfg": "theirs", "配置.txt": "theirs", "notes.txt": "mine"} {
		if b, _ := os.ReadFile(filepath.Join(tt.a.repo.Dir, name)); string(b) != want {
			t.Fatalf("%s: %q", name, b)
		}
	}
	if out, _ := tt.a.repo.Git(ctx, "show", "stash@{0}^3:local.cfg"); out != "mine" {
		t.Fatalf("stashed local.cfg %q", out)
	}
}

func TestFoldUnchanged(t *testing.T) {
	defer func(on bool) { prefs.FoldUnchanged = on }(prefs.FoldUnchanged)
	prefs.FoldUnchanged = false
	tt := newTester(t)
	ctx := context.Background()
	var lines []string
	for i := 1; i <= 40; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	path := filepath.Join(tt.a.repo.Dir, "long.txt")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := tt.a.repo.Git(ctx, "add", "long.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := tt.a.repo.Git(ctx, "-c", "user.name=a", "-c", "user.email=a@a", "commit", "-qm", "Long"); err != nil {
		t.Fatal(err)
	}
	lines[29] = "line thirty"
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tt.a.refresh()
	tt.settle()
	tt.click(t, "long.txt")
	tt.settle()
	// Folded, the lines away from the change give way to separators that
	// expand them.
	whole := tt.Texts()
	if !tt.a.detail.whole || !tt.a.detail.diff.ExpandUnchanged {
		t.Fatalf("whole %v", tt.a.detail.whole)
	}
	tt.click(t, "折叠未更改")
	tt.Frame()
	if folded := tt.Texts(); !prefs.FoldUnchanged || len(folded) <= len(whole) {
		t.Fatalf("folded %v, texts %q", prefs.FoldUnchanged, folded)
	}
	tt.click(t, "折叠未更改")
	tt.Frame()
	if prefs.FoldUnchanged || len(tt.Texts()) != len(whole) {
		t.Fatalf("unfolded: texts %q", tt.Texts())
	}
}

func TestOverview(t *testing.T) {
	tt := newTester(t)
	ctx := context.Background()
	var lines []string
	for i := 1; i <= 60; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	path := filepath.Join(tt.a.repo.Dir, "long.txt")
	write := func() {
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write()
	if _, err := tt.a.repo.Git(ctx, "add", "long.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := tt.a.repo.Git(ctx, "-c", "user.name=a", "-c", "user.email=a@a", "commit", "-qm", "Long"); err != nil {
		t.Fatal(err)
	}
	lines[4] = "line five"
	lines = append(lines[:49], lines[50:]...) // line 50 goes
	write()
	tt.a.refresh()
	tt.settle()
	tt.click(t, "long.txt")
	tt.settle()
	d := tt.a.detail.diff
	h := d.Hunks()
	if len(h) != 2 || hunkPreview(d, h[0]) != "- line 5\n+ line five" || hunkPreview(d, h[1]) != "- line 50" {
		t.Fatalf("hunks %+v", h)
	}
	tt.click(t, "第 2/2 处")
	tt.Frame()
	if i, inside := d.CurrentHunk(); i != 1 || !inside {
		t.Fatalf("at change %d, inside %v", i, inside)
	}
}

func TestUnpushedCommits(t *testing.T) {
	tt := newTester(t)
	ctx := context.Background()
	origin := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "--bare", "-b", "main", origin}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
	}
	for _, args := range [][]string{
		{"remote", "add", "origin", origin},
		{"push", "-q", "-u", "origin", "main"},
		{"-c", "user.name=a", "-c", "user.email=a@a", "commit", "-q", "--allow-empty", "-m", "Polish"},
	} {
		if _, err := tt.a.repo.Git(ctx, args...); err != nil {
			t.Fatal(err)
		}
	}
	tt.a.refresh()
	tt.settle()
	head, _ := tt.a.repo.Git(ctx, "rev-parse", "HEAD")
	if len(tt.a.unpushed) != 1 || !tt.a.unpushed[strings.TrimSpace(head)] || !tt.HasText("Polish") {
		t.Fatalf("unpushed %v", tt.a.unpushed)
	}
	if _, err := tt.a.repo.Git(ctx, "push", "-q"); err != nil {
		t.Fatal(err)
	}
	tt.a.refresh()
	tt.settle()
	if len(tt.a.unpushed) != 0 {
		t.Fatalf("unpushed after pushing %v", tt.a.unpushed)
	}
}

func TestWelcome(t *testing.T) {
	dir := t.TempDir()
	w := &welcome{recent: []string{filepath.Join(dir, "project")}}
	tt := ui.NewTester(w.view, 520, 600)
	for _, s := range []string{"MyGit", "把仓库文件夹拖到这里", "或点击选择文件夹", "最近打开", "project"} {
		if !tt.HasText(s) {
			t.Fatalf("no %q in %q", s, tt.Texts())
		}
	}
	w.open(dir) // not a repository
	tt.Frame()
	if w.errMsg == "" || !tt.HasText(w.errMsg) {
		t.Fatalf("error %q, texts %q", w.errMsg, tt.Texts())
	}
}

func TestCredentialDialog(t *testing.T) {
	tt := newTester(t)
	done := make(chan credAnswer, 1)
	tt.a.askCredential("Password for 'https://ada@example.com': ", done)
	tt.Frame()
	if !tt.HasText("git 需要验证") {
		t.Fatalf("texts %q", tt.Texts())
	}
	tt.Type("ghp_token")
	tt.click(t, "确定")
	if r := <-done; !r.ok || r.value != "ghp_token" {
		t.Fatalf("answer %+v", r)
	}
	tt.a.askCredential("Username for 'https://example.com': ", done)
	tt.Frame()
	tt.click(t, "取消")
	if r := <-done; r.ok {
		t.Fatalf("canceled answer %+v", r)
	}
}

func TestEditInDiff(t *testing.T) {
	tt := newTester(t)
	dir := tt.a.repo.Dir
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() { println(1) }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tt.a.refresh()
	tt.settle()
	tt.click(t, "main.go")
	d := tt.a.detail.diff
	if !tt.a.detail.whole || d == nil || !d.Editable || d.Old.String() != "package main\n\nfunc main() {}\n" {
		t.Fatalf("detail %+v", tt.a.detail)
	}
	d.New.Edit(editor.Edit{Start: d.New.Len(), End: d.New.Len(), Text: "// edited\n"})
	tt.Frame()
	if !tt.a.unsaved() || !tt.HasText("保存") {
		t.Fatal("no unsaved edit")
	}
	tt.click(t, "保存")
	b, _ := os.ReadFile(filepath.Join(dir, "main.go"))
	if string(b) != "package main\n\nfunc main() { println(1) }\n// edited\n" || tt.a.unsaved() {
		t.Fatalf("file %q", b)
	}
	if tt.a.detail.diff != d {
		t.Fatal("saving replaced the diff, losing its place")
	}
}

func TestSettingsLayout(t *testing.T) {
	tt := newTester(t)
	tt.click(t, "Write main")
	prefs.Unified = true
	defer func() { prefs.Unified = false }()
	tt.a.reloadDiff()
	tt.settle()
	if tt.a.detail.diff == nil || tt.a.detail.diff.Layout != editor.DiffUnified {
		t.Fatalf("layout %+v", tt.a.detail.diff)
	}
}

func TestCompare(t *testing.T) {
	tt := newTester(t)
	tt.click(t, "Start")
	if err := tt.ClickWith(ui.Cmd, "Write main"); err != nil {
		t.Fatal(err)
	}
	tt.settle()
	d := tt.a.detail
	if d.commit == nil || !comparing(d.commit) || len(d.files) != 1 || d.files[0].NewName != "main.go" {
		t.Fatalf("comparison %+v, files %+v", d.commit, d.files)
	}
	if !strings.HasPrefix(d.commit.Subject, "比较 ") || !tt.HasText("1 个文件有差异") {
		t.Fatalf("texts %q", tt.Texts())
	}
	// A commit against the working tree, which has no changes to tracked
	// files: what the commits after it changed.
	tt.a.showCompare("x", tt.a.commits[3].Hash, "")
	tt.settle()
	if n := len(tt.a.detail.files); n != 2 {
		t.Fatalf("Start against the working tree: %d files", n)
	}
}

func TestIgnore(t *testing.T) {
	tt := newTester(t)
	if err := tt.RightClick("notes.txt"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("添加到 .gitignore", "*.txt"); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	tt.settle()
	b, _ := os.ReadFile(filepath.Join(tt.a.repo.Dir, ".gitignore"))
	if string(b) != "*.txt\n" {
		t.Fatalf(".gitignore %q", b)
	}
	// notes.txt is ignored now; .gitignore itself is the change.
	if len(tt.a.unstaged) != 1 || tt.a.unstaged[0].Path != ".gitignore" {
		t.Fatalf("changes %+v", tt.a.unstaged)
	}
	if err := appendIgnore(context.Background(), tt.a.repo, "*.txt"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(tt.a.repo.Dir, ".gitignore")); string(b) != "*.txt\n" {
		t.Fatalf("added twice: %q", b)
	}
}

func TestWelcomeOffersTrust(t *testing.T) {
	w := &welcome{}
	tt := ui.NewTester(w.view, 520, 600)
	w.failed("/mnt/disk/repo", &git.UnsafeError{Dir: "/mnt/disk/repo"})
	tt.Frame()
	if w.unsafe == "" || !tt.HasText("信任并打开") || !tt.HasText(w.errMsg) {
		t.Fatalf("texts %q", tt.Texts())
	}
	w.failed("/tmp", errors.New("/tmp 不是 git 仓库"))
	tt.Frame()
	if w.unsafe != "" || tt.HasText("信任并打开") {
		t.Fatal("offers to trust a folder that isn't a repository")
	}
}

// The sections' headers stay put as files are staged and unstaged.
func TestHeadersDontMove(t *testing.T) {
	tt := newTester(t)
	header := func() ui.Rect {
		t.Helper()
		r, ok := tt.Find("暂存的更改")
		if !ok {
			t.Fatalf("no header in %q", tt.Texts())
		}
		return r
	}
	before := header()
	fileBefore, _ := tt.Find("notes.txt")
	tt.click(t, "全部暂存")
	staged := header()
	tt.click(t, "全部取消暂存")
	after := header()
	fileAfter, _ := tt.Find("notes.txt")
	if before != staged || staged != after || fileBefore.Y != fileAfter.Y {
		t.Fatalf("header at %+v, %+v, %+v; file %+v → %+v", before, staged, after, fileBefore, fileAfter)
	}
}

func TestDropTarget(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if dropTarget(dir) != dir || dropTarget(file) != dir {
		t.Fatalf("folder %q, file %q", dropTarget(dir), dropTarget(file))
	}
}

// A folder that isn't a repository, dropped on a repository's window,
// says so there rather than opening the welcome window.
func TestDropNotARepo(t *testing.T) {
	tt := newTester(t)
	dir := t.TempDir()
	tt.a.openDropped([]string{dir})
	tt.Frame()
	if !strings.Contains(tt.a.errMsg, "不是 git 仓库") || !tt.HasText(tt.a.errMsg) || welcomeWin != nil {
		t.Fatalf("error %q, welcome %v", tt.a.errMsg, welcomeWin != nil)
	}
	tt.click(t, "关闭")
	if tt.a.errMsg != "" {
		t.Fatal("error stays")
	}
}

func TestBranchesPopoverAndKeys(t *testing.T) {
	tt := newTester(t)
	tt.click(t, "分支")
	if !tt.a.branchesOpen || !tt.HasText("本地分支") || !tt.HasText("feature") {
		t.Fatalf("texts %q", tt.Texts())
	}
	tt.Type("feat")
	tt.Frame()
	if tt.HasText("main") && !tt.HasText("⎇ main ▾") {
		t.Fatalf("main still listed: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	tt.settle()
	if tt.a.status.Branch != "feature" || tt.a.branchesOpen {
		t.Fatalf("branch %q, open %v", tt.a.status.Branch, tt.a.branchesOpen)
	}
	// Space stages the change chosen in the list.
	tt.click(t, "notes.txt")
	tt.Key(0, ui.KeySpace)
	tt.settle()
	if len(tt.a.staged) != 1 || tt.a.staged[0].Path != "notes.txt" {
		t.Fatalf("staged %+v", tt.a.staged)
	}
}

// The tests read the window in Chinese, as it was written.
func TestMain(m *testing.M) {
	i18n.SetLocaleFunc(func() string { return "zh-Hans" })
	os.Exit(m.Run())
}

// In English the window says everything in English.
func TestEnglish(t *testing.T) {
	i18n.SetLocaleFunc(func() string { return "en" })
	defer i18n.SetLocaleFunc(func() string { return "zh-Hans" })
	tt := newTester(t)
	for _, s := range []string{"Staged Changes", "Changes", "Commit message", "Amend last commit", "History", "4 commits"} {
		if !tt.HasText(s) {
			t.Fatalf("no %q in %q", s, tt.Texts())
		}
	}
	for _, s := range tt.Texts() {
		for _, r := range s {
			if r >= 0x4e00 && r <= 0x9fff {
				t.Fatalf("Chinese in the English window: %q", s)
			}
		}
	}
	if loc.In("en").CommitCount(1) != "1 commit" || loc.In("zh-Hans").CommitCount(3) != "3 个提交" {
		t.Fatal("plurals", loc.In("en").CommitCount(1), loc.In("zh-Hans").CommitCount(3))
	}
}

// TestSaveWithKeys saves the file typed in, in the diff, with ⌘S (Ctrl+S
// elsewhere), the editor holding the keys' focus.
func TestSaveWithKeys(t *testing.T) {
	tt := newTester(t)
	dir := tt.a.repo.Dir
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() { println(1) }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tt.a.refresh()
	tt.settle()
	tt.click(t, "main.go")
	tt.ClickAt(1100, 520) // in the file, on the diff's right
	tt.Frame()
	tt.Type("X")
	tt.Frame()
	if !tt.a.unsaved() {
		t.Fatal("typing in the diff didn't edit the file")
	}
	tt.Key(ui.Cmd, ui.KeyS)
	tt.settle()
	b, _ := os.ReadFile(filepath.Join(dir, "main.go"))
	if tt.a.unsaved() || string(b) != tt.a.detail.diff.New.String() {
		t.Fatalf("⌘S didn't save: unsaved %v, file %q", tt.a.unsaved(), b)
	}
}

// TestEditKeepsItsPlace edits a file in the diff back to how it was: the
// diff shows the whole file as it's edited, opened at the first change,
// so that a change typed away moves nothing.
func TestEditKeepsItsPlace(t *testing.T) {
	tt := newTester(t)
	dir := tt.a.repo.Dir
	var lines []string
	for i := 1; i <= 80; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	os.WriteFile(filepath.Join(dir, "long.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	tt.commitFiles(t, "Long", nil)
	lines[39] = "line 40 changed"
	os.WriteFile(filepath.Join(dir, "long.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	tt.a.refresh()
	tt.settle()
	ch, _ := find(tt.a.unstaged, "long.txt")
	tt.a.showChange(ch, false)
	tt.settle()
	d := tt.a.detail.diff
	if !d.ExpandUnchanged || d.CursorPos().Line != 39 {
		t.Fatalf("expanded %v, caret on line %d", d.ExpandUnchanged, d.CursorPos().Line)
	}
	tt.Frame()
	// The lines above the change, as they show.
	above := image.Rect(760, 450, 1180, 610)
	before := image.NewRGBA(above)
	draw.Draw(before, above, tt.Image(), above.Min, draw.Src)
	// Type the change away, from the end of its line, the caret's.
	tt.ClickAt(1000, 624)
	tt.Frame()
	for range " changed" {
		tt.Key(0, ui.KeyBackspace)
		tt.Frame()
	}
	if got := strings.Split(d.New.String(), "\n")[39]; got != "line 40" || len(d.Hunks()) != 0 {
		t.Fatalf("line 40 %q, %d changes", got, len(d.Hunks()))
	}
	after := tt.Image()
	for y := above.Min.Y; y < above.Max.Y; y++ {
		for x := above.Min.X; x < above.Max.X; x++ {
			if before.RGBAAt(x, y) != after.RGBAAt(x, y) {
				t.Fatalf("the lines above moved: (%d, %d) differs", x, y)
			}
		}
	}
}

// TestChooseAfterAChangeIsGone saves the second of two changes back to how
// it was: it leaves the list, and the first, chosen then, shows its diff.
func TestChooseAfterAChangeIsGone(t *testing.T) {
	tt := newTester(t)
	dir := tt.a.repo.Dir
	tt.commitFiles(t, "Two files", map[string]string{"a.txt": "a\n", "b.txt": "b\n"})
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a changed\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b changed\n"), 0o644)
	tt.a.refresh()
	tt.settle()
	tt.click(t, "b.txt")
	d := tt.a.detail.diff
	if !tt.a.detail.whole || d == nil {
		t.Fatalf("b.txt not shown to edit: %+v", tt.a.detail)
	}
	d.New.Set("b\n")
	tt.a.saveFile()
	tt.settle()
	if _, ok := find(tt.a.unstaged, "b.txt"); ok || tt.a.detail.change != nil {
		t.Fatalf("b.txt still a change: %v, shown %v", tt.a.unstaged, tt.a.detail.change)
	}
	tt.click(t, "a.txt")
	if ch := tt.a.detail.change; ch == nil || ch.Path != "a.txt" || tt.a.detail.diff == nil {
		t.Fatalf("a.txt not shown: %+v", tt.a.detail)
	}
}

// What a branch's menu in the branches' popover does closes the popover:
// its history, which a banner tells, and switching to it, which shows the
// current branch's again.
func TestBranchMenuClosesPopover(t *testing.T) {
	tt := newTester(t)
	// The popover's first local branch, under its heading: "feature" is
	// in the history too.
	firstBranch := func() {
		t.Helper()
		r, ok := tt.Find(loc.BranchesLocal())
		if !ok {
			t.Fatalf("texts %q", tt.Texts())
		}
		tt.ClickAt(r.X+30, r.Y+r.H+16)
		tt.Frame()
	}
	tt.click(t, "分支")
	firstBranch()
	if err := tt.ChooseMenuItem(loc.BranchHistory()); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	tt.settle()
	if tt.a.branchesOpen || tt.a.scope.Ref != "feature" || !tt.HasText(loc.RefOnly("feature")) {
		t.Fatalf("open %v, scope %+v, texts %q", tt.a.branchesOpen, tt.a.scope, tt.Texts())
	}
	tt.click(t, "分支")
	firstBranch()
	if err := tt.ChooseMenuItem(loc.BranchSwitch()); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	tt.settle()
	if tt.a.branchesOpen || tt.a.status.Branch != "feature" {
		t.Fatalf("open %v, branch %q", tt.a.branchesOpen, tt.a.status.Branch)
	}
	if tt.a.scope.Ref != "HEAD" || tt.HasText(loc.RefOnly("feature")) {
		t.Errorf("after switching, scope %+v", tt.a.scope)
	}
}

// A section of the branches' popover folds, as kept, and shows again
// while searching.
func TestFoldBranchSections(t *testing.T) {
	defer func(f map[string]bool) { prefs.FoldedBranches = f }(prefs.FoldedBranches)
	prefs.FoldedBranches = nil
	tt := newTester(t)
	if err := tt.a.repo.Tag(context.Background(), "v1.0", "HEAD", ""); err != nil {
		t.Fatal(err)
	}
	tt.a.refresh()
	tt.settle()
	tt.click(t, "分支")
	tags := func() int { // the history's badge, and the popover's rows
		return strings.Count(strings.Join(tt.Texts(), "\x00")+"\x00", "v1.0\x00")
	}
	shown := tags()
	tt.click(t, loc.BranchesTags())
	tt.Frame()
	folded := tags()
	if !prefs.FoldedBranches["tags"] || folded >= shown || !tt.HasText("feature") {
		t.Fatalf("folded %v, %d of %d tags, texts %q", prefs.FoldedBranches, folded, shown, tt.Texts())
	}
	tt.a.branchFilter = "v1"
	tt.Frame()
	if tags() != shown {
		t.Fatalf("searching: texts %q", tt.Texts())
	}
	tt.a.branchFilter = ""
	tt.Frame()
	tt.click(t, loc.BranchesTags())
	tt.Frame()
	if prefs.FoldedBranches["tags"] || tags() != shown {
		t.Fatalf("unfolded %v, texts %q", prefs.FoldedBranches, tt.Texts())
	}
}

// The branches' popover lists the tags too, each with its menu: its
// history shows the tag's.
func TestTagsInBranchesPopover(t *testing.T) {
	tt := newTester(t)
	if err := tt.a.repo.Tag(context.Background(), "v1.0", "HEAD", ""); err != nil {
		t.Fatal(err)
	}
	tt.a.refresh()
	tt.settle()
	tt.click(t, "分支")
	r, ok := tt.Find(loc.BranchesTags())
	if !ok || !tt.HasText("v1.0") {
		t.Fatalf("texts %q", tt.Texts())
	}
	tt.ClickAt(r.X+30, r.Y+r.H+16)
	tt.Frame()
	if err := tt.ChooseMenuItem(loc.TagHistory()); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	tt.settle()
	if tt.a.branchesOpen || tt.a.scope.Ref != "v1.0" {
		t.Errorf("open %v, scope %+v", tt.a.branchesOpen, tt.a.scope)
	}
	tt.click(t, loc.BackToCurrentBranch())
	if tt.a.scope.Ref != "HEAD" {
		t.Errorf("back: scope %+v", tt.a.scope)
	}
}

// A branch checked out in a new worktree from its menu opens in a window
// of its own, and the popover lists the worktrees, each with its menu:
// removing one with a file in it asks first.
func TestWorktreesInBranchesPopover(t *testing.T) {
	var opened []string
	openRepo = func(r *git.Repo) { opened = append(opened, r.Dir) }
	t.Cleanup(func() { openRepo = open })
	tt := newTester(t)
	branchRow := func(section string, n int) {
		t.Helper()
		r, ok := tt.Find(section)
		if !ok {
			t.Fatalf("texts %q", tt.Texts())
		}
		tt.ClickAt(r.X+30, r.Y+r.H+16+float32(n*27))
		tt.Frame()
	}
	tt.click(t, "分支")
	if tt.HasText(loc.BranchesWorktrees()) {
		t.Fatal("the repository's own worktree alone is listed")
	}
	branchRow(loc.BranchesLocal(), 0) // feature
	if err := tt.ChooseMenuItem(loc.WorktreeNew()); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	tt.settle()
	d := tt.a.worktree
	want := filepath.Join(filepath.Dir(tt.a.repo.Dir), filepath.Base(tt.a.repo.Dir)+"-feature")
	if !d.open || d.asksBranch || d.dir != want {
		t.Fatalf("dialog %+v, want %s", d, want)
	}
	tt.click(t, loc.Create())
	if len(opened) != 1 || opened[0] != want {
		t.Fatalf("opened %q", opened)
	}
	tt.click(t, "分支")
	tt.settle()
	if !tt.HasText(loc.BranchesWorktrees()) || !tt.HasText(filepath.Base(want)) {
		t.Fatalf("texts %q", tt.Texts())
	}
	if err := os.WriteFile(filepath.Join(want, "new.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	branchRow(loc.BranchesWorktrees(), 1)
	if err := tt.ChooseMenuItem(loc.WorktreeRemove()); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	tt.settle()
	if !tt.a.confirm.open {
		t.Fatalf("didn't ask; error %q", tt.a.errMsg)
	}
	tt.click(t, loc.DeleteAnyway())
	if _, err := os.Stat(want); !os.IsNotExist(err) {
		t.Fatalf("the worktree's folder after removing it: %v", err)
	}
}

// F7 and ⇧F7 step the caret from change to change, and stop at the ends.
func TestStepChange(t *testing.T) {
	var oldText, newText strings.Builder
	for i := range 40 {
		line := fmt.Sprintf("line %d\n", i)
		oldText.WriteString(line)
		if i == 5 || i == 20 || i == 30 {
			line = fmt.Sprintf("changed %d\n", i)
		}
		newText.WriteString(line)
	}
	a := &app{}
	a.detail.diff = &editor.DiffState{Old: editor.NewBuffer(oldText.String()), New: editor.NewBuffer(newText.String())}
	at := func() int {
		i, inside := a.detail.diff.CurrentHunk()
		if !inside {
			return -1
		}
		return i
	}
	for _, step := range []struct {
		back bool
		want int
	}{{false, 0}, {false, 1}, {false, 2}, {false, 2}, {true, 1}, {true, 0}, {true, 0}} {
		a.stepChange(step.back)
		if got := at(); got != step.want {
			t.Fatalf("back %v: at change %d, want %d", step.back, got, step.want)
		}
	}
}
