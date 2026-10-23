package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"mygit/internal/git"
	"mygit/internal/helper"
)

func TestParseLocation(t *testing.T) {
	for in, want := range map[string][2]string{
		"box:/srv/repo":         {"box", "/srv/repo"},
		"ada@10.0.0.2:~/code/x": {"ada@10.0.0.2", "code/x"},
		"win:C:/Users/ada/repo": {"win", "C:/Users/ada/repo"},
		"box:~":                 {"box", "."},
		" box:repo ":            {"box", "repo"},
		"wsl+Ubuntu:~/code/x":   {"wsl+Ubuntu", "code/x"},
	} {
		host, dir, ok := parseLocation(in)
		if !ok || host != want[0] || dir != want[1] {
			t.Errorf("%q: %q %q %v", in, host, dir, ok)
		}
	}
	for _, in := range []string{`C:\Users\ada\repo`, "C:/repo", "/srv/repo", "./a:b", "repo", "a b:/x"} {
		if _, _, ok := parseLocation(in); ok {
			t.Errorf("%q read as another computer's", in)
		}
	}
}

func TestSplitHost(t *testing.T) {
	for in, want := range map[string][2]string{
		"box":            {"box", ""},
		" ada@10.0.0.2 ": {"ada@10.0.0.2", ""},
		"box:/srv/repo":  {"box", "/srv/repo"},
		"box:~/code":     {"box", "code"},
		"wsl+Ubuntu":     {"wsl+Ubuntu", ""},
	} {
		host, dir, ok := splitHost(in)
		if !ok || host != want[0] || dir != want[1] {
			t.Errorf("%q: %q %q %v", in, host, dir, ok)
		}
	}
	for _, in := range []string{"", "a b", "/srv/repo", `C:\repo`} {
		if _, _, ok := splitHost(in); ok {
			t.Errorf("%q read as a host", in)
		}
	}
}

// The dialog connects to a host of ~/.ssh/config, then goes through its
// folders to the repository.
func TestConnectDialog(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".ssh"), 0o755)
	os.WriteFile(filepath.Join(home, ".ssh", "config"), []byte("Host box\n  HostName 10.0.0.2\nHost *\n  ServerAliveInterval 30\n"), 0o644)
	folders := map[string]helper.Listing{
		"":                {Dir: "/home/ada", Parent: "/home", Entries: []helper.Entry{{Name: "code"}, {Name: "notes"}}},
		"/home/ada/code":  {Dir: "/home/ada/code", Parent: "/home/ada", Entries: []helper.Entry{{Name: "app", Repo: true}}},
		"/home/ada/notes": {Dir: "/home/ada/notes", Parent: "/home/ada"},
	}
	folders["/home/ada"] = folders[""]
	var listed []string
	old := listFolder
	listFolder = func(_ context.Context, host, dir string) (helper.Listing, error) {
		listed = append(listed, host+":"+dir)
		if l, ok := folders[dir]; ok {
			return l, nil
		}
		return helper.Listing{}, errors.New(dir + ": no such folder")
	}
	defer func() { listFolder = old }()

	queue := make(chan func(), 8)
	w := &welcome{updFn: func(fn func()) { queue <- fn }}
	tt := ui.NewTester(w.view, 520, 640)
	settle := func() {
		tt.Frame()
		for w.ssh.running {
			(<-queue)()
			tt.Frame()
		}
	}
	click := func(s string) {
		t.Helper()
		if err := tt.Click(s); err != nil {
			t.Fatalf("%v; texts %q", err, tt.Texts())
		}
		settle()
	}
	click("通过 SSH 打开…")
	click("SSH 主机")
	if err := tt.ChooseMenuItem("box"); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	settle()
	if !tt.HasText("选择 box 上的仓库") || !tt.HasText("code") || !tt.HasText("notes") || w.ssh.path != "/home/ada" {
		t.Fatalf("texts %q", tt.Texts())
	}
	click("code")
	if !tt.HasText("app") || !tt.HasText("打开") {
		t.Fatalf("texts %q", tt.Texts())
	}
	click("..")
	click("notes")
	if !tt.HasText("这里没有文件夹") {
		t.Fatalf("texts %q", tt.Texts())
	}
	// A folder written that isn't there says so, and stays.
	w.ssh.path = "/nope"
	w.browse("/nope")
	settle()
	if !tt.HasText("/nope: no such folder") || w.ssh.listing.Dir != "/home/ada/notes" {
		t.Errorf("texts %q", tt.Texts())
	}
	// The folder written opens, though not gone into: a hidden one isn't
	// listed to go into.
	oldOpen := openRemote
	defer func() { openRemote = oldOpen }()
	var opened string
	openRemote = func(_ context.Context, host, dir string) (string, []*git.Repo, error) {
		opened = host + ":" + dir
		return "", nil, errors.New("stop here")
	}
	w.ssh.path = " /home/ada/.dotfiles "
	click("打开这个文件夹")
	if opened != "box:/home/ada/.dotfiles" || !tt.HasText("stop here") {
		t.Errorf("opened %q; texts %q", opened, tt.Texts())
	}
	click("换主机")
	if !tt.HasText("通过 SSH 打开仓库") || w.ssh.host != "box" {
		t.Errorf("texts %q", tt.Texts())
	}
	want := []string{"box:", "box:/home/ada/code", "box:/home/ada", "box:/home/ada/notes", "box:/nope"}
	if !slices.Equal(listed, want) {
		t.Errorf("listed %q", listed)
	}
}

