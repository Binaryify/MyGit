package main

import (
	"context"
	"path"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mygit/internal/git"
	"mygit/internal/graph"
	"mygit/internal/patch"
)

// logBatch is how many commits the history shows at a time as it loads.
const logBatch = 2000

// app is a repository's window: its history, its changes, and the diff of
// what was chosen last.
type app struct {
	repo   *git.Repo
	win    *mygo.Window
	update func(func()) // runs a function on the main thread, the window's Update

	// The history and its graph, a row per commit.
	commits []git.Commit
	rows    []graph.Row
	logList ui.ListState
	logSel  int
	logGen  int // the load running; the batches of older ones are dropped
	loading bool
	refs    string // the refs the history was read at, to reload it when they move
	scope   git.Scope

	// The history's search: the rows of the commits that match it.
	search    string
	searchFor string // what matches was found for, with the commits it looked at
	searched  int
	matches   []int
	searchBox ui.Handle

	refreshing, again bool // a refresh is running, and another is due after it

	status                   git.Status
	branches                 []string
	remotes                  []string // the remotes' branches
	op                       git.Operation
	stashes                  []git.Stash
	stashList                ui.ListState
	stashSel                 int
	leftTab                  int // the left panel's: changes or the stash
	staged                   []git.Change
	unstaged                 []git.Change
	stagedList, unstagedList ui.ListState
	stagedSel, unstagedSel   int

	message string
	amend   bool

	detail   detail
	fileList ui.ListState // of the commit's files

	confirm confirmation
	prompt  prompt

	busy     string // the operation running, as the toolbar says it
	errMsg   string
	sidebarW float32
	logH     float32
	filesW   float32
}

// detail is what the bottom of the window shows: a commit's files and the
// diff of one, or the diff of a change.
type detail struct {
	commit *git.Commit // nil for a change
	change *git.Change
	staged bool // the change's staged side, else its working tree's

	files   []editor.FilePatch
	fileSel int
	diff    *editor.DiffState
	merge   *merger           // a file in conflict's
	raw     string            // the change's patch as git printed it
	patch   *editor.FilePatch // the change's, when its lines can be chosen
	gen     int
	loading bool
}

func newApp(repo *git.Repo) *app {
	return &app{repo: repo, logSel: -1, stagedSel: -1, unstagedSel: -1, stashSel: -1, sidebarW: 300, logH: 360, filesW: 260}
}

func (a *app) start() {
	a.logList.Selected = &a.logSel
	a.logList.Key = func(i int) any { return a.commits[a.commitAt(i)].Hash }
	a.logList.Label = func(i int) string { return a.commits[a.commitAt(i)].Subject }
	a.stagedList.Selected, a.unstagedList.Selected = &a.stagedSel, &a.unstagedSel
	a.stashList.Selected = &a.stashSel
	a.fileList.Selected = &a.detail.fileSel
	if a.win != nil {
		a.update = a.win.Update
		a.win.OnFocus(a.refresh) // the repository may have changed in a terminal
		a.watch()
	}
	a.refresh()
}

// watch refreshes as files of the working tree or the repository change,
// as editors and terminals change them. A tree too big to watch is
// refreshed as the window is focused.
func (a *app) watch() {
	ctx, cancel := context.WithCancel(context.Background())
	a.win.OnClosed(cancel)
	go func() {
		w, err := mygo.WatchFiles(ctx, a.repo.Dir, mygo.FileWatchOptions{
			Recursive: true,
			HintsOnly: true,
			Ignore: func(p string, isDir bool) bool {
				base := path.Base(p)
				return isDir && (base == "node_modules" || base == "objects" && path.Base(path.Dir(p)) == ".git" ||
					base == "logs" && path.Base(path.Dir(p)) == ".git")
			},
		})
		if err != nil {
			return
		}
		defer w.Close()
		for {
			if _, err := w.Next(ctx); err != nil {
				return
			}
			a.update(a.refresh)
		}
	}()
}

// work runs fn off the main thread, then done on it.
func work[T any](a *app, fn func(ctx context.Context) (T, error), done func(T, error)) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		v, err := fn(ctx)
		a.update(func() { done(v, err) })
	}()
}

// run runs a git operation that changes the repository, saying it in the
// toolbar, and refreshes what shows after it.
func (a *app) run(label string, fn func(ctx context.Context) error, then func()) {
	if a.busy != "" {
		return
	}
	a.busy, a.errMsg = label, ""
	work(a, func(ctx context.Context) (struct{}, error) { return struct{}{}, fn(ctx) }, func(_ struct{}, err error) {
		a.busy = ""
		if err != nil {
			a.errMsg = label + "失败：" + err.Error()
		} else if then != nil {
			then()
		}
		a.refresh()
	})
}

type snapshot struct {
	status   git.Status
	branches []string
	remotes  []string
	refs     string
	op       git.Operation
	stashes  []git.Stash
}

