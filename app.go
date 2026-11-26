package main

import (
	"context"
	"errors"
	"path"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mygit/internal/git"
	"mygit/internal/graph"
	"mygit/internal/patch"
	loc "mygit/locales"
)

// logBatch is how many commits the history shows at a time as it loads.
const logBatch = 2000

// app is a repository's window: its history, its changes, and the diff of
// what was chosen last.
type app struct {
	repo   *git.Repo
	win    *mygo.Window
	update func(func())  // runs a function on the main thread, the window's Update
	online chan struct{} // a refresh reached the repository, for a watch to start again
	ws     *workspace    // the workspace it is one of, nil in a window of its own

	// The history and its graph, a row per commit.
	commits   []git.Commit
	rows      []graph.Row
	logList   ui.ListState
	logSel    int
	logChosen ui.Selection[any] // the commits chosen, two to compare
	logGen    int               // the load running; the batches of older ones are dropped
	loading   bool
	refs      string // the refs the history was read at, to reload it when they move
	scope     git.Scope

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
	remoteURLs               []git.Remote
	tags                     []string // the newest first
	worktrees                []git.Worktree
	super                    string          // the superproject's working tree, of a submodule
	submodules               []git.Submodule // the repository's, theirs too, checked out or not
	op                       git.Operation
	stashes                  []git.Stash
	stashList                ui.ListState
	stashSel                 int
	leftTab                  int // the left panel's: changes or the stash
	staged                   []git.Change
	unstaged                 []git.Change
	chosen                   map[string]bool // the one list's changes to commit, by path; not set, the default (#37)
	genMarked                map[string]bool // the changes .gitattributes marks generated
	stagedList, unstagedList ui.ListState
	stagedSel, unstagedSel   int
	stagedRow, unstagedRow   int             // the rows of the changes chosen, as the filter shows them
	unpushed                 map[string]bool // the branch's commits its upstream hasn't, by hash
	foldedDirs               map[string]bool // the folders folded of the lists shown as trees, by list and path
	filter                   changeFilter

	message string
	amend   bool
	// The recent messages Up in the message box brings back, and how far
	// back the one it shows is: 0 for one typed.
	recalled  []string
	recall    int
	recallEnd bool // put the caret after the message brought back

	detail   detail
	fileList ui.ListState // of the commit's files
	fileRow  int          // its row chosen, a file's or, shown as a tree, a folder's

	unfolded map[string]bool // generated files whose diffs were asked for, by foldKey
	viewed   map[string]bool // files marked viewed, by viewKey

	confirm      confirmation
	nextConflict bool   // to open the next file in conflict once the status comes
	branchesOpen bool   // the branches' popover
	branchFilter string // its search
	prompt       prompt
	worktree     worktreeDialog
	rebase       rebasePlan
	reword       rewordPlan
	push         pushPlan
	cred         credPrompt
	remotes2     remotesPlan // the remotes dialog
	identity     identityDialog
	settings     settingsDialog
	palette      palette   // ⌘P's files and ⌘K's commands
	comments     []comment // the review's, to copy as Markdown
	composer     composer
	goingOn      bool // asked whether to save first, the user said no
	pulls        pullsPlan
	submit       submitPlan
	pullReviews  map[int]*pullReview // the pull requests opened, with their reviews

	busy     string // the operation running, as the toolbar says it
	errMsg   string
	notice   string // news that isn't an error, as an update check's
	toast    string // a toast to show, said off the UI thread
	trustDir string // a repository another user owns, dropped, to trust
	// undoBranches are the local branches deleted while the app runs, which
	// it offers to bring back until it quits, as IDEA does.
	undoBranches []deletedBranch
	sidebarW     float32
	logH         float32
	filesW       float32
	stagedPct    float32 // of the changes' room, the staged changes'
}

// detail is what the bottom of the window shows: a commit's files and the
// diff of one, or the diff of a change.
type detail struct {
	commit *git.Commit // nil for a change
	change *git.Change
	staged bool // the change's staged side, else its working tree's

	files      []editor.FilePatch
	fileSel    int
	generated  []bool      // of files, those people don't write
	folded     bool        // the file shown is generated, its diff folded until asked
	binary     bool        // the change shown is of a binary file, without lines
	disk       string      // the file edited as it was on disk when its edits began, to save over it knowingly
	images     *images     // the file shown is a picture: its sides, in place of a diff
	pr         *pullReview // the pull request whose changes the commit is
	diff       *editor.DiffState
	starts     []int                // the lines of the diff's new side its hunks start at, for the minimap's bubble
	merge      *merger              // a file in conflict's
	blame      *blame               // a file's, shown in place of the diff
	notes      *annotations         // who last changed the diff's lines, beside them (#51)
	load       patchLoad            // reads the commit's patch
	message    string               // the commit's whole message
	parent     int                  // the merge's parent its change is against
	raw        string               // the change's patch as git printed it
	whole      bool                 // the change shows whole files, the new one to edit
	patch      *editor.FilePatch    // the change's, when its lines can be chosen
	patchWhole bool                 // the diff shows the whole files, a patch's changes on them
	all        *allChanges          // a list's changes one after another, in place of one (#44)
	commitAll  []allFile            // the commit's files one after another, as prefs.CommitAll asks
	allReveal  int                  // the file one after another to scroll to, plus 1; 0 for none
	fold       bool                 // the whole files fold away from the changes, as the file is long
	reveal     bool                 // the first change waits to be scrolled into view
	sides      [2]textSide          // where the diff's texts come from: old first, then new
	texts      map[string][2]string // the texts of the files' sides, read with the patch
	gen        int
	loading    bool
	// F7's past the diff's last change (1) or ⇧F7's before its first (-1),
	// with the caret it left, saying the next goes on to another file.
	edge, edgeAt int
	enter        int // the way the change loading was stepped into, to open at its first change or last
}

func newApp(repo *git.Repo) *app {
	return &app{repo: repo, scope: git.Scope{Ref: "HEAD"}, online: make(chan struct{}, 1), logSel: -1, stagedSel: -1, unstagedSel: -1, stashSel: -1,
		sidebarW: prefs.SidebarW, logH: prefs.LogH, filesW: prefs.FilesW, stagedPct: prefs.StagedPct,
		unfolded: map[string]bool{}, viewed: map[string]bool{}, pullReviews: map[int]*pullReview{}, chosen: map[string]bool{}}
}