// helperRepo opens the repository in dir as on host, through a helper
// over pipes.
func helperRepo(t *testing.T, host, dir string) *git.Repo {
	t.Helper()
	toServer, fromClient := io.Pipe()
	toClient, fromServer := io.Pipe()
	go func() { _ = helper.Serve(toServer, fromServer) }()
	c, err := helper.NewClient(host, toClient, fromClient, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fromClient.Close() })
	repo, err := git.OpenOn(c, dir)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

// A repository on another computer says so in the toolbar, with its host,
// and its history and changes show as a local one's.
func TestRemoteRepository(t *testing.T) {
	a := newApp(helperRepo(t, "box", testRepo(t).Dir))
	tt := &tester{a: a, queue: make(chan func(), 64)}
	a.update = func(fn func()) { tt.queue <- fn }
	tt.Tester = ui.NewTester(a.view, 1200, 800)
	a.start()
	tt.settle()
	for _, s := range []string{"SSH · box", "Merge the feature", "notes.txt"} {
		if !tt.HasText(s) {
			t.Fatalf("no %q in %q", s, tt.Texts())
		}
	}
	if tt := newTester(t); tt.HasText("SSH · ") {
		t.Error("a local repository shows a host")
	}
}

// The welcome window's dialogs fit it, as narrow as Windows makes it.
func TestWelcomeDialogsFit(t *testing.T) {
	for _, width := range []float32{520, 470, 420} {
		for name, plan := range map[string]func(w *welcome){
			"连接": func(w *welcome) { w.ssh = connectPlan{open: true} },
			"打开这个文件夹": func(w *welcome) {
				w.ssh = connectPlan{open: true, connected: "box", listing: helper.Listing{Dir: "/home/ada", Entries: []helper.Entry{{Name: "app", Repo: true}}}}
			},
			"克隆": func(w *welcome) { w.clone = clonePlan{open: true, parent: "/home/ada"} },
		} {
			w := &welcome{}
			plan(w)
			tt := ui.NewTester(w.view, int(width), 640)
			r, ok := tt.Find(name)
			if !ok {
				t.Fatalf("no %q in %q", name, tt.Texts())
			}
			if right := r.X + r.W; right > width-16 {
				t.Errorf("width %v: %q ends at %v, past the dialog", width, name, right)
			}
		}
	}
}