// refresh reads the status, and the history again when a ref moved. A
// refresh asked for while one runs runs once after it.
func (a *app) refresh() {
	if a.refreshing {
		a.again = true
		return
	}
	a.refreshing = true
	work(a, func(ctx context.Context) (snapshot, error) {
		st, err := a.repo.Status(ctx)
		if err != nil {
			return snapshot{}, err
		}
		b, _ := a.repo.Branches(ctx)
		rb, _ := a.repo.RemoteBranches(ctx)
		stashes, _ := a.repo.Stashes(ctx)
		return snapshot{st, b, rb, a.repo.Refs(ctx), a.repo.Operation(), stashes}, nil
	}, func(s snapshot, err error) {
		a.refreshing = false
		if a.again {
			a.again = false
			defer a.refresh()
		}
		if err != nil {
			a.errMsg = err.Error()
			return
		}
		a.setStatus(s.status)
		a.branches, a.remotes, a.op, a.stashes = s.branches, s.remotes, s.op, s.stashes
		a.stashSel = min(a.stashSel, len(a.stashes)-1)
		if s.refs != a.refs || a.commits == nil && !a.loading {
			a.refs = s.refs
			a.loadLog()
		}
	})
}

func (a *app) setStatus(st git.Status) {
	a.status = st
	a.staged, a.unstaged = a.staged[:0], a.unstaged[:0]
	for _, c := range st.Changes {
		if c.Staged != 0 {
			a.staged = append(a.staged, c)
		}
		if c.Work != 0 {
			a.unstaged = append(a.unstaged, c)
		}
	}
	a.stagedSel = min(a.stagedSel, len(a.staged)-1)
	a.unstagedSel = min(a.unstagedSel, len(a.unstaged)-1)
	// The diff of a change shows it as it is now, or nothing once it's gone.
	if ch := a.detail.change; ch != nil {
		if c, ok := find(a.changes(a.detail.staged), ch.Path); ok {
			a.showChange(c, a.detail.staged)
		} else {
			a.detail = detail{}
		}
	}
}

func (a *app) changes(staged bool) []git.Change {
	if staged {
		return a.staged
	}
	return a.unstaged
}

func find(changes []git.Change, path string) (git.Change, bool) {
	for _, c := range changes {
		if c.Path == path {
			return c, true
		}
	}
	return git.Change{}, false
}

// loadLog reads the history again, keeping the commit chosen.
func (a *app) loadLog() {
	a.logGen++
	gen := a.logGen
	chosen := ""
	if a.logSel >= 0 && a.logSel < len(a.commits) {
		chosen = a.commits[a.logSel].Hash
	}
	if a.searching() {
		chosen = "" // the row of a match isn't the commit's
	}
	a.commits, a.rows, a.loading, a.logSel = nil, nil, true, -1
	a.matches, a.searched = a.matches[:0], 0
	go func() {
		var layout graph.Layout
		err := a.repo.Log(context.Background(), a.scope, logBatch, func(batch []git.Commit) {
			rows := make([]graph.Row, len(batch))
			for i := range batch {
				rows[i] = layout.Add(batch[i].Hash, batch[i].Parents)
			}
			a.update(func() {
				if gen != a.logGen {
					return
				}
				for i := range batch {
					if batch[i].Hash == chosen {
						a.logSel = len(a.commits) + i
					}
				}
				a.commits = append(a.commits, batch...)
				a.rows = append(a.rows, rows...)
			})
		})
		a.update(func() {
			if gen != a.logGen {
				return
			}
			a.loading = false
			if err != nil {
				a.errMsg = "读取历史失败：" + err.Error()
			}
		})
	}()
}

// showCommit loads the files a commit changed.
func (a *app) showCommit(i int) {
	c := a.commits[i]
	a.showPatch(c, func(ctx context.Context) (string, error) { return a.repo.Show(ctx, c.Hash) })
}

// showStash shows a stash's files as a commit's.
func (a *app) showStash(s git.Stash) {
	c := git.Commit{Hash: s.Hash, Time: s.Time, Subject: s.Subject, Refs: []string{s.Ref}}
	a.showPatch(c, func(ctx context.Context) (string, error) { return a.repo.StashShow(ctx, s.Ref) })
}

// showPatch loads the files of a commit's patch, which load reads.
func (a *app) showPatch(c git.Commit, load func(ctx context.Context) (string, error)) {
	if a.detail.commit != nil && a.detail.commit.Hash == c.Hash {
		return
	}
	a.detail = detail{commit: &c, gen: a.detail.gen + 1, loading: true}
	gen := a.detail.gen
	work(a, func(ctx context.Context) ([]editor.FilePatch, error) {
		out, err := load(ctx)
		if err != nil {
			return nil, err
		}
		return editor.ParsePatch(out)
	}, func(files []editor.FilePatch, err error) {
		if gen != a.detail.gen {
			return
		}
		a.detail.loading = false
		if err != nil {
			a.errMsg = err.Error()
			return
		}
		a.detail.files = files
		a.detail.fileSel = -1
		if len(files) > 0 {
			a.showFile(0)
		}
	})
}

