package main

import (
	"context"
	"path"
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
	op                       git.Operation
	stashes                  []git.Stash
	stashList                ui.ListState
	stashSel                 int
	leftTab                  int // the left panel's: changes or the stash
	staged                   []git.Change
	unstaged                 []git.Change
	genMarked                map[string]bool // the changes .gitattributes marks generated
	stagedList, unstagedList ui.ListState
	stagedSel, unstagedSel   int

	message string
	amend   bool

	detail   detail
	fileList ui.ListState // of the commit's files

	unfolded map[string]bool // generated files whose diffs were asked for, by foldKey
	viewed   map[string]bool // files marked viewed, by viewKey

	confirm      confirmation
	branchesOpen bool   // the branches' popover
	branchFilter string // its search
	prompt       prompt
	rebase       rebasePlan
	push         pushPlan
	cred         credPrompt
	remotes2     remotesPlan // the remotes dialog
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
	trustDir string // a repository another user owns, dropped, to trust
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

	files     []editor.FilePatch
	fileSel   int
	generated []bool      // of files, those people don't write
	folded    bool        // the file shown is generated, its diff folded until asked
	binary    bool        // the change shown is of a binary file, without lines
	disk      string      // the file edited as it was on disk when its edits began, to save over it knowingly
	images    *images     // the file shown is a picture: its sides, in place of a diff
	pr        *pullReview // the pull request whose changes the commit is
	diff      *editor.DiffState
	merge     *merger           // a file in conflict's
	blame     *blame            // a file's, shown in place of the diff
	message   string            // the commit's whole message
	parent    int               // the merge's parent its change is against
	raw       string            // the change's patch as git printed it
	whole     bool              // the change shows whole files, the new one to edit
	patch     *editor.FilePatch // the change's, when its lines can be chosen
	gen       int
	loading   bool
}

func newApp(repo *git.Repo) *app {
	return &app{repo: repo, online: make(chan struct{}, 1), logSel: -1, stagedSel: -1, unstagedSel: -1, stashSel: -1,
		sidebarW: prefs.SidebarW, logH: prefs.LogH, filesW: prefs.FilesW,
		unfolded: map[string]bool{}, viewed: map[string]bool{}, pullReviews: map[int]*pullReview{}}
}