// With room, the folders show 600 wide, and the host's dialog 460.
func TestWelcomeDialogsWidth(t *testing.T) {
	for _, tc := range []struct {
		plan        connectPlan
		left, right string
		want        float32
	}{
		{connectPlan{open: true, hosts: []string{"box"}}, "SSH 主机", "连接", 460},
		{connectPlan{open: true, connected: "box", listing: helper.Listing{Dir: "/home/ada"}}, "换主机", "打开这个文件夹", 600},
	} {
		w := &welcome{ssh: tc.plan}
		tt := ui.NewTester(w.view, 680, 720)
		l, ok1 := tt.Find(tc.left)
		r, ok2 := tt.Find(tc.right)
		// From the first label to the last: the buttons' padding is outside.
		if got := r.X + r.W - l.X; !ok1 || !ok2 || got < tc.want-40 || got > tc.want {
			t.Errorf("%s…%s is %v wide, want %v", tc.left, tc.right, got, tc.want)
		}
	}
}

// A repository's window opens another over SSH from its menu and the
// commands.
func TestOpenOverSSHFromAWindow(t *testing.T) {
	tt := newTester(t)
	if err := tt.Click("仓库"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	if !slices.Contains(tt.Menu(), "通过 SSH 打开…") {
		t.Errorf("menu %q", tt.Menu())
	}
	tt.CloseMenu()
	tt.a.openCommands()
	tt.Frame()
	if !tt.HasText("通过 SSH 打开…") || !tt.HasText(shiftShortcut("O")) {
		t.Errorf("commands %q", tt.Texts())
	}
}

// A repository owned by another user over there is offered to trust, and
// trusted by its whole path there.
func TestConnectTrust(t *testing.T) {
	oldList, oldTrust := listFolder, trustRemote
	defer func() { listFolder, trustRemote = oldList, oldTrust }()
	listFolder = func(_ context.Context, host, dir string) (helper.Listing, error) {
		return helper.Listing{Dir: "/home/ada/" + dir}, nil
	}
	var trusted string
	trustRemote = func(_ context.Context, host, dir string) error {
		trusted = host + ":" + dir
		return errors.New("stop here")
	}
	queue := make(chan func(), 8)
	w := &welcome{updFn: func(fn func()) { queue <- fn }}
	w.ssh = connectPlan{open: true, connected: "box", listing: helper.Listing{Dir: "/home/ada"},
		fail: "app is owned by another user", unsafe: &git.UnsafeError{Dir: "app", Host: "box"}}
	tt := ui.NewTester(w.view, 680, 720)
	if err := tt.Click("信任并打开"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	for w.ssh.running {
		(<-queue)()
		tt.Frame()
	}
	if trusted != "box:/home/ada/app" || w.ssh.fail != "stop here" || w.ssh.unsafe != nil {
		t.Errorf("trusted %q, fail %q, unsafe %v", trusted, w.ssh.fail, w.ssh.unsafe)
	}
}

func TestHostName(t *testing.T) {
	for host, want := range map[string]string{"devbox": "devbox", "ada@box": "ada@box", "wsl+Ubuntu-24.04": "WSL · Ubuntu-24.04"} {
		if got := hostName(host); got != want {
			t.Errorf("hostName(%q) = %q, want %q", host, got, want)
		}
	}
}

// A folder opened goes first among those opened lately, a remote one or
// not; those opened before the order was kept come after, as they were.
func TestRecentLocations(t *testing.T) {
	oldRemotes, oldRecent := prefs.Remotes, prefs.Recent
	t.Cleanup(func() { prefs.Remotes, prefs.Recent = oldRemotes, oldRecent })
	prefs.Remotes, prefs.Recent = []string{"devbox:/a", "devbox:/b"}, nil
	remember("devbox", "/b")
	remember("pi", "/c")
	got := slices.DeleteFunc(recentLocations(), func(l string) bool { _, _, ok := parseLocation(l); return !ok })
	if !slices.Equal(got, []string{"pi:/c", "devbox:/b", "devbox:/a"}) {
		t.Fatalf("recent %q", got)
	}
	remember("devbox", "/a")
	if got := recentLocations(); got[0] != "devbox:/a" {
		t.Fatalf("recent %q", got)
	}
}