// showFile shows the diff of a file of the commit shown.
func (a *app) showFile(i int) {
	a.detail.fileSel = i
	a.detail.diff = patchDiff(a.detail.files[i])
}

// showChange shows the diff of a change, staged or in the working tree.
// Shown again as the change moves on, it keeps the diff, with its
// selection and scroll, while the patch is the same.
func (a *app) showChange(c git.Change, staged bool) {
	if c.Conflict {
		if a.detail.merge == nil || a.detail.merge.path != c.Path {
			a.showConflict(c)
		}
		return
	}
	same := a.detail.change != nil && a.detail.change.Path == c.Path && a.detail.staged == staged
	if !same {
		a.detail = detail{change: &c, staged: staged, gen: a.detail.gen + 1, loading: true}
	}
	gen := a.detail.gen
	type result struct {
		raw   string
		patch *editor.FilePatch
		diff  *editor.DiffState
	}
	work(a, func(ctx context.Context) (result, error) {
		if c.Work == '?' && !staged { // untracked: all of it is new
			text, err := a.repo.ReadFile(c.Path)
			if err != nil {
				return result{}, err
			}
			return result{raw: text, diff: textDiff(c.Path, "", text)}, nil
		}
		out, err := a.repo.Diff(ctx, c.Path, staged)
		if err != nil {
			return result{}, err
		}
		files, err := editor.ParsePatch(out)
		if err != nil || len(files) == 0 {
			return result{raw: out}, err
		}
		return result{raw: out, patch: &files[0], diff: patchDiff(files[0])}, nil
	}, func(r result, err error) {
		if gen != a.detail.gen {
			return
		}
		a.detail.loading = false
		if err != nil {
			a.errMsg = err.Error()
		}
		if same && r.raw == a.detail.raw && a.detail.diff != nil {
			return
		}
		a.detail.raw, a.detail.patch, a.detail.diff = r.raw, r.patch, r.diff
		a.detail.change = &c
		if r.patch != nil && patch.Partial(*r.patch) {
			a.reviewLines(r.diff)
		}
	})
}

func patchDiff(p editor.FilePatch) *editor.DiffState {
	d := &editor.DiffState{}
	d.SetPatch(p)
	if lang := editor.LanguageFor(fileName(p)); lang != nil {
		d.Old.SetLanguage(lang)
		d.New.SetLanguage(lang)
	}
	return d
}

func textDiff(name, old, new string) *editor.DiffState {
	d := &editor.DiffState{Old: editor.NewBuffer(old), New: editor.NewBuffer(new)}
	if lang := editor.LanguageFor(name); lang != nil {
		d.Old.SetLanguage(lang)
		d.New.SetLanguage(lang)
	}
	return d
}

func fileName(p editor.FilePatch) string {
	if p.NewName != "" {
		return p.NewName
	}
	return p.OldName
}

func (a *app) stage(c git.Change) {
	a.run("暂存", func(ctx context.Context) error { return a.repo.Stage(ctx, c.Path) }, nil)
}

func (a *app) unstage(c git.Change) {
	paths := []string{c.Path}
	if c.OldPath != "" {
		paths = append(paths, c.OldPath) // a rename is staged as both
	}
	a.run("取消暂存", func(ctx context.Context) error { return a.repo.Unstage(ctx, paths...) }, nil)
}

// discard throws a change of the working tree away, once the user says so.
func (a *app) discard(c git.Change) {
	msg := "“" + c.Path + "”的更改会被丢弃，无法撤销。"
	if c.Work == '?' {
		msg = "未跟踪的文件“" + c.Path + "”会被删除，无法撤销。"
	}
	a.ask("丢弃更改？", msg, "丢弃", func() {
		a.run("丢弃", func(ctx context.Context) error { return a.repo.Discard(ctx, c) }, nil)
	})
}

func (a *app) stageAll() {
	a.run("暂存", func(ctx context.Context) error { return a.repo.Stage(ctx, ".") }, nil)
}

func (a *app) unstageAll() {
	a.run("取消暂存", func(ctx context.Context) error { return a.repo.Unstage(ctx, ".") }, nil)
}

func (a *app) canCommit() bool {
	return a.busy == "" && a.message != "" && (len(a.staged) > 0 || a.amend)
}

func (a *app) commit() {
	if !a.canCommit() {
		return
	}
	msg, amend := a.message, a.amend
	a.run("提交", func(ctx context.Context) error { return a.repo.Commit(ctx, msg, amend) }, func() {
		a.message, a.amend = "", false
	})
}

// toggleAmend starts amending from HEAD's message, and stops with an
// empty one.
func (a *app) toggleAmend() {
	if a.amend {
		a.message = a.repo.LastMessage(context.Background())
	} else if a.message == a.repo.LastMessage(context.Background()) {
		a.message = ""
	}
}