func (a *app) start() {
	a.logList.Selected = &a.logSel
	a.logList.Selection = &a.logChosen
	a.logList.Key = func(i int) any { return a.commits[a.commitAt(i)].Hash }
	a.logList.Label = func(i int) string { return a.commits[a.commitAt(i)].Subject }
	a.stagedList.Selected, a.unstagedList.Selected = &a.stagedRow, &a.unstagedRow
	a.stashList.Selected = &a.stashSel
	a.fileList.Selected = &a.fileRow
	if a.win != nil {
		a.update = a.win.Update
		a.win.OnFocus(a.refresh) // the repository may have changed in a terminal
		// A window closed with a file edited and not saved asks first.
		if a.ws == nil { // a workspace asks for its repositories
			a.win.OnClose(func(e *mygo.CloseEvent) {
				if a.unsaved() && a.leaving(a.win.Close) {
					e.PreventDefault()
				}
			})
		}
		a.win.OnClosed(func() { // the next window opens as this one was laid out
			prefs.SidebarW, prefs.LogH, prefs.FilesW, prefs.StagedPct = a.sidebarW, a.logH, a.filesW, a.stagedPct
			prefs.save()
		})
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
	if !a.local() {
		go a.watchRemote(ctx)
		return
	}
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

// watchRemote has the helper watch a tree on another computer. Once the
// connection ends it waits for a refresh to reach the computer again, as
// the window is focused, and watches again.
func (a *app) watchRemote(ctx context.Context) {
	h := remoteHost{a.repo.Host()}
	for {
		_ = h.Watch(ctx, a.repo.Dir, func() { a.update(a.refresh) })
		select {
		case <-ctx.Done():
			return
		case <-a.online:
		}
	}
}

// local tells whether the repository is on this computer.
func (a *app) local() bool { return a.repo.Host() == "" }

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
			a.errMsg = loc.OpFailed(label, err.Error())
			if git.NoIdentity(err) {
				a.openIdentity() // what git asks for, to commit again
			}
		} else if then != nil {
			then()
		}
		a.refresh()
	})
}

// runPull runs a pull, and when untracked files are in the way of files
// it brings, offers to stash them and pull again, as they would be lost.
func (a *app) runPull(label string, pull func(ctx context.Context) error) {
	pull = a.upToDate(pull)
	a.run(label, func(ctx context.Context) error {
		err := pull(ctx)
		files := git.UntrackedInTheWay(err)
		if len(files) == 0 {
			return err
		}
		a.update(func() {
			a.ask(loc.UntrackedInTheWayTitle(), loc.UntrackedInTheWayMessage(listed(files, 10)), loc.StashAndPull(), func() {
				a.run(label, func(ctx context.Context) error {
					if err := a.repo.StashFiles(ctx, loc.UntrackedStashMessage(), files); err != nil {
						return err
					}
					return pull(ctx)
				}, nil)
			})
		})
		return nil
	}, nil)
}

// upToDate runs fn, and when it moved neither HEAD, its upstream nor the
// refs also named says so with a toast, as git's "Already up to date"
// (#57).
func (a *app) upToDate(fn func(ctx context.Context) error, also ...string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		before := a.repo.Tips(ctx, also...)
		if err := fn(ctx); err != nil {
			return err
		}
		if a.repo.Tips(ctx, also...) == before {
			a.update(func() { a.toast = loc.UpToDate() })
		}
		return nil
	}
}

// showToast shows the toast said, once.
func (a *app) showToast(c *ui.Context) {
	if a.toast != "" {
		c.Toast(a.toast)
		a.toast = ""
	}
}

// listed lists items a line each, the first n of them and how many more.
func listed(items []string, n int) string {
	if len(items) <= n {
		return strings.Join(items, "\n")
	}
	return strings.Join(items[:n], "\n") + "\n" + loc.AndMore(len(items)-n)
}

type snapshot struct {
	status   git.Status
	branches []string
	remotes  []string
	urls     []git.Remote // the remotes, with their URLs
	tags     []string
	refs     string
	op       git.Operation
	stashes  []git.Stash
	marked   map[string]bool // the changes .gitattributes marks generated
	unpushed map[string]bool // the branch's commits its upstream hasn't, by hash
	subs     []git.Submodule
}

// refresh reads the status, and the history again when a ref moved. A
// refresh asked for while one runs runs once after it.
func (a *app) refresh() {
	a.checkPull()
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
		tags, _ := a.repo.Tags(ctx)
		stashes, _ := a.repo.Stashes(ctx)
		urls, _ := a.repo.Remotes(ctx)
		paths := make([]string, len(st.Changes))
		for i, c := range st.Changes {
			paths[i] = c.Path
		}
		var unpushed map[string]bool
		if st.Upstream != "" && st.Ahead > 0 {
			unpushed = a.repo.Unpushed(ctx)
		}
		subs, _ := a.repo.Submodules(ctx)
		return snapshot{st, b, rb, urls, tags, a.repo.Refs(ctx), a.repo.Operation(ctx), stashes, a.repo.Generated(ctx, paths), unpushed, subs}, nil
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
		select {
		case a.online <- struct{}{}:
		default:
		}
		a.genMarked, a.unpushed = s.marked, s.unpushed
		reload := s.refs != a.refs || a.commits == nil && !a.loading
		if s.status.Branch != a.status.Branch && a.scope.Ref != "HEAD" && a.scope.Ref != "" {
			// Switching branches shows the new one's history; all of
			// them stay all.
			a.scope.Ref, reload = "HEAD", true
		}
		a.setStatus(s.status)
		a.branches, a.remotes, a.tags, a.op, a.stashes = s.branches, s.remotes, s.tags, s.op, s.stashes
		a.remoteURLs = s.urls
		a.submodules = s.subs
		a.stashSel = min(a.stashSel, len(a.stashes)-1)
		if reload {
			a.refs = s.refs
			a.loadLog()
		}
	})
}

