package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"mygit/internal/llm"
	loc "mygit/locales"
)

// blogWithTheme makes a repository with another as a submodule in
// themes/nivis, the theme's commit "Style the theme", as #54's; git
// clones submodules from folders only when allowed.
func blogWithTheme(t *testing.T) string {
	t.Helper()
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "protocol.file.allow")
	t.Setenv("GIT_CONFIG_VALUE_0", "always")
	root := t.TempDir()
	theme, blog := filepath.Join(root, "nivis"), filepath.Join(root, "zola_blog")
	gitInit(t, theme, "Style the theme", "")
	gitInit(t, blog, "Start the blog", "")
	run(t, blog, "submodule", "add", "-q", theme, "themes/nivis")
	run(t, blog, "commit", "-q", "-m", "Add the theme")
	return blog
}

// A repository opens with its submodules checked out, each a section of
// its own; choosing a file in one shows that one's history, and the
// superproject's row of the submodule shows its commits, not a failure
// to read a folder (#54).
func TestWorkspaceSubmodules(t *testing.T) {
	blog := blogWithTheme(t)
	os.WriteFile(filepath.Join(blog, "themes", "nivis", "README.md"), []byte("# nivis, changed\n"), 0o644)
	ws, tt, settle := newWorkspace(t, blog)
	settle()
	if len(ws.apps) != 2 || ws.apps[0].repo.Name() != "zola_blog" || ws.apps[1].repo.Name() != "nivis" {
		t.Fatalf("repositories %v", ws.apps)
	}
	if err := tt.Click("README.md"); err != nil { // the theme's file, in its section
		t.Fatal(err)
	}
	settle()
	if ws.active != 1 || !tt.HasText("Style the theme") || tt.HasText("Add the theme") {
		t.Fatalf("active %d, texts %q", ws.active, tt.Texts())
	}
	// The superproject's row of it: a change, its diff shown.
	if err := tt.Click("themes"); err != nil { // its folder, beside its name: the toolbar's nivis is the project's
		t.Fatal(err)
	}
	settle()
	blogApp := ws.apps[0]
	if ws.active != 0 {
		t.Fatalf("active %d", ws.active)
	}
	if blogApp.errMsg != "" || blogApp.detail.change == nil || !blogApp.detail.change.Submodule ||
		!strings.Contains(blogApp.detail.raw, "Subproject commit") || tt.HasText("暂存选中行") {
		t.Fatalf("error %q, detail %+v, texts %q", blogApp.errMsg, blogApp.detail.change, tt.Texts())
	}
	// Its menu doesn't move the whole submodule to the trash.
	if onDisk(*blogApp.detail.change) {
		t.Error("a submodule offered to the trash")
	}
}

// A clone without its submodules says they are empty, and initializing
// them adds them to the window.
func TestInitializeSubmodules(t *testing.T) {
	blog := blogWithTheme(t)
	clone := filepath.Join(t.TempDir(), "zola_blog")
	if out, err := exec.Command("git", "clone", "-q", blog, clone).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	ws, tt, settle := newWorkspace(t, clone)
	settle()
	if len(ws.apps) != 1 || !tt.HasText("1 个子模块还没初始化，目录是空的") {
		t.Fatalf("repositories %d, texts %q", len(ws.apps), tt.Texts())
	}
	if err := tt.Click("初始化"); err != nil {
		t.Fatal(err)
	}
	settle()
	settle()
	if len(ws.apps) != 2 || ws.apps[1].repo.Name() != "nivis" || tt.HasText("还没初始化") {
		t.Fatalf("repositories %d, texts %q", len(ws.apps), tt.Texts())
	}
	if _, err := os.Stat(filepath.Join(clone, "themes", "nivis", "README.md")); err != nil {
		t.Fatal(err)
	}
}

// A submodule added from the repository's menu joins the window.
func TestAddSubmodule(t *testing.T) {
	blog := blogWithTheme(t) // allows cloning from folders, and gives a theme to add
	theme := filepath.Join(filepath.Dir(blog), "nivis")
	site := filepath.Join(t.TempDir(), "site")
	gitInit(t, site, "Start the site", "")
	ws, tt, settle := newWorkspace(t, site)
	settle()
	a := ws.apps[0]
	a.addSubmodule()
	tt.Frame()
	if !a.prompt.open || !tt.HasText("输入要克隆的仓库 URL") {
		t.Fatalf("prompt %+v", a.prompt)
	}
	a.prompt.open = false
	a.prompt.ok(theme, "libs/theme")
	settle()
	settle()
	if len(ws.apps) != 2 || ws.apps[1].repo.Name() != "theme" || a.errMsg != "" {
		t.Fatalf("repositories %d, error %q", len(ws.apps), a.errMsg)
	}
	if _, ok := find(a.staged, ".gitmodules"); !ok {
		t.Errorf(".gitmodules not staged: %+v", a.staged)
	}
}

