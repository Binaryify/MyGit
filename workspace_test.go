package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"mygit/internal/git"
	"mygit/internal/helper"
)

// gitInit makes a repository in dir with a commit, and leaves a change.
func gitInit(t *testing.T, dir, commit, change string) {
	t.Helper()
	os.MkdirAll(dir, 0o755)
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"config", "commit.gpgsign", "false"}} {
		run(t, dir, args...)
	}
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# "+filepath.Base(dir)+"\n"), 0o644)
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-q", "-m", commit)
	if change != "" {
		os.WriteFile(filepath.Join(dir, change), []byte("new\n"), 0o644)
	}
}

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Ada", "GIT_AUTHOR_EMAIL=a@example.com", "GIT_COMMITTER_NAME=Ada", "GIT_COMMITTER_EMAIL=a@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// A folder of repositories: two side by side, one a folder down, and
// those it doesn't look in.
func workspaceDir(t *testing.T) string {
	root := filepath.Join(t.TempDir(), "crafts")
	gitInit(t, filepath.Join(root, "artcraft"), "Draw the first stroke", "brush.ts")
	gitInit(t, filepath.Join(root, "filmcraft"), "Cut the first frame", "")
	gitInit(t, filepath.Join(root, "apps", "printcraft"), "Print the first page", "page.ts")
	gitInit(t, filepath.Join(root, "artcraft", "vendor-repo"), "Inside a repository", "")
	gitInit(t, filepath.Join(root, "node_modules", "dep"), "A dependency", "")
	gitInit(t, filepath.Join(root, ".cache", "hidden"), "Hidden", "")
	os.MkdirAll(filepath.Join(root, "notes"), 0o755)
	return root
}

func TestFindRepos(t *testing.T) {
	root := workspaceDir(t)
	whole, repos, err := findRepos(context.Background(), listLocal, root, 3, 64)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, r := range repos {
		rel, _ := filepath.Rel(whole, r)
		names = append(names, filepath.ToSlash(rel))
	}
	slices.Sort(names)
	if want := []string{"apps/printcraft", "artcraft", "filmcraft"}; !slices.Equal(names, want) {
		t.Errorf("repositories %q, want %q", names, want)
	}
	// In a repository, that one.
	_, opened, err := openFolders(context.Background(), git.Local, listLocal, filepath.Join(root, "artcraft"))
	if err != nil || len(opened) != 1 || opened[0].Name() != "artcraft" {
		t.Errorf("opened %v, %v", opened, err)
	}
	// Nothing under a folder of none.
	if _, _, err := openFolders(context.Background(), git.Local, listLocal, filepath.Join(root, "notes")); err == nil {
		t.Error("a folder of no repositories opened")
	}
}

// newWorkspace shows the workspace of dir without a window.
func newWorkspace(t *testing.T, dir string) (*workspace, *ui.Tester, func()) {
	t.Helper()
	whole, repos, err := openFolders(context.Background(), git.Local, listLocal, dir)
	if err != nil {
		t.Fatal(err)
	}
	ws := &workspace{dir: whole, closed: map[string]bool{}}
	return startWorkspace(ws, repos, func(view func(*ui.Context)) *ui.Tester { return ui.NewTester(view, 1200, 900) })
}

// startWorkspace opens the repositories in ws, rendered by the tester
// newTester makes, and returns it with what waits for them to settle.
func startWorkspace(ws *workspace, repos []*git.Repo, newTester func(view func(*ui.Context)) *ui.Tester) (*workspace, *ui.Tester, func()) {
	queue := make(chan func(), 256)
	for _, r := range repos {
		a := newApp(r)
		a.ws = ws
		a.update = func(fn func()) { queue <- fn }
		ws.apps = append(ws.apps, a)
	}
	tt := newTester(ws.view)
	settle := func() {
		for {
			tt.Frame()
			busy := false
			for _, a := range ws.apps {
				busy = busy || a.refreshing || a.loading || a.busy != "" || a.detail.loading
			}
			select {
			case fn := <-queue:
				fn()
				continue
			default:
			}
			if !busy {
				return
			}
			(<-queue)()
		}
	}
	for _, a := range ws.apps {
		a.start()
	}
	settle()
	return ws, tt, settle
}