func (a *app) setStatus(st git.Status) {
	a.status = st
	// The changes chosen stay chosen by their paths, as rows come and go;
	// one gone leaves its list without a choice, so that choosing a row
	// shows it.
	stagedPath, unstagedPath := chosenPath(a.staged, a.stagedSel), chosenPath(a.unstaged, a.unstagedSel)
	a.staged, a.unstaged = a.staged[:0], a.unstaged[:0]
	if prefs.OneList {
		// The one list shows every change against HEAD; the index is not
		// its own, and a check only says what the commit includes (#37).
		a.unstaged = append(a.unstaged, st.Changes...)
		for p := range a.chosen {
			if _, ok := find(st.Changes, p); !ok {
				delete(a.chosen, p)
			}
		}
	} else {
		for _, c := range st.Changes {
			if c.Staged != 0 {
				a.staged = append(a.staged, c)
			}
			if c.Work != 0 {
				a.unstaged = append(a.unstaged, c)
			}
		}
	}
	a.stagedSel, a.unstagedSel = indexOf(a.staged, stagedPath), indexOf(a.unstaged, unstagedPath)
	a.refilter()
	if a.nextConflict { // a file resolved: on to the next in conflict
		a.nextConflict = false
		for i, c := range a.unstaged {
			if c.Conflict {
				a.chooseChange(i, false)
				return
			}
		}
	}
	if all := a.detail.all; all != nil { // the list's changes as they are now
		a.showAll(all.staged)
	}
	// The diff of a change shows it as it is now, or nothing once it's gone.
	if ch := a.detail.change; ch != nil {
		staged := a.detail.staged
		if _, ok := find(st.Changes, ch.Path); ok && prefs.OneList {
			staged = false // the one list shows the working tree's side (#37)
		}
		if c, ok := find(a.changes(staged), ch.Path); ok {
			a.showChange(c, staged)
		} else if !a.unsaved() { // edits not saved stay, to save or give up
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

// chosenPath is the path of a list's change chosen, "" for none.
func chosenPath(changes []git.Change, sel int) string {
	if sel < 0 || sel >= len(changes) {
		return ""
	}
	return changes[sel].Path
}

// indexOf is the row of a list's change at path, -1 for none.
func indexOf(changes []git.Change, path string) int {
	for i, c := range changes {
		if path != "" && c.Path == path {
			return i
		}
	}
	return -1
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
				a.errMsg = loc.ErrReadHistory(err.Error())
			}
		})
	}()
}

// showCommit loads the files a commit changed.
func (a *app) showCommit(i int) {
	a.showCommitOf(a.commits[i])
}

// showCommitOf loads the files a commit changed, one the history lists or not.
func (a *app) showCommitOf(c git.Commit) {
	if a.leaving(func() { a.showCommitOf(c) }) {
		return
	}
	old := textSide{}
	if len(c.Parents) > 0 {
		old = revSide(c.Parents[0])
	}
	a.showPatch(c, old, revSide(c.Hash), func(ctx context.Context, ws git.Whitespace) (string, error) { return a.repo.Show(ctx, c.Hash, ws) })
}

// showCompare shows the change from one revision to another, the working
// tree when to is empty, as a commit's.
func (a *app) showCompare(label, from, to string) {
	if a.leaving(func() { a.showCompare(label, from, to) }) {
		return
	}
	c := git.Commit{Hash: from + ".." + to, Subject: label}
	now := diskSide
	if to != "" {
		now = revSide(to)
	}
	a.showPatch(c, revSide(from), now, func(ctx context.Context, ws git.Whitespace) (string, error) {
		if to == "" {
			return a.repo.DiffWorktree(ctx, from, ws)
		}
		return a.repo.DiffFrom(ctx, from, to, ws)
	})
}

// comparing tells whether a commit shown is a comparison of two.
func comparing(c *git.Commit) bool { return strings.Contains(c.Hash, "..") }

// showStash shows a stash's files as a commit's.
func (a *app) showStash(s git.Stash) {
	if a.leaving(func() { a.showStash(s) }) {
		return
	}
	c := git.Commit{Hash: s.Hash, Time: s.Time, Subject: s.Subject, Refs: []string{s.Ref}}
	a.showPatch(c, revSide(s.Ref+"^1"), revSide(s.Ref), func(ctx context.Context, ws git.Whitespace) (string, error) { return a.repo.StashShow(ctx, s.Ref, ws) })
}

// patchLoad reads a commit's patch, leaving out the whitespace changes ws
// says.
type patchLoad func(ctx context.Context, ws git.Whitespace) (string, error)

// showPatch loads the files of a commit's patch, which load reads, and
// its message. old and now say where the texts of its diffs' sides come
// from, for the files it shows whole.
func (a *app) showPatch(c git.Commit, old, now textSide, load patchLoad) {
	if a.detail.commit != nil && a.detail.commit.Hash == c.Hash && a.detail.blame == nil {
		return
	}
	a.detail = detail{commit: &c, gen: a.detail.gen + 1, loading: true, sides: [2]textSide{old, now}}
	a.loadPatch(load, "")
}

// showParent shows a merge's change against another of its parents.
func (a *app) showParent(i int) {
	c := *a.detail.commit
	parent := c.Parents[i]
	a.detail = detail{commit: &c, parent: i, message: a.detail.message, gen: a.detail.gen + 1, loading: true,
		sides: [2]textSide{revSide(parent), revSide(c.Hash)}}
	a.loadPatch(func(ctx context.Context, ws git.Whitespace) (string, error) {
		return a.repo.DiffFrom(ctx, parent, c.Hash, ws)
	}, "")
}

// loadPatch loads the commit's patch with load, showing the file named
// keep when it has it.
func (a *app) loadPatch(load patchLoad, keep string) {
	gen, hash, ws := a.detail.gen, a.detail.commit.Hash, prefs.whitespace()
	old, now := a.detail.sides[0], a.detail.sides[1]
	a.detail.load = load
	type result struct {
		files   []editor.FilePatch
		message string
		marked  map[string]bool
		texts   map[string][2]string
	}
	work(a, func(ctx context.Context) (result, error) {
		out, err := load(ctx, ws)
		if err != nil {
			return result{}, err
		}
		files, err := editor.ParsePatch(out)
		msg := ""
		if !strings.Contains(hash, "..") {
			msg = a.repo.Message(ctx, hash)
		}
		names := make([]string, len(files))
		for i, f := range files {
			names[i] = fileName(f)
		}
		marked := a.repo.Generated(ctx, names)
		// The files' texts here, with the patch: choosing one shows it at
		// once, rather than waiting for git to start again.
		texts := map[string][2]string{}
		if !ws.Any() {
			texts = a.prefetch(ctx, files, old, now)
		}
		return result{files, msg, marked, texts}, err
	}, func(r result, err error) {
		if gen != a.detail.gen {
			return
		}
		a.detail.loading = false
		if err != nil {
			a.errMsg = err.Error()
			return
		}
		a.detail.files, a.detail.message, a.detail.texts = r.files, r.message, r.texts
		a.detail.generated = make([]bool, len(r.files))
		for i := range r.files {
			a.detail.generated[i] = isGenerated(fileName(r.files[i]), r.marked, &r.files[i], nil)
		}
		a.detail.fileSel = -1
		if prefs.CommitAll && len(r.files) > 0 { // one after another (#44)
			a.showCommitAll()
			return
		}
		kept := slices.IndexFunc(r.files, func(f editor.FilePatch) bool { return fileName(f) == keep })
		// The first file people wrote, past lockfiles and the like.
		if kept >= 0 {
			a.showFile(kept)
		} else if first := slices.Index(a.detail.generated, false); first >= 0 {
			a.showFile(first)
		} else if len(r.files) > 0 {
			a.showFile(0)
		}
	})
}

