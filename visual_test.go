package main

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"mygit/internal/git"
	"mygit/internal/helper"
	"mygit/internal/remote"
	loc "mygit/locales"
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

	w := &welcome{ssh: connectPlan{open: true, connected: "devbox", listing: helper.Listing{Dir: "/home/mia", Parent: "/home", Entries: []helper.Entry{
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

	a := newApp(helperRepo(t, "devbox", orbitRepo(t)))
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
	demoRepo(t, dir)
	return dir
}

// gitAs runs git in dir as author, at a time.
type gitAs func(author string, at time.Time, args ...string)

// teamGit makes dir and returns its git, run as an author at a time.
func teamGit(t *testing.T, dir string) gitAs {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return func(author string, at time.Time, args ...string) {
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
}

// demoRepo makes orbit in dir, and returns its git, to go on with.
func demoRepo(t *testing.T, dir string) gitAs {
	t.Helper()
	day := time.Date(2026, 9, 20, 10, 13, 0, 0, time.Local)
	git := teamGit(t, dir)
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
	return git
}

// TestDumpWelcome renders the welcome window into MYGIT_DUMP, 520×640 at
// 2×, as the READMEs and the site show it: repositories opened lately, one
// on another computer. MYGIT_LOCALE renders it in another language, as en.
func TestDumpWelcome(t *testing.T) {
	out := os.Getenv("MYGIT_DUMP")
	if out == "" {
		t.Skip()
	}
	if l := os.Getenv("MYGIT_LOCALE"); l != "" {
		i18n.SetLocaleFunc(func() string { return l })
		defer i18n.SetLocaleFunc(func() string { return "zh-Hans" })
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	var recent []string
	for _, name := range []string{"conflict", "atlas"} {
		dir := filepath.Join(home, "Projects", name)
		os.MkdirAll(dir, 0o755)
		recent = append(recent, dir)
	}
	old := prefs.Remotes
	prefs.Remotes = []string{"devbox:/home/mia/orbit"}
	defer func() { prefs.Remotes = old }()
	w := &welcome{recent: recent}
	tt := ui.NewTester(w.view, 520, 640)
	tt.SetDark(true)
	tt.SetScale(2)
	tt.SetTitleBar(ui.TitleBar{Left: 80, Height: 28})
	tt.Frame()
	f, _ := os.Create(out + "/welcome.png")
	png.Encode(f, tt.Image())
	f.Close()
}

// TestDumpMerge renders the merge window into MYGIT_DUMP as the READMEs
// show it, merge-zh.png, or merge-en.png with MYGIT_LOCALE=en: orbit's
// theme merged with a branch that changed it too, 1120×746 at 1.4×, the
// traffic lights macOS draws painted where it draws them. merge-wand,
// merge-base and merge-deleted show it after the magic wand, comparing
// the base with the left, and a file the left deleted; ssh-merge and
// wsl-merge the merge over SSH and through WSL.
func TestDumpMerge(t *testing.T) {
	sc := newShowcase(t)
	dir := filepath.Join(t.TempDir(), "orbit")
	run := demoRepo(t, dir)
	day := time.Date(2026, 10, 2, 9, 40, 0, 0, time.Local)
	theme := func(accent, trail, glow, draw string) string {
		return "export const theme = {\n  dark: true,\n  accent: '" + accent + "',\n  orbit: 1.5," + trail +
			"\n  label: 12,\n  glow: " + glow + ",\n}\n\nexport function paint(s) {\n  ring(s, theme.orbit)\n  " + draw + "\n}\n"
	}
	commit := func(author string, at time.Time, msg, text string) {
		if err := os.WriteFile(filepath.Join(dir, "src/theme.ts"), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		run(author, at, "add", ".")
		run(author, at, "commit", "-q", "-m", msg)
	}
	run("Mia Chen", day, "stash", "-q") // the change of the working tree
	if err := os.WriteFile(filepath.Join(dir, "src/legacy.ts"), []byte("// The theme before the dark one.\nexport const light = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commit("Mia Chen", day, "Add a dark theme", theme("#3b82f6", "", "0.4", "dot(s, 2, 'white')"))
	run("Ava Rossi", day.Add(30*time.Hour), "switch", "-q", "-c", "feature/palette")
	if err := os.WriteFile(filepath.Join(dir, "src/legacy.ts"), []byte("// The theme before the dark one.\nexport const light = true\nexport const sat = palette.sat\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commit("Ava Rossi", day.Add(31*time.Hour), "Color satellites from the new palette",
		theme("#a78bfa", "\n  trail: 8,", "0.4", "dot(s, 2, palette.sat)"))
	run("Mia Chen", day.Add(32*time.Hour), "switch", "-q", "main")
	run("Mia Chen", day.Add(33*time.Hour), "rm", "-q", "src/legacy.ts")
	commit("Mia Chen", day.Add(33*time.Hour), "Brighten satellites in the dark theme", theme("#60a5fa", "", "0.8", "dot(s, 3, 'white')"))
	cmd := exec.Command("git", "merge", "-q", "feature/palette")
	cmd.Dir = dir
	if err := cmd.Run(); err == nil {
		t.Fatal("merged without a conflict")
	}

	repo, err := git.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(repo)
	tt := &tester{a: a, queue: make(chan func(), 64)}
	a.update = func(fn func()) { tt.queue <- fn }
	tt.Tester = sc.tester(a.view, 1120, 746)
	a.start()
	tt.settle()
	shot := func(name string) { sc.save(t, tt.Tester, name) }
	tt.click(t, "theme.ts")
	shot("merge")
	m := a.detail.merge
	m.resolveSimple()
	shot("merge-wand")
	m.compareBase(paneOurs, loc.CompareBaseLeft())
	shot("merge-base")
	m.compare = nil
	tt.click(t, "legacy.ts")
	shot("merge-deleted")

	// The same merge on another computer: over SSH, and through WSL in
	// Windows' window.
	remoteShot := func(host, name string) {
		ra := newApp(helperRepo(t, host, dir))
		rt := &tester{a: ra, queue: make(chan func(), 64)}
		ra.update = func(fn func()) { rt.queue <- fn }
		rt.Tester = sc.tester(ra.view, 1120, 746)
		ra.start()
		rt.settle()
		rt.click(t, "theme.ts")
		sc.save(t, rt.Tester, name)
	}
	remoteShot("devbox", "ssh-merge")
	sc.asWindows(t)
	remoteShot(remote.WSLHost("Ubuntu-24.04"), "wsl-merge")
}

// showcase renders windows as the READMEs show them, into MYGIT_DUMP:
// dark at 1.4×, named -zh, or -en with MYGIT_LOCALE=en, the window's
// controls painted where the system draws them, macOS' traffic lights or,
// for Windows, its caption buttons.
type showcase struct {
	out, lang string
	windows   bool
}

// newShowcase skips the test unless MYGIT_DUMP says where to render.
func newShowcase(t *testing.T) *showcase {
	out := os.Getenv("MYGIT_DUMP")
	if out == "" {
		t.Skip()
	}
	sc := &showcase{out: out, lang: "zh"}
	if l := os.Getenv("MYGIT_LOCALE"); l != "" {
		i18n.SetLocaleFunc(func() string { return l })
		t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return "zh-Hans" }) })
		sc.lang = l
	}
	return sc
}

// asWindows renders the windows after it as Windows': its caption buttons,
// Ctrl in the shortcuts, the menus in the windows.
func (sc *showcase) asWindows(t *testing.T) {
	old := macOS
	t.Cleanup(func() { macOS = old })
	sc.windows, macOS = true, false
}

const showcaseScale = 1.4

// tester renders view w×h points.
func (sc *showcase) tester(view func(*ui.Context), w, h int) *ui.Tester {
	tt := ui.NewTester(view, w, h)
	tt.SetDark(true)
	tt.SetScale(showcaseScale)
	bar := ui.TitleBar{Left: 80, Height: titleBarHeight}
	if sc.windows {
		bar = ui.TitleBar{Right: 138, Height: titleBarHeight}
	}
	tt.SetTitleBar(bar)
	return tt
}

// save writes the window as it is now.
func (sc *showcase) save(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	tt.Frame()
	img := tt.Image()
	pt := func(v float64) float64 { return v * showcaseScale }
	if sc.windows {
		// Windows 11's caption buttons, 46 points wide, their glyphs 10.
		w, c := float64(img.Bounds().Dx())/showcaseScale, color.RGBA{220, 220, 220, 255}
		y := float64(titleBarHeight) / 2
		x := w - 138 + 23
		line(img, pt(x-5), pt(y), pt(x+5), pt(y), pt(1), c)
		x += 46
		for _, e := range [][4]float64{{-5, -5, 5, -5}, {5, -5, 5, 5}, {5, 5, -5, 5}, {-5, 5, -5, -5}} {
			line(img, pt(x+e[0]), pt(y+e[1]), pt(x+e[2]), pt(y+e[3]), pt(1), c)
		}
		x += 46
		line(img, pt(x-5), pt(y-5), pt(x+5), pt(y+5), pt(1), c)
		line(img, pt(x-5), pt(y+5), pt(x+5), pt(y-5), pt(1), c)
	} else {
		// The traffic lights: 12 points across, 20 apart, from 16 by 13.
		for i, c := range []color.RGBA{{255, 95, 87, 255}, {254, 188, 46, 255}, {40, 200, 64, 255}} {
			disc(img, pt(float64(23+20*i)), pt(20), pt(6), c)
		}
	}
	f, err := os.Create(filepath.Join(sc.out, name+"-"+sc.lang+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, img)
}

// disc paints a disc of color c centered at x, y with radius r.
func disc(img *image.RGBA, x, y, r float64, c color.RGBA) {
	cover(img, x-r, y-r, x+r, y+r, c, func(px, py float64) bool { return (px-x)*(px-x)+(py-y)*(py-y) <= r*r })
}

// line paints a line w wide from x0, y0 to x1, y1.
func line(img *image.RGBA, x0, y0, x1, y1, w float64, c color.RGBA) {
	dx, dy := x1-x0, y1-y0
	n := dx*dx + dy*dy
	cover(img, min(x0, x1)-w, min(y0, y1)-w, max(x0, x1)+w, max(y0, y1)+w, c, func(px, py float64) bool {
		t := max(0, min(1, ((px-x0)*dx+(py-y0)*dy)/n)) // the nearest point of the line
		qx, qy := x0+t*dx-px, y0+t*dy-py
		return qx*qx+qy*qy <= w*w/4
	})
}

// cover paints c on img where in says, in the box from x0, y0 to x1, y1,
// its edges smoothed: as much of a pixel as its 4×4 samples are in.
func cover(img *image.RGBA, x0, y0, x1, y1 float64, c color.RGBA, in func(x, y float64) bool) {
	for py := int(y0 - 1); py <= int(y1+1); py++ {
		for px := int(x0 - 1); px <= int(x1+1); px++ {
			n := 0
			for sy := range 4 {
				for sx := range 4 {
					if in(float64(px)+(float64(sx)+0.5)/4, float64(py)+(float64(sy)+0.5)/4) {
						n++
					}
				}
			}
			if n == 0 {
				continue
			}
			a := float64(n) / 16
			bg := img.RGBAAt(px, py)
			mix := func(f, b uint8) uint8 { return uint8(float64(f)*a + float64(b)*(1-a) + 0.5) }
			img.SetRGBA(px, py, color.RGBA{mix(c.R, bg.R), mix(c.G, bg.G), mix(c.B, bg.B), 255})
		}
	}
}

// TestDumpShowcase renders, into MYGIT_DUMP, MyGit opening repositories on
// other computers, to show it: over SSH from macOS, choosing the host
// (ssh-connect), then a folder of it (ssh-folders), the folder of several
// repositories open together (ssh-workspace) and one of them alone
// (ssh-repo); and the same through WSL in Windows' windows (wsl-connect,
// wsl-folders, wsl-workspace, wsl-repo). As newShowcase names and sizes
// them.
func TestDumpShowcase(t *testing.T) {
	sc := newShowcase(t)
	home := t.TempDir()
	t.Setenv("HOME", home) // the welcome window's recent repositories
	var recent []string
	for _, name := range []string{"orbit", "atlas"} {
		dir := filepath.Join(home, "Projects", name)
		os.MkdirAll(dir, 0o755)
		recent = append(recent, dir)
	}

	// mia's home: orbit, and two repositories of the team's beside it.
	mia := filepath.Join(t.TempDir(), "mia")
	demoRepo(t, filepath.Join(mia, "orbit"))
	day := time.Date(2026, 10, 12, 9, 30, 0, 0, time.Local)
	team := func(name string, commits [][3]string, work map[string]string) {
		dir := filepath.Join(mia, name)
		run := teamGit(t, dir)
		run("Mia Chen", day, "init", "-q", "-b", "main")
		run("Mia Chen", day, "config", "commit.gpgsign", "false")
		for i, c := range commits { // author, message, file
			at := day.Add(time.Duration(i*7) * time.Hour)
			if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, c[2])), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, c[2]), []byte("// "+c[1]+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			run(c[0], at, "add", ".")
			run(c[0], at, "commit", "-q", "-m", c[1])
		}
		for name, text := range work {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	team("api-gateway", [][3]string{
		{"Leo Park", "Route the positions through the gateway", "routes.ts"},
		{"Noah Kim", "Check tokens before routing", "auth.ts"},
		{"Leo Park", "Log the time each route takes", "log.ts"},
	}, map[string]string{"routes.ts": "// Route the positions through the gateway\nexport const positions = '/v2/positions'\n", "limit.ts": "export const perMinute = 600\n"})
	team("infra", [][3]string{
		{"Ava Rossi", "Run the API on two zones", "deploy/api.tf"},
		{"Mia Chen", "Back the database up nightly", "deploy/db.tf"},
	}, map[string]string{"deploy/api.tf": "# Run the API on two zones\nzones = [\"a\", \"b\", \"c\"]\n"})
	repos := func(host string) []*git.Repo {
		var out []*git.Repo
		for _, name := range []string{"api-gateway", "infra", "orbit"} {
			out = append(out, helperRepo(t, host, filepath.Join(mia, name)))
		}
		return out
	}
	listing := helper.Listing{Dir: "/home/mia", Parent: "/home", Entries: []helper.Entry{
		{Name: "api-gateway", Repo: true}, {Name: "dotfiles"}, {Name: "infra", Repo: true},
		{Name: "notes"}, {Name: "orbit", Repo: true}, {Name: "scratch"},
	}}

	old := prefs.Remotes
	t.Cleanup(func() { prefs.Remotes = old })
	shoot := func(prefix, host string, hosts, distros []string) {
		prefs.Remotes = []string{host + ":/home/mia/orbit"}
		// Choosing the host, written or from ~/.ssh/config, or WSL's.
		w := &welcome{recent: recent, ssh: connectPlan{open: true, hosts: hosts, distros: distros, host: host}}
		sc.save(t, sc.tester(w.view, 1120, 746), prefix+"-connect")
		// Its folders, the repositories marked.
		w = &welcome{recent: recent, ssh: connectPlan{open: true, connected: host, listing: listing, path: listing.Dir}}
		sc.save(t, sc.tester(w.view, 1120, 746), prefix+"-folders")
		// The folder open: its repositories together, orbit's change shown.
		ws, tt, settle := startWorkspace(&workspace{host: host, dir: mia, closed: map[string]bool{}}, repos(host),
			func(view func(*ui.Context)) *ui.Tester { return sc.tester(view, 1120, 746) })
		if err := tt.Click("api.ts"); err != nil {
			t.Fatalf("%v: %q", err, tt.Texts())
		}
		settle()
		if ws.apps[ws.active].repo.Name() != "orbit" {
			t.Fatalf("active %s", ws.apps[ws.active].repo.Name())
		}
		sc.save(t, tt, prefix+"-workspace")
		// orbit alone.
		a := newApp(helperRepo(t, host, filepath.Join(mia, "orbit")))
		at := &tester{a: a, queue: make(chan func(), 64)}
		a.update = func(fn func()) { at.queue <- fn }
		at.Tester = sc.tester(a.view, 1120, 746)
		a.start()
		at.settle()
		at.click(t, "api.ts")
		sc.save(t, at.Tester, prefix+"-repo")
	}
	shoot("ssh", "devbox", []string{"devbox", "gpu-box", "staging"}, nil)
	sc.asWindows(t)
	shoot("wsl", remote.WSLHost("Ubuntu-24.04"), []string{"devbox", "gpu-box"}, []string{"Ubuntu-24.04", "Debian"})
}

// TestDumpTags renders, into MYGIT_DUMP, the branches' popover listing
// orbit's tags (tags), and the history of one chosen from its menu
// (tag-history), as newShowcase names and sizes them. The menu itself is
// the platform's, not drawn: the test logs its items.
func TestDumpTags(t *testing.T) {
	sc := newShowcase(t)
	dir := orbitRepo(t)
	run := teamGit(t, dir)
	day := time.Date(2026, 9, 20, 10, 13, 0, 0, time.Local)
	run("Mia Chen", day.Add(46*time.Hour), "tag", "v0.1.0", "HEAD~6")
	run("Mia Chen", day.Add(232*time.Hour), "tag", "-a", "v0.3.0", "-m", "Alerts and caching")
	r, err := git.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(r)
	at := &tester{a: a, queue: make(chan func(), 64)}
	a.update = func(fn func()) { at.queue <- fn }
	at.Tester = sc.tester(a.view, 1120, 746)
	a.start()
	at.settle()
	at.click(t, loc.Branches())
	sc.save(t, at.Tester, "tags")
	// v0.2.0, the second tag, under the heading: it shows in the history too.
	h, ok := at.Find(loc.BranchesTags())
	if !ok {
		t.Fatalf("texts %q", at.Texts())
	}
	at.ClickAt(h.X+30, h.Y+h.H+16+27)
	at.Frame()
	t.Logf("v0.2.0's menu: %q", at.Menu())
	if err := at.ChooseMenuItem(loc.TagHistory()); err != nil {
		t.Fatalf("%v; menu %q", err, at.Menu())
	}
	at.settle()
	sc.save(t, at.Tester, "tag-history")
}

// TestDumpWorktrees renders the branches' popover of a submodule with a
// worktree of its own (worktrees), and the dialog checking a branch out
// in another (worktree-new), which says the superproject keeps its
// submodule.
func TestDumpWorktrees(t *testing.T) {
	sc := newShowcase(t)
	orbit := orbitRepo(t)
	super := filepath.Join(filepath.Dir(orbit), "station")
	run := teamGit(t, super)
	day := time.Date(2026, 9, 20, 10, 13, 0, 0, time.Local)
	run("Mia Chen", day, "init", "-q", "-b", "main")
	run("Mia Chen", day, "-c", "protocol.file.allow=always", "submodule", "add", "-q", orbit, "orbit")
	run("Mia Chen", day, "commit", "-q", "-m", "Add orbit")
	r, err := git.Open(filepath.Join(super, "orbit"))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.AddWorktree(context.Background(), filepath.Join(filepath.Dir(super), "orbit-alerts"), "HEAD", "alerts"); err != nil {
		t.Fatal(err)
	}
	a := newApp(r)
	at := &tester{a: a, queue: make(chan func(), 64)}
	a.update = func(fn func()) { at.queue <- fn }
	at.Tester = sc.tester(a.view, 1120, 746)
	a.start()
	at.settle()
	at.click(t, loc.Branches())
	at.settle()
	sc.save(t, at.Tester, "worktrees")
	at.click(t, loc.Branches())
	a.newWorktree("main", true)
	at.Frame()
	sc.save(t, at.Tester, "worktree-new")
}