// countText is how many of texts are s.
func countText(texts []string, s string) int {
	n := 0
	for _, text := range texts {
		if text == s {
			n++
		}
	}
	return n
}

// A repository with nothing to commit says so where the message's box
// would be, rather than offering a box no commit would take; one with a
// staged change has the box, and Generate beside it writes its own message
// (#54).
func TestWorkspaceCommitArea(t *testing.T) {
	defer func(s aiSettings) { prefs.AI = s }(prefs.AI)
	*fakeKey(t) = "sk-test"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		if !strings.Contains(body.Messages[len(body.Messages)-1].Content, "deploy.sh") {
			t.Errorf("asked %q", body.Messages[len(body.Messages)-1].Content)
		}
		io.WriteString(w, `{"choices":[{"message":{"content":"Add the deploy script"}}]}`)
	}))
	defer srv.Close()
	prefs.AI = aiSettings{API: llm.OpenAI, BaseURL: srv.URL, Model: "m", KeySaved: true}

	blog := blogWithTheme(t)
	os.WriteFile(filepath.Join(blog, "deploy.sh"), []byte("#!/bin/sh\n"), 0o644)
	run(t, blog, "add", "deploy.sh")
	ws, tt, settle := newWorkspace(t, blog)
	settle()
	if ws.apps[0].repo.Name() != "zola_blog" || ws.apps[1].repo.Name() != "nivis" {
		t.Fatalf("repositories %v", ws.apps)
	}
	if ws.apps[0].nothingToCommit() || !ws.apps[1].nothingToCommit() {
		t.Fatalf("blog %v, theme %v", ws.apps[0].nothingToCommit(), ws.apps[1].nothingToCommit())
	}
	texts := tt.Texts()
	if n := countText(texts, loc.NoChanges()); n != 1 {
		t.Fatalf("%d lines of no changes in %q", n, texts)
	}
	// One box, the blog's, with Generate beside it; the clean theme has
	// neither.
	if n := countText(texts, loc.Message()); n != 1 {
		t.Fatalf("%d message boxes in %q", n, texts)
	}
	// Generate's label and its text, one button.
	if n := countText(texts, loc.GenerateMessage()); n != 2 {
		t.Fatalf("%d Generate texts in %q", n, texts)
	}
	if err := tt.Click(loc.GenerateMessage()); err != nil {
		t.Fatal(err)
	}
	settle()
	if ws.apps[0].message != "Add the deploy script" || ws.apps[1].message != "" || ws.apps[0].errMsg != "" {
		t.Fatalf("messages %q %q, error %q", ws.apps[0].message, ws.apps[1].message, ws.apps[0].errMsg)
	}
}

// The toolbar's menu in a workspace is the window's; each repository's own
// things sit in the menu beside its name, so that the two aren't the same
// menu twice (#54).
func TestWorkspaceMenus(t *testing.T) {
	ws, tt, settle := newWorkspace(t, workspaceDir(t))
	settle()
	// The toolbar's "..." comes first in the frame.
	if err := tt.Click(loc.Repository()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	menu := tt.Menu()
	if !slices.Contains(menu, loc.MenuOpenRepo()) || !slices.Contains(menu, loc.SettingsMenu()) {
		t.Fatalf("toolbar menu %q", menu)
	}
	if slices.Contains(menu, loc.RemotesMenu()) || slices.Contains(menu, loc.HideRepoMenu()) {
		t.Fatalf("toolbar menu has a repository's items: %q", menu)
	}
	tt.CloseMenu()
	tt.Frame()
	// The section's, at the right of filmcraft's row (a repository not in
	// the toolbar, whose name would be found there first).
	r, ok := tt.Find("filmcraft")
	if !ok {
		t.Fatalf("texts %q", tt.Texts())
	}
	tt.ClickAt(ws.apps[0].sidebarW-17, r.Y+r.H/2)
	tt.Frame()
	menu = tt.Menu()
	if !slices.Contains(menu, loc.RemotesMenu()) || !slices.Contains(menu, loc.HideRepoMenu()) {
		t.Fatalf("section menu %q", menu)
	}
	if slices.Contains(menu, loc.MenuOpenRepo()) || slices.Contains(menu, loc.SettingsMenu()) {
		t.Fatalf("section menu has the window's items: %q", menu)
	}
}