// showFile shows the diff of a file of the commit shown.
func (a *app) showFile(i int) {
	d := &a.detail
	d.fileSel = i
	d.folded = d.generated[i] && !a.unfolded[foldKey(d.commit.Hash, fileName(d.files[i]))]
	d.diff, d.images, d.starts, d.patchWhole = nil, nil, nil, false
	f := d.files[i]
	if isImage(fileName(f)) {
		d.folded, d.enter = false, 0 // no changes to step into
		a.loadImages(a.commitSides(f.OldName, f.NewName))
	} else if !d.folded {
		a.showWhole(f, i)
	} else {
		d.enter = 0 // folded, it has no diff to open at a change
	}
}

// showWhole shows the diff of a file of the commit shown: its patch over
// the whole files when they can be read, as IntelliJ shows them, else its
// hunks alone, as git prints them. The texts read with the patch show it
// at once; others are read here, on a worker.
func (a *app) showWhole(f editor.FilePatch, i int) {
	name := fileName(f)
	if texts, ok := a.detail.texts[name]; ok {
		if d := fullDiff(f, texts[0], texts[1]); d != nil {
			a.detail.diff, a.detail.patchWhole = d, true
			a.foldWhole(d)
			a.openAtChange(d)
			return
		}
	}
	// A file added, deleted or renamed, or too long, shows by its hunks
	// alone: at once, with no worker's round trip leaving the diff empty
	// for a frame (#55).
	if !patch.Partial(f) || patchTooLong(f) {
		a.detail.loading = false // a file chosen before may be loading still
		a.showHunks(i)
		return
	}
	gen, sel, old, now := a.detail.gen, i, a.detail.sides[0], a.detail.sides[1]
	a.detail.loading = true
	type result struct {
		diff  *editor.DiffState
		texts [2]string
	}
	work(a, func(ctx context.Context) (result, error) {
		d, texts, _ := a.patchFull(ctx, f, old, now)
		return result{d, texts}, nil
	}, func(r result, err error) {
		if gen != a.detail.gen || sel != a.detail.fileSel {
			return
		}
		a.detail.loading = false
		if r.diff != nil {
			if a.detail.texts == nil {
				a.detail.texts = map[string][2]string{}
			}
			a.detail.texts[name] = r.texts
			a.detail.diff, a.detail.patchWhole = r.diff, true
			a.foldWhole(r.diff)
			a.openAtChange(r.diff)
			return
		}
		if sel < len(a.detail.files) {
			a.showHunks(sel)
		}
	})
}

// showHunks shows the diff of a file of the commit shown by its hunks
// alone, as git prints them.
func (a *app) showHunks(i int) {
	f := a.detail.files[i]
	d := patchDiff(f)
	a.detail.diff, a.detail.starts = d, patchStarts(f)
	a.openAtChange(d)
}

// openAtChange puts the caret at the change the file shown opens at, when
// stepped into going on or back: its first, or its last. A file chosen
// alone keeps its caret where it was, its first change scrolled into view
// instead, as no step of the changes is under way.
func (a *app) openAtChange(d *editor.DiffState) {
	if a.detail.enter != 0 {
		if n := len(d.Hunks()); n > 0 {
			d.GoToHunk(enteredHunk(n, a.detail.enter))
		}
		a.detail.enter = 0
		return
	}
	a.detail.reveal = true
}

// revealChange scrolls the first change of the diff shown into view, once
// the frame has laid its lines out, as a file shows whole from its top.
func (a *app) revealChange() {
	d := &a.detail
	if !d.reveal || d.diff == nil {
		return
	}
	hunks := d.diff.Hunks()
	if len(hunks) == 0 {
		d.reveal = false
		return
	}
	if d.diff.ViewportHeight() <= 0 { // not laid out yet
		return
	}
	d.reveal = false
	top := d.diff.LineTop(editor.DiffNew, hunks[0].NewStart)
	lh := d.diff.LineTop(editor.DiffNew, hunks[0].NewStart+1) - top
	if top+lh <= d.diff.ViewportHeight() {
		return // already in view
	}
	d.diff.SetScrollY(max(top-2*lh, 0)) // a couple of lines below the top
}

// foldWhole folds the unchanged lines of whole files away from their
// changes, as the last choice of the toggle keeps (#47) or as a file long
// enough to read a screen at a time is opened.
func (a *app) foldWhole(d *editor.DiffState) {
	d.ExpandUnchanged = !prefs.FoldUnchanged && max(d.Old.LineCount(), d.New.LineCount()) <= wholeLines
	a.detail.fold = !d.ExpandUnchanged
}

// unfold shows the diff of the generated file shown.
func (a *app) unfold() {
	d := &a.detail
	switch {
	case d.commit != nil && d.fileSel >= 0:
		a.unfolded[foldKey(d.commit.Hash, fileName(d.files[d.fileSel]))] = true
		a.showFile(d.fileSel)
	case d.change != nil:
		a.unfolded[foldKey("", d.change.Path)] = true
		ch, staged := *d.change, d.staged
		a.detail = detail{gen: d.gen}
		a.showChange(ch, staged)
	}
}

