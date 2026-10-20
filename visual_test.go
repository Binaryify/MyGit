package main

import (
	"image/color"
	"image/png"
	"mygit/internal/agent"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
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

// TestDumpSSH renders, for the demo film, a repository on another computer
// into MYGIT_DUMP: the folders of devbox to choose it from (ssh-folders),
// and its window (ssh-remote), 1120×733 at 1.4×, as the film's shots are.
// MYGIT_LOCALE renders them in another language, as en.
func TestDumpSSH(t *testing.T) {
	out := os.Getenv("MYGIT_DUMP")
	if out == "" {
		t.Skip()
	}
	if l := os.Getenv("MYGIT_LOCALE"); l != "" {
		i18n.SetLocaleFunc(func() string { return l })
		defer i18n.SetLocaleFunc(func() string { return "zh-Hans" })
	}
	save := func(tt *ui.Tester, name string) {
		tt.Frame()
		f, _ := os.Create(out + "/" + name + ".png")
		png.Encode(f, tt.Image())
		f.Close()
	}
	shoot := func(tt *ui.Tester) {
		tt.SetDark(true)
		tt.SetScale(1.4)
		tt.SetTitleBar(ui.TitleBar{Left: 80, Height: 28})
	}

	w := &welcome{ssh: connectPlan{open: true, connected: "devbox", listing: agent.Listing{Dir: "/home/mia", Parent: "/home", Entries: []agent.Entry{
		{Name: "api-gateway", Repo: true}, {Name: "dotfiles"}, {Name: "infra", Repo: true},
		{Name: "notes"}, {Name: "orbit", Repo: true}, {Name: "scratch"}, {Name: "scripts"},
	}}}}
	w.ssh.path = w.ssh.listing.Dir
	tt := ui.NewTester(w.view, 1120, 733)
	shoot(tt)
	save(tt, "ssh-folders")
	if r, ok := tt.Find("orbit"); ok {
		t.Logf("orbit's row at %v (×1.4)", r)
	}

	a := newApp(agentRepo(t, "devbox", orbitRepo(t)))
	at := &tester{a: a, queue: make(chan func(), 64)}
	a.update = func(fn func()) { at.queue <- fn }
	at.Tester = ui.NewTester(a.view, 1120, 733)
	shoot(at.Tester)
	a.start()
	at.settle()
	save(at.Tester, "ssh-remote")
	if r, ok := at.Find("SSH · devbox"); ok {
		t.Logf("the badge at %v (×1.4)", r)
	}
}

// orbitRepo makes the film's demo repository: orbit, a satellite tracker,
// its history by four people and a change of the working tree.
func orbitRepo(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "orbit")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 9, 20, 10, 13, 0, 0, time.Local)
	git := func(author string, at time.Time, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		email := strings.ToLower(strings.Fields(author)[0]) + "@orbit.dev"
		date := at.Format(time.RFC3339)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME="+author, "GIT_AUTHOR_EMAIL="+email, "GIT_AUTHOR_DATE="+date,
			"GIT_COMMITTER_NAME="+author, "GIT_COMMITTER_EMAIL="+email, "GIT_COMMITTER_DATE="+date)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name, text string) {
		t.Helper()
		os.MkdirAll(filepath.Join(dir, filepath.Dir(name)), 0o755)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	commit := func(author string, at time.Time, msg string, files map[string]string) {
		t.Helper()
		for name, text := range files {
			write(name, text)
		}
		git(author, at, "add", ".")
		git(author, at, "commit", "-q", "-m", msg)
	}
	git("Mia Chen", day, "init", "-q", "-b", "main")
	git("Mia Chen", day, "config", "commit.gpgsign", "false")
	commit("Mia Chen", day, "Start orbit", map[string]string{"README.md": "# orbit\n\nTrack satellites as they pass.\n"})
	commit("Leo Park", day.Add(20*time.Hour), "Fetch the positions from the API", map[string]string{"src/api.ts": "export async function fetchAll() {\n  return (await fetch('/positions')).json()\n}\n"})
	commit("Ava Rossi", day.Add(46*time.Hour), "Draw the orbits on the map", map[string]string{"src/map.ts": "export function draw() {}\n"})
	git("Mia Chen", day.Add(70*time.Hour), "switch", "-q", "-c", "feature/orbits")
	commit("Mia Chen", day.Add(72*time.Hour), "Show each orbit's period", map[string]string{"src/orbits.ts": "export const period = (o: Orbit) => o.period\n"})
	git("Mia Chen", day.Add(73*time.Hour), "switch", "-q", "main")
	commit("Leo Park", day.Add(74*time.Hour), "Retry the first fetch", map[string]string{"src/api.ts": "export async function fetchAll() {\n  return (await fetch('/positions', { cache: 'no-store' })).json()\n}\n"})
	git("Mia Chen", day.Add(170*time.Hour), "merge", "-q", "--no-ff", "-m", "Merge branch 'feature/orbits'", "feature/orbits")
	git("Mia Chen", day.Add(170*time.Hour), "tag", "v0.2.0")
	commit("Ava Rossi", day.Add(192*time.Hour), "Keep the search when the page reloads", map[string]string{"src/search.ts": "export const keep = true\n"})
	git("Noah Kim", day.Add(208*time.Hour), "switch", "-q", "-c", "feature/alerts")
	commit("Noah Kim", day.Add(209*time.Hour), "Notify before a satellite passes overhead", map[string]string{"src/alerts.ts": "export function notify() {}\n"})
	commit("Noah Kim", day.Add(223*time.Hour), "Let alerts be snoozed", map[string]string{"src/alerts.ts": "export function notify() {}\nexport function snooze() {}\n"})
	git("Noah Kim", day.Add(224*time.Hour), "switch", "-q", "main")
	commit("Leo Park", day.Add(228*time.Hour), "Cache positions for a minute", map[string]string{"src/cache.ts": "export const ttl = 60_000\n"})
	commit("Mia Chen", day.Add(231*time.Hour), "Document the configuration", map[string]string{"README.md": "# orbit\n\nTrack satellites as they pass.\n\n## Configuration\n"})
	write("src/api.ts", "export async function fetchAll(tries = 3) {\n  for (let i = 1; ; i++) {\n    try {\n      return (await fetch('/positions')).json()\n    } catch (e) {\n      if (i === tries) throw e\n    }\n  }\n}\n")
	return dir
}