func TestWorkspace(t *testing.T) {
	ws, tt, settle := newWorkspace(t, workspaceDir(t))
	if len(ws.apps) != 3 {
		t.Fatalf("%d repositories", len(ws.apps))
	}
	for _, s := range []string{"crafts /", "artcraft", "filmcraft", "printcraft", "brush.ts", "page.ts"} {
		if !tt.HasText(s) {
			t.Fatalf("no %q in %q", s, tt.Texts())
		}
	}
	// Working in a repository shows its history.
	if err := tt.Click("page.ts"); err != nil {
		t.Fatal(err)
	}
	settle()
	active := ws.apps[ws.active]
	if active.repo.Name() != "printcraft" || !tt.HasText("Print the first page") || tt.HasText("Draw the first stroke") {
		t.Fatalf("active %s, texts %q", active.repo.Name(), tt.Texts())
	}
	if active.detail.change == nil || active.detail.change.Path != "page.ts" {
		t.Errorf("detail %+v", active.detail.change)
	}
	// Staged and committed from its section.
	active.toggleStage(active.unstaged[0], false)
	settle()
	active.message = "Add the page"
	active.commit()
	settle()
	if !tt.HasText("Add the page") || len(active.staged) != 0 {
		t.Errorf("not committed: %q", tt.Texts())
	}
	// Folded away, its changes don't show; the count does.
	art := ws.apps[slices.IndexFunc(ws.apps, func(a *app) bool { return a.repo.Name() == "artcraft" })]
	ws.closed[art.repo.Dir] = true
	tt.Frame()
	if tt.HasText("brush.ts") {
		t.Error("a folded repository's changes show")
	}
	if out, _ := exec.Command("git", "-C", active.repo.Dir, "log", "-1", "--format=%s").Output(); strings.TrimSpace(string(out)) != "Add the page" {
		t.Errorf("last commit %q", out)
	}
}

// Over SSH, the folders are the helper's listings.
func TestFindReposOnHost(t *testing.T) {
	old := listFolder
	defer func() { listFolder = old }()
	tree := map[string][]helper.Entry{
		"/srv":                  {{Name: "api", Repo: true}, {Name: "web"}, {Name: "tmp"}},
		"/srv/web":              {{Name: "app", Repo: true}, {Name: "node_modules"}},
		"/srv/tmp":              {},
		"/srv/web/node_modules": {{Name: "dep", Repo: true}},
	}
	var listed []string
	listFolder = func(_ context.Context, host, dir string) (helper.Listing, error) {
		listed = append(listed, dir)
		return helper.Listing{Dir: dir, Entries: tree[dir]}, nil
	}
	whole, repos, err := findRepos(context.Background(), listOn("box"), "/srv", 3, 64)
	if err != nil || whole != "/srv" || !slices.Equal(repos, []string{"/srv/api", "/srv/web/app"}) {
		t.Errorf("%q %q %v", whole, repos, err)
	}
	if slices.Contains(listed, "/srv/web/node_modules") {
		t.Error("looked in node_modules")
	}
}

// A workspace folder that is a repository too: the repositories cloned in
// folders it ignores join it, its submodules and those it doesn't ignore
// don't, and only the folders git ignores are looked in.
func TestIgnoredRepos(t *testing.T) {
	root := filepath.Join(t.TempDir(), "meta")
	gitInit(t, root, "Start the meta repository", "")
	os.WriteFile(filepath.Join(root, ".gitignore"), []byte("repos/\ntools/cli/\nnode_modules/\ndist/\n"), 0o644)
	gitInit(t, filepath.Join(root, "repos", "api"), "Start the api", "")
	gitInit(t, filepath.Join(root, "repos", "web"), "Start the web", "")
	gitInit(t, filepath.Join(root, "tools", "cli"), "Start the cli", "")
	gitInit(t, filepath.Join(root, "node_modules", "dep"), "A dependency", "")
	gitInit(t, filepath.Join(root, "scratch"), "Not ignored", "") // untracked, not ignored
	os.MkdirAll(filepath.Join(root, "dist", "assets"), 0o755)
	os.WriteFile(filepath.Join(root, "dist", "assets", "a.js"), nil, 0o644)
	for i := range 30 { // the repository's own folders, many
		os.MkdirAll(filepath.Join(root, "src", fmt.Sprint(i)), 0o755)
		os.WriteFile(filepath.Join(root, "src", fmt.Sprint(i), "f.go"), []byte("package f\n"), 0o644)
	}
	lib := filepath.Join(t.TempDir(), "lib")
	gitInit(t, lib, "Start the lib", "")
	run(t, root, "add", ".gitignore", "src")
	run(t, root, "-c", "protocol.file.allow=always", "submodule", "add", "-q", lib, "lib")
	run(t, root, "commit", "-q", "-m", "Add the sources and the lib")

	var listed []string
	counting := func(ctx context.Context, dir string) (string, []folder, error) {
		listed = append(listed, dir)
		return listLocal(ctx, dir)
	}
	_, repos, err := openFolders(context.Background(), git.Local, counting, root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, r := range repos {
		names = append(names, r.Name())
	}
	if want := []string{"meta", "api", "web", "cli"}; !slices.Equal(names, want) {
		t.Errorf("repositories %q, want %q", names, want)
	}
	for _, d := range listed {
		if strings.Contains(d, "src") || strings.Contains(d, "node_modules") {
			t.Errorf("looked in %s", d)
		}
	}
	if len(listed) > 5 { // meta, repos, tools, dist and dist/assets
		t.Errorf("%d folders listed: %q", len(listed), listed)
	}

	// A repository with nothing ignored lists no folder at all.
	listed = nil
	plain := filepath.Join(t.TempDir(), "plain")
	gitInit(t, plain, "Start", "")
	_, repos, err = openFolders(context.Background(), git.Local, counting, plain)
	if err != nil || len(repos) != 1 || len(listed) != 0 {
		t.Errorf("%d repositories, %v, listed %q", len(repos), err, listed)
	}
}