// showChange shows the diff of a change, staged or in the working tree.
// A file changed in the working tree shows whole, the index's version on
// the left and the file on the right to edit. Shown again as the change
// moves on, the diff stays, with its selection, scroll and edits, while
// the texts are the same.
func (a *app) showChange(c git.Change, staged bool) {
	if c.Conflict {
		if a.detail.merge == nil || a.detail.merge.path != c.Path {
			a.showConflict(c)
		}
		return
	}
	same := a.detail.change != nil && a.detail.change.Path == c.Path && a.detail.staged == staged
	if isImage(c.Path) { // the picture's sides, read again as it changes
		if !same || a.detail.images == nil {
			a.detail = detail{change: &c, staged: staged, gen: a.detail.gen + 1}
		}
		a.detail.change = &c
		a.loadImages(changeSides(c, staged))
		return
	}
	if !same {
		a.detail = detail{change: &c, staged: staged, gen: a.detail.gen + 1, loading: true}
	}
	gen := a.detail.gen
	ws := prefs.whitespace()
	byName := isGenerated(c.Path, a.genMarked, nil, nil)
	unfolded := a.unfolded[foldKey("", c.Path)]
	// The working tree's file, whole and editable, when it is changed in
	// place and its patch, all of it, can be cut for staging. The one list
	// shows every file whole against HEAD, as IDEA's commit window (#37).
	whole := !staged && !ws.Any() && (prefs.OneList || c.Work == 'M') && !c.Submodule // a submodule's folder, no file (#54)
	old, now := changeTexts(staged)
	type result struct {
		raw      string
		patch    *editor.FilePatch
		old, new string // the whole texts
		diff     *editor.DiffState
		whole    bool      // the diff shows the whole files, a patch's changes on them
		texts    [2]string // the texts the whole files were made of
		starts   []int     // the lines its hunks start at, for the minimap's bubble
		folded   bool
		binary   bool
	}
	work(a, func(ctx context.Context) (result, error) {
		if c.Work == '?' && !staged { // untracked: all of it is new
			text, err := a.repo.ReadFile(ctx, c.Path)
			if err != nil {
				return result{}, err
			}
			if isBinary(text) { // shown as git shows it: no lines
				return result{raw: text, binary: true}, nil
			}
			if !unfolded && (byName || isGenerated(c.Path, nil, nil, firstLines(text, 5))) {
				return result{raw: text, folded: true}, nil
			}
			return result{raw: text, diff: textDiff(c.Path, "", text)}, nil
		}
		// The texts of its sides read beside its patch, rather than after
		// it: one call of git each, and the window waits for the longer.
		read := make(chan [2]string, 1)
		readSides := func() {
			oldText, newText, err := a.sideTexts(ctx, old, now, c.Path, c.Path)
			if err != nil {
				read <- [2]string{}
				return
			}
			read <- [2]string{oldText, newText}
		}
		reading := !whole
		if reading {
			go readSides()
		}
		var out string
		var err error
		if prefs.OneList { // the change against HEAD, staged or not
			out, err = a.repo.DiffHead(ctx, c.Path, ws)
		} else {
			out, err = a.repo.Diff(ctx, c.Path, staged, ws)
		}
		if err != nil {
			return result{}, err
		}
		files, err := editor.ParsePatch(out)
		if err != nil || len(files) == 0 {
			return result{raw: out}, err
		}
		whole = whole && files[0].NewName != "" // a file gone has nothing to read
		r := result{raw: out, patch: &files[0]}
		if files[0].Binary { // no lines to show, and none to edit
			r.binary = true
			return r, nil
		}
		if !unfolded && (byName || isGenerated(c.Path, nil, &files[0], nil)) {
			r.folded = true
			return r, nil
		}
		if whole {
			if prefs.OneList {
				oldPath := c.Path
				if c.OldPath != "" {
					oldPath = c.OldPath
				}
				r.old = a.repo.ReadHead(ctx, oldPath)
			} else {
				r.old = a.repo.ReadIndex(ctx, c.Path)
			}
			if r.new, err = a.repo.ReadFile(ctx, c.Path); err != nil {
				return result{}, err
			}
			r.diff = textDiff(c.Path, r.old, r.new)
			r.diff.Editable = true
			// The whole file, as it is edited, unless folded as asked: folding
			// the lines around the changes moves them as a change is typed away.
			r.diff.ExpandUnchanged = !prefs.FoldUnchanged
		} else if !patch.Partial(files[0]) || patchTooLong(files[0]) {
			r.diff = patchDiff(files[0])
			r.starts = patchStarts(files[0])
		} else {
			if !reading { // shown whole until its patch said otherwise
				readSides()
			}
			texts := <-read
			var d *editor.DiffState
			if len(texts[0])+len(texts[1]) <= wholeMaxByte {
				d = fullDiff(files[0], texts[0], texts[1])
			}
			if d == nil {
				d = patchDiff(files[0])
				r.starts = patchStarts(files[0])
			} else {
				r.whole, r.texts = true, texts
			}
			r.diff = d
		}
		return r, nil
	}, func(r result, err error) {
		if gen != a.detail.gen {
			return
		}
		a.detail.loading = false
		if err != nil {
			a.errMsg = err.Error()
		}
		d := a.detail.diff
		if same && d != nil {
			if r.raw == a.detail.raw {
				return
			}
			// The file changed as shown, or its index did: keep the diff
			// and what's being edited, taking the new patch for staging.
			if a.detail.whole && whole && (d.New.Modified() || d.New.String() == r.new) {
				if d.Old.String() != r.old {
					d.Old.Set(r.old)
				}
				if !d.New.Modified() {
					a.detail.disk = r.new
				}
				a.detail.raw, a.detail.patch = r.raw, r.patch
				return
			}
		}
		a.detail.raw, a.detail.patch, a.detail.diff, a.detail.whole = r.raw, r.patch, r.diff, whole && r.diff != nil
		a.detail.patchWhole = r.whole && r.diff != nil
		if a.detail.patchWhole {
			a.foldWhole(r.diff)
			a.detail.texts = map[string][2]string{c.Path: r.texts}
		}
		a.detail.starts = r.starts
		a.detail.folded, a.detail.binary, a.detail.disk = r.folded, r.binary, r.new
		a.detail.change = &c
		// A generated file folded has a patch but no diff yet (#35). The one
		// list does not stage lines: its check is the commit's (#37).
		if r.diff != nil && r.patch != nil && patch.Partial(*r.patch) && !ws.Any() && !prefs.OneList {
			a.reviewLines(r.diff)
		}
		// The file opens at its first change, or its last when ⇧F7 came
		// back to it.
		if r.diff != nil && (a.detail.whole || a.detail.enter != 0) {
			r.diff.GoToHunk(enteredHunk(len(r.diff.Hunks()), a.detail.enter))
		}
		a.detail.enter = 0
	})
}

// saveFile writes the working tree's file edited in the diff.
func (a *app) saveFile() { a.save(false) }

// editing tells whether the file at path is the one edited and not saved.
func (a *app) editing(path string) bool {
	return a.unsaved() && a.detail.change.Path == path
}