func (a *app) start() {
	a.logList.Selected = &a.logSel
	a.logList.Selection = &a.logChosen
	a.logList.Key = func(i int) any { return a.commits[a.commitAt(i)].Hash }
	a.logList.Label = func(i int) string { return a.commits[a.commitAt(i)].Subject }
	a.stagedList.Selected, a.unstagedList.Selected = &a.stagedSel, &a.unstagedSel
	a.stashList.Selected = &a.stashSel
	a.fileList.Selected = &a.detail.fileSel
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
			prefs.SidebarW, prefs.LogH, prefs.FilesW = a.sidebarW, a.logH, a.filesW
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
	marked   map[string]bool // the changes .gitattributes marks generated
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
		stashes, _ := a.repo.Stashes(ctx)
		paths := make([]string, len(st.Changes))
		for i, c := range st.Changes {
			paths[i] = c.Path
		}
		return snapshot{st, b, rb, a.repo.Refs(ctx), a.repo.Operation(ctx), stashes, a.repo.Generated(ctx, paths)}, nil
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
		a.genMarked = s.marked
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
	// The changes chosen stay chosen by their paths, as rows come and go;
	// one gone leaves its list without a choice, so that choosing a row
	// shows it.
	stagedPath, unstagedPath := chosenPath(a.staged, a.stagedSel), chosenPath(a.unstaged, a.unstagedSel)
	a.staged, a.unstaged = a.staged[:0], a.unstaged[:0]
	for _, c := range st.Changes {
		if c.Staged != 0 {
			a.staged = append(a.staged, c)
		}
		if c.Work != 0 {
			a.unstaged = append(a.unstaged, c)
		}
	}
	a.stagedSel, a.unstagedSel = indexOf(a.staged, stagedPath), indexOf(a.unstaged, unstagedPath)
	// The diff of a change shows it as it is now, or nothing once it's gone.
	if ch := a.detail.change; ch != nil {
		if c, ok := find(a.changes(a.detail.staged), ch.Path); ok {
			a.showChange(c, a.detail.staged)
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
	if a.leaving(func() { a.showCommit(i) }) {
		return
	}
	c := a.commits[i]
	a.showPatch(c, func(ctx context.Context) (string, error) { return a.repo.Show(ctx, c.Hash, prefs.IgnoreWhitespace) })
}

// showCompare shows the change from one revision to another, the working
// tree when to is empty, as a commit's.
func (a *app) showCompare(label, from, to string) {
	if a.leaving(func() { a.showCompare(label, from, to) }) {
		return
	}
	c := git.Commit{Hash: from + ".." + to, Subject: label}
	ignore := prefs.IgnoreWhitespace
	a.showPatch(c, func(ctx context.Context) (string, error) {
		if to == "" {
			return a.repo.DiffWorktree(ctx, from, ignore)
		}
		return a.repo.DiffFrom(ctx, from, to, ignore)
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
	a.showPatch(c, func(ctx context.Context) (string, error) { return a.repo.StashShow(ctx, s.Ref) })
}

// showPatch loads the files of a commit's patch, which load reads, and
// its message.
func (a *app) showPatch(c git.Commit, load func(ctx context.Context) (string, error)) {
	if a.detail.commit != nil && a.detail.commit.Hash == c.Hash && a.detail.blame == nil {
		return
	}
	a.detail = detail{commit: &c, gen: a.detail.gen + 1, loading: true}
	a.loadPatch(load)
}

// showParent shows a merge's change against another of its parents.
func (a *app) showParent(i int) {
	c := *a.detail.commit
	parent := c.Parents[i]
	a.detail = detail{commit: &c, parent: i, message: a.detail.message, gen: a.detail.gen + 1, loading: true}
	a.loadPatch(func(ctx context.Context) (string, error) {
		return a.repo.DiffFrom(ctx, parent, c.Hash, prefs.IgnoreWhitespace)
	})
}

func (a *app) loadPatch(load func(ctx context.Context) (string, error)) {
	gen, hash := a.detail.gen, a.detail.commit.Hash
	type result struct {
		files   []editor.FilePatch
		message string
		marked  map[string]bool
	}
	work(a, func(ctx context.Context) (result, error) {
		out, err := load(ctx)
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
		return result{files, msg, a.repo.Generated(ctx, names)}, err
	}, func(r result, err error) {
		if gen != a.detail.gen {
			return
		}
		a.detail.loading = false
		if err != nil {
			a.errMsg = err.Error()
			return
		}
		a.detail.files, a.detail.message = r.files, r.message
		a.detail.generated = make([]bool, len(r.files))
		for i := range r.files {
			a.detail.generated[i] = isGenerated(fileName(r.files[i]), r.marked, &r.files[i], nil)
		}
		a.detail.fileSel = -1
		// The first file people wrote, past lockfiles and the like.
		if first := slices.Index(a.detail.generated, false); first >= 0 {
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
	d.diff, d.images = nil, nil
	if f := d.files[i]; isImage(fileName(f)) {
		d.folded = false
		a.loadImages(a.commitSides(f.OldName, f.NewName))
	} else if !d.folded {
		d.diff = patchDiff(f)
	}
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
	ignoreSpace := prefs.IgnoreWhitespace
	byName := isGenerated(c.Path, a.genMarked, nil, nil)
	unfolded := a.unfolded[foldKey("", c.Path)]
	// The working tree's file, whole and editable, when it is changed in
	// place and its patch, all of it, can be cut for staging.
	whole := !staged && c.Work == 'M' && !ignoreSpace
	type result struct {
		raw      string
		patch    *editor.FilePatch
		old, new string // the whole texts
		diff     *editor.DiffState
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
		out, err := a.repo.Diff(ctx, c.Path, staged, ignoreSpace)
		if err != nil {
			return result{}, err
		}
		files, err := editor.ParsePatch(out)
		if err != nil || len(files) == 0 {
			return result{raw: out}, err
		}
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
			r.old = a.repo.ReadIndex(ctx, c.Path)
			if r.new, err = a.repo.ReadFile(ctx, c.Path); err != nil {
				return result{}, err
			}
			r.diff = textDiff(c.Path, r.old, r.new)
			r.diff.Editable = true
			// The whole file, as it is edited: folding the lines around the
			// changes would move everything once a change is typed away.
			r.diff.ExpandUnchanged = true
		} else {
			r.diff = patchDiff(files[0])
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
		a.detail.folded, a.detail.binary, a.detail.disk = r.folded, r.binary, r.new
		a.detail.change = &c
		if r.patch != nil && patch.Partial(*r.patch) && !ignoreSpace {
			a.reviewLines(r.diff)
		}
		if a.detail.whole && len(r.diff.Hunks()) > 0 {
			r.diff.GoToHunk(0) // the file opens at its first change
		}
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

func patchDiff(p editor.FilePatch) *editor.DiffState {
	d := &editor.DiffState{Layout: diffLayout()}
	d.SetPatch(p)
	if lang := editor.LanguageFor(fileName(p)); lang != nil {
		d.Old.SetLanguage(lang)
		d.New.SetLanguage(lang)
	}
	return d
}

func textDiff(name, old, new string) *editor.DiffState {
	d := &editor.DiffState{Old: editor.NewBuffer(old), New: editor.NewBuffer(new), Layout: diffLayout()}
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

// discard throws a change of the working tree away, once the user says so.
func (a *app) discard(c git.Change) {
	if c.Work == '?' && !a.local() { // no trash to reach over SSH: deleted
		a.ask(loc.DeleteTitle(), loc.DeleteMessage(c.Path, a.repo.Host()), loc.DeleteButton(), func() {
			a.run(loc.OpDiscard(), func(ctx context.Context) error { return a.repo.Remove(ctx, c.Path) }, nil)
		})
		return
	}
	if c.Work == '?' { // a file git doesn't have: to the trash, where it can be found again
		a.ask(loc.TrashTitle(), loc.TrashMessage(c.Path), loc.TrashButton(), func() {
			a.run(loc.OpDiscard(), func(context.Context) error {
				return trash(a.repo.Path(c.Path))
			}, nil)
		})
		return
	}
	a.ask(loc.DiscardTitle(), loc.DiscardMessage(c.Path), loc.Discard(), func() {
		a.run(loc.OpDiscard(), func(ctx context.Context) error { return a.repo.Discard(ctx, c) }, nil)
	})
}

func (a *app) stageAll() {
	if a.savingFirst(a.stageAll) {
		return
	}
	a.run(loc.OpStage(), func(ctx context.Context) error { return a.repo.Stage(ctx, ".") }, nil)
}

func (a *app) unstageAll() {
	a.run(loc.OpUnstage(), func(ctx context.Context) error { return a.repo.Unstage(ctx, ".") }, nil)
}

func (a *app) canCommit() bool {
	return a.busy == "" && a.message != "" && (len(a.staged) > 0 || a.amend)
}

func (a *app) commit() {
	if a.canCommit() && a.savingFirst(a.commit) {
		return
	}
	if !a.canCommit() {
		return
	}
	msg, amend := a.message, a.amend
	a.run(loc.OpCommit(), func(ctx context.Context) error { return a.repo.Commit(ctx, msg, amend) }, func() {
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
	case d.change != nil && d.merge == nil:
		a.detail = detail{gen: d.gen}
		a.showChange(*d.change, d.staged)
	case d.commit != nil && d.fileSel >= 0 && d.fileSel < len(d.files):
		a.showFile(d.fileSel)
	}
}
