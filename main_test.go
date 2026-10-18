package main

import (
	"context"
	"errors"
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
	if m == nil || len(m.blocks) != 1 || !tt.HasText("冲突 1 / 1") {
		t.Fatalf("merge %+v, texts %q", m, tt.Texts())
	}
	if m.ours.Buffer.String() != "package mine\n" || m.theirs.Buffer.String() != "package other\n" {
		t.Fatalf("sides %q, %q", m.ours.Buffer.String(), m.theirs.Buffer.String())
	}
	tt.click(t, "采用右侧")
	if m.result.Buffer.String() != "package other\n" || !tt.HasText("没有剩余冲突") {
		t.Fatalf("result %q", m.result.Buffer.String())
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
	if err := appendIgnore(tt.a.repo.Dir, "*.txt"); err != nil {
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