// save writes the file edited; changed on disk since its edits began, as
// by another editor or git, it asks before writing over that, unless over.
func (a *app) save(over bool) {
	d := a.detail
	if !d.whole || d.diff == nil || !d.diff.New.Modified() {
		return
	}
	text, path := d.diff.New.String(), d.change.Path
	// Not for long on the main thread, though a computer reached over SSH
	// is slow to answer.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if now, err := a.repo.ReadFile(ctx, path); !over && err == nil && now != d.disk && now != text {
		a.ask(loc.SaveConflictTitle(path), loc.SaveConflictMessage(), loc.SaveOver(), func() { a.save(true) })
		return
	}
	if err := a.repo.WriteFile(ctx, path, text); err != nil {
		a.errMsg = loc.ErrSave(err.Error())
		return
	}
	a.detail.disk = text
	d.diff.New.MarkSaved()
	a.refresh()
}

// wholeLines is the most lines of a side of a diff shown whole at once;
// longer files fold their unchanged lines away from the changes as they
// open, and patchFull leaves the longest ones to their hunks alone.
const (
	wholeLines   = 2000
	wholeMaxLine = 50000
	wholeMaxByte = 8 << 20
	// wholeMaxFiles and wholeMaxCache are the most files of a commit whose
	// texts are kept, and the most bytes of them: a commit of thousands of
	// files keeps none, reading the chosen one alone.
	wholeMaxFiles = 400
	wholeMaxCache = 24 << 20
)

// sideKind says where the text of a side of a diff comes from.
type sideKind int

const (
	sideNone  sideKind = iota // no text: a file added or deleted
	sideRev                   // a revision's blob, its rev names it
	sideIndex                 // the index's version of the file
	sideDisk                  // the file on disk
)

// side is where the text of a side of a diff comes from.
type textSide struct {
	kind sideKind
	rev  string
}

func revSide(rev string) textSide { return textSide{kind: sideRev, rev: rev} }

var (
	indexSide = textSide{kind: sideIndex}
	diskSide  = textSide{kind: sideDisk}
)

// changeTexts is where the sides of a change's diff come from: the index
// and the working tree, HEAD and the index when staged, or HEAD and the
// working tree in the one list (#37).
func changeTexts(staged bool) (old, now textSide) {
	switch {
	case prefs.OneList: // against HEAD, staged or not
		return revSide("HEAD"), diskSide
	case staged:
		return revSide("HEAD"), indexSide
	}
	return indexSide, diskSide
}

// patchFull shows a patch over the whole files of its sides, as IntelliJ
// shows a change, and whether it could: a patch whose sides are too long,
// whose files are not changed in place, or which does not make the new one
// from the old, as one leaving whitespace out may not, is shown by its
// hunks alone.
func (a *app) patchFull(ctx context.Context, f editor.FilePatch, old, now textSide) (*editor.DiffState, [2]string, bool) {
	var texts [2]string
	if !patch.Partial(f) || patchTooLong(f) {
		return nil, texts, false
	}
	oldText, newText, err := a.sideTexts(ctx, old, now, f.OldName, f.NewName)
	if err != nil || len(oldText)+len(newText) > wholeMaxByte {
		return nil, texts, false
	}
	texts[0], texts[1] = oldText, newText
	d := fullDiff(f, oldText, newText)
	return d, texts, d != nil // the patch may not make the new file from the old
}

// fullDiff shows a patch over the whole files it makes the new one from,
// nil when it does not, as one leaving whitespace out may not.
func fullDiff(f editor.FilePatch, oldText, newText string) *editor.DiffState {
	d := &editor.DiffState{Layout: diffLayout(), Wrap: prefs.Wrap, KeepScrollbars: true}
	if err := d.SetPatchFiles(f, oldText, newText); err != nil {
		return nil
	}
	return d
}

// prefetch reads the texts of the files of a commit's patch, in one call of
// git, so that choosing a file shows it at once: a file shown by its hunks
// alone, or one past the room kept for them, keeps none.
func (a *app) prefetch(ctx context.Context, files []editor.FilePatch, old, now textSide) map[string][2]string {
	texts := make(map[string][2]string, len(files))
	var specs []string
	var at []int // the file of each spec, and its side
	for i, f := range files {
		if !patch.Partial(f) || patchTooLong(f) {
			continue
		}
		for side, s := range [2]textSide{old, now} {
			switch s.kind {
			case sideRev:
				specs = append(specs, s.rev+":"+f.NewName)
			case sideIndex:
				specs = append(specs, ":"+f.NewName)
			default:
				continue
			}
			at = append(at, i*2+side)
		}
	}
	if len(specs) == 0 {
		return texts
	}
	if len(files) > wholeMaxFiles {
		return texts // too many to keep the texts of, they read one at a time
	}
	keep, kept := specs, make([]int, len(specs))
	for j := range specs {
		kept[j] = j
	}
	// A commit of many files says their sizes before they are read, so that
	// a file of megabytes is not pulled in with the rest; a few files read
	// what they are, the room kept for them capping what is used.
	if len(specs) > 20 {
		sizes, err := a.repo.BlobSizes(ctx, specs)
		if err != nil {
			return texts
		}
		keep, kept = nil, nil
		size := int64(0)
		for j, n := range sizes {
			if n < 0 || size+n > wholeMaxCache {
				continue
			}
			size += n
			keep = append(keep, specs[j])
			kept = append(kept, j)
		}
		if len(keep) == 0 {
			return texts
		}
	}
	blobs, err := a.repo.Blobs(ctx, keep)
	if err != nil || len(blobs) != len(keep) {
		return texts // read one at a time, as before
	}
	for j, b := range blobs {
		if b == nil {
			continue
		}
		k := at[kept[j]]
		name := files[k/2].NewName
		t := texts[name]
		t[k%2] = string(b)
		texts[name] = t
	}
	return texts
}

// patchTooLong tells whether a patch's hunks say their files are too long
// to show whole, read or not.
func patchTooLong(f editor.FilePatch) bool {
	for _, h := range f.Hunks {
		if h.OldStart+h.OldLines > wholeMaxLine || h.NewStart+h.NewLines > wholeMaxLine {
			return true
		}
	}
	return false
}

// sideTexts is the texts of the two sides of a diff: a side with no file
// has none. Those git holds are read in one call of it, as reading the two
// of a file one call each waits twice as long for git to start.
func (a *app) sideTexts(ctx context.Context, old, now textSide, oldPath, newPath string) (string, string, error) {
	var specs []string
	at := [2]int{-1, -1}
	add := func(i int, s textSide, path string) {
		if path == "" {
			return
		}
		switch s.kind {
		case sideRev:
			at[i] = len(specs)
			specs = append(specs, s.rev+":"+path)
		case sideIndex:
			at[i] = len(specs)
			specs = append(specs, ":"+path)
		}
	}
	add(0, old, oldPath)
	add(1, now, newPath)
	texts := [2]string{}
	if len(specs) > 0 {
		blobs, err := a.repo.Blobs(ctx, specs)
		if err != nil {
			return "", "", err
		}
		for i, j := range at {
			if j < 0 {
				continue
			}
			if j >= len(blobs) || blobs[j] == nil {
				return "", "", errors.New("no such file in that revision")
			}
			texts[i] = string(blobs[j])
		}
	}
	for i, s := range [2]textSide{old, now} {
		if s.kind != sideDisk {
			continue
		}
		path := oldPath
		if i == 1 {
			path = newPath
		}
		if path == "" {
			continue
		}
		text, err := a.repo.ReadFile(ctx, path)
		if err != nil {
			return "", "", err
		}
		texts[i] = text
	}
	return texts[0], texts[1], nil
}

func patchDiff(p editor.FilePatch) *editor.DiffState {
	// The patch sets the language its lines are in, and the starts of its
	// hunks, which its highlighter restarts at: the file's lines between two
	// hunks are not in the text, so one hunk ending in a block comment would
	// leave the next all comment.
	d := &editor.DiffState{Layout: diffLayout(), Wrap: prefs.Wrap, KeepScrollbars: true}
	d.SetPatch(p)
	return d
}

// patchStarts are the lines of a patch's new side that its hunks start at: a
// patch's lines are its hunks' lines one after another, the file's lines
// between them not there, so a hunk ending in a block comment would leave the
// next all comment.
func patchStarts(p editor.FilePatch) []int {
	starts := make([]int, 0, len(p.Hunks))
	at := 0
	for _, h := range p.Hunks {
		starts = append(starts, at)
		for _, l := range h.Lines {
			if l == "" || l[0] == '\\' {
				continue
			}
			if l[0] != '-' {
				at++
			}
		}
	}
	return starts
}

func textDiff(name, old, new string) *editor.DiffState {
	d := &editor.DiffState{Old: editor.NewBuffer(old), New: editor.NewBuffer(new), Layout: diffLayout(), Wrap: prefs.Wrap, KeepScrollbars: true}
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
	if a.editing(c.Path) && a.savingFirst(func() { a.stage(c) }) {
		return
	}
	a.run(loc.OpStage(), func(ctx context.Context) error { return a.repo.Stage(ctx, c.Path) }, nil)
}

func (a *app) unstage(c git.Change) {
	paths := []string{c.Path}
	if c.OldPath != "" {
		paths = append(paths, c.OldPath) // a rename is staged as both
	}
	a.run(loc.OpUnstage(), func(ctx context.Context) error { return a.repo.Unstage(ctx, paths...) }, nil)
}

// trash moves a file to the trash.
var trash = func(path string) error { return mygo.Shell.TrashItem(path) }

// onDisk tells whether a change's file is in the working tree, to trash.
func onDisk(c git.Change) bool {
	return !c.Conflict && !c.Submodule && c.Work != 'D' && (c.Work != 0 || c.Staged != 'D')
}

// trashFile moves a file of the working tree to the trash, once the user
// says so, or over SSH, with no trash to reach, deletes it (#52); git then
// lists a tracked one as deleted, to discard if it was a mistake.
func (a *app) trashFile(path string) {
	if !a.local() {
		a.ask(loc.DeleteTitle(), loc.DeleteMessage(path, a.repo.Host()), loc.DeleteButton(), func() {
			a.run(loc.OpDiscard(), func(ctx context.Context) error { return a.repo.Remove(ctx, path) }, nil)
		})
		return
	}
	a.ask(loc.TrashTitle(runtime.GOOS), loc.TrashMessage(path, runtime.GOOS), loc.TrashButton(runtime.GOOS), func() { // Windows's Recycle Bin
		a.run(loc.OpDiscard(), func(context.Context) error { return trash(a.repo.Path(path)) }, nil)
	})
}

// discard throws a change of the working tree away, once the user says so.
func (a *app) discard(c git.Change) {
	if c.Work == '?' { // a file git doesn't have: to the trash, where it can be found again
		a.trashFile(c.Path)
		return
	}
	if prefs.OneList && c.OldPath != "" { // a rename, put back as HEAD has it
		a.ask(loc.DiscardTitle(), loc.DiscardMessage(c.Path), loc.Discard(), func() {
			a.run(loc.OpDiscard(), func(ctx context.Context) error { return a.discardRename(ctx, c) }, nil)
		})
		return
	}
	if prefs.OneList && c.Staged == 'A' { // added, not in HEAD: unstage it, then trash it
		a.ask(loc.TrashTitle(runtime.GOOS), loc.TrashMessage(c.Path, runtime.GOOS), loc.TrashButton(runtime.GOOS), func() {
			a.run(loc.OpDiscard(), func(ctx context.Context) error {
				if err := a.repo.Unstage(ctx, c.Path); err != nil {
					return err
				}
				if !a.local() {
					return a.repo.Remove(ctx, c.Path)
				}
				return trash(a.repo.Path(c.Path))
			}, nil)
		})
		return
	}
	a.ask(loc.DiscardTitle(), loc.DiscardMessage(c.Path), loc.Discard(), func() {
		a.run(loc.OpDiscard(), func(ctx context.Context) error {
			if prefs.OneList { // the one list's changes are against HEAD
				return a.repo.RestoreHead(ctx, c.Path)
			}
			return a.repo.Discard(ctx, c)
		}, nil)
	})
}

// discardAll throws away the whole list of changes, or with a filter the
// changes it shows, once the user says so; a file in conflict stays for
// its sides to choose.
func (a *app) discardAll() {
	changes := a.changes(false)
	rows := a.shownRows(false)
	var chosen []git.Change
	for i, c := range changes {
		if !c.Conflict && (rows == nil || slices.Contains(rows, i)) {
			chosen = append(chosen, c)
		}
	}
	if len(chosen) == 0 {
		return
	}
	a.ask(loc.DiscardAllTitle(), loc.DiscardAllMessage(len(chosen)), loc.Discard(), func() {
		a.run(loc.OpDiscard(), func(ctx context.Context) error { return a.discardChanges(ctx, chosen) }, nil)
	})
}

// discardChanges throws the changes away: tracked files go back to the
// index, untracked ones to the trash, where there is one.
func (a *app) discardChanges(ctx context.Context, changes []git.Change) error {
	var tracked, untracked, added []string
	var renames []git.Change
	for _, c := range changes {
		switch {
		case c.Work == '?':
			untracked = append(untracked, c.Path)
		case prefs.OneList && c.OldPath != "": // both sides are HEAD's to restore
			renames = append(renames, c)
		case prefs.OneList && c.Staged == 'A': // added, not in HEAD: unstaged, then trashed
			added = append(added, c.Path)
			untracked = append(untracked, c.Path)
		default:
			tracked = append(tracked, c.Path)
		}
	}
	restore := a.repo.Restore
	if prefs.OneList { // the one list's changes are against HEAD
		restore = a.repo.RestoreHead
	}
	if err := restore(ctx, tracked...); err != nil {
		return err
	}
	if err := a.repo.Unstage(ctx, added...); err != nil {
		return err
	}
	if !a.local() { // no trash to reach over SSH: deleted
		if err := a.repo.Remove(ctx, untracked...); err != nil {
			return err
		}
	} else {
		for _, p := range untracked {
			if err := trash(a.repo.Path(p)); err != nil {
				return err
			}
		}
	}
	for _, c := range renames {
		if err := a.discardRename(ctx, c); err != nil {
			return err
		}
	}
	return nil
}

// folderItems are the indices, of a list's items, of a folder's changes.
func folderItems(changes []git.Change, items []int, dir string) []int {
	prefix := dir + "/"
	var out []int
	for _, i := range items {
		if strings.HasPrefix(changes[i].Path, prefix) {
			out = append(out, i)
		}
	}
	return out
}

// folderChanges are the changes of a folder in a list of them, those it
// can discard: a file in conflict stays for its sides to choose.
func folderChanges(changes []git.Change, items []int, dir string) []git.Change {
	var out []git.Change
	for _, i := range folderItems(changes, items, dir) {
		// The one list's changes are all against HEAD, so all can go back.
		if c := changes[i]; !c.Conflict && (prefs.OneList || c.Work != 0) {
			out = append(out, c)
		}
	}
	return out
}

// folderPaths are the paths of a folder's changes, for staging them.
func folderPaths(changes []git.Change, items []int, dir string) []string {
	var out []string
	for _, i := range folderItems(changes, items, dir) {
		out = append(out, changes[i].Path)
	}
	return out
}

// discardFolder throws away a folder's changes, once the user says so.
func (a *app) discardFolder(dir string, changes []git.Change) {
	if len(changes) == 0 {
		return
	}
	a.ask(loc.DiscardFolderTitle(), loc.DiscardFolderMessage(len(changes), path.Base(dir)), loc.Discard(), func() {
		a.run(loc.OpDiscard(), func(ctx context.Context) error { return a.discardChanges(ctx, changes) }, nil)
	})
}

// stageAll and unstageAll act on a whole list, or with a filter on the
// changes it shows.
func (a *app) stageAll() { a.stagePaths(a.shownPaths(false)...) }

func (a *app) unstageAll() { a.unstagePaths(a.shownPaths(true)...) }

// stagePaths and unstagePaths stage or unstage the changes of a folder,
// or of a whole list, as the buttons of their rows and headers do.
func (a *app) stagePaths(paths ...string) {
	if len(paths) == 0 {
		return
	}
	if a.savingFirst(func() { a.stagePaths(paths...) }) {
		return
	}
	a.run(loc.OpStage(), func(ctx context.Context) error { return a.repo.Stage(ctx, paths...) }, nil)
}

func (a *app) unstagePaths(paths ...string) {
	if len(paths) == 0 {
		return
	}
	a.run(loc.OpUnstage(), func(ctx context.Context) error { return a.repo.Unstage(ctx, paths...) }, nil)
}

func (a *app) canCommit() bool {
	return a.busy == "" && a.message != "" && (a.commitCount() > 0 || a.amend)
}

func (a *app) commit() {
	if a.canCommit() && a.savingFirst(a.commit) {
		return
	}
	if !a.canCommit() {
		return
	}
	msg, amend, chosen := a.message, a.amend, a.chosenChanges()
	a.run(loc.OpCommit(), func(ctx context.Context) error { return a.commitChanges(ctx, msg, amend, chosen) }, func() {
		a.message, a.amend, a.recall = "", false, 0
	})
}

// recallKey brings back the recent messages with Up, as a shell does its
// commands, and goes forward again with Down: from an empty box, or one
// still showing a message brought back, so the arrows move the caret
// through what is typed.
func (a *app) recallKey(k ui.TextKey) bool {
	if k.Mods != 0 || k.Key != ui.KeyUp && k.Key != ui.KeyDown {
		return false
	}
	text, step := k.Text(), 1
	if k.Key == ui.KeyDown {
		step = -1
	}
	if a.recall == 0 {
		if text != "" || step < 0 {
			return false
		}
		a.recalled = a.repo.RecentMessages(context.Background(), 50)
	} else if text != a.recalled[a.recall-1] {
		a.recall = 0
		return false
	}
	if i := a.recall + step; i <= len(a.recalled) {
		a.recall, a.message = i, ""
		if i > 0 {
			a.message = a.recalled[i-1]
		}
		k.SetText(a.message)
		a.recallEnd = true
	}
	return true
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

// remoteNames are the remotes the remotes' branches name.
func (a *app) remoteNames() []string {
	var names []string
	for _, b := range a.remotes {
		if name, _, ok := strings.Cut(b, "/"); ok && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}

// reloadDiff shows the diff of what's chosen again, as the diff settings
// changed.
func (a *app) reloadDiff() {
	switch d := a.detail; {
	case d.all != nil: // read again, as what it leaves out may have changed
		a.detail.all = nil
		a.showAll(d.all.staged)
	case d.change != nil && d.merge == nil:
		a.detail = detail{gen: d.gen}
		a.showChange(*d.change, d.staged)
	case d.commit != nil && d.load != nil && d.blame == nil:
		// Read again, as what it leaves out may have changed, the file
		// shown staying while it loads.
		keep := ""
		if d.fileSel >= 0 && d.fileSel < len(d.files) {
			keep = fileName(d.files[d.fileSel])
		}
		a.detail.gen++
		a.detail.loading = true
		a.loadPatch(d.load, keep)
	}
}
