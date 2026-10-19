package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mygit/internal/git"
	"mygit/internal/github"
	loc "mygit/locales"
)

// pullReview is a pull request being reviewed: where its changes are,
// its review threads, and the comments of the review not yet submitted,
// which stay as the window shows other things.
type pullReview struct {
	client     *github.Client
	remote     string // the remote its commits are fetched from
	pull       github.Pull
	base, head string // the commit its changes are against, and its head
	threads    []github.Thread
	pending    []comment
	stale      bool      // commits were pushed to it since it was opened
	checked    time.Time // when it was last looked at for new commits
	expanded   map[int64]bool
}

// pullsPlan is the dialog listing the open pull requests.
type pullsPlan struct {
	open, loading bool
	slug          string // owner/repo
	list          []github.Pull
	err           string
	number        string
}

// submitPlan is the dialog submitting a review.
type submitPlan struct {
	open  bool
	body  string
	event string
}

// Where pull requests come from, to swap in tests: the owner and
// repository of a remote's URL, and the API's client for them.
var (
	remoteRepo = github.ParseRemote
	newGitHub  = github.New
)

// gitHub is the API of the repository's GitHub remote, upstream before
// origin as a fork's are, and the remote's name.
func (a *app) gitHub(ctx context.Context) (*github.Client, string, error) {
	remotes, err := a.repo.Remotes(ctx)
	if err != nil {
		return nil, "", err
	}
	slices.SortStableFunc(remotes, func(x, y git.Remote) int { return rank(x.Name) - rank(y.Name) })
	for _, r := range remotes {
		if owner, repo, ok := remoteRepo(r.URL); ok {
			return newGitHub(ctx, owner, repo), r.Name, nil
		}
	}
	return nil, "", errors.New(loc.PullNoGitHub())
}

func rank(remote string) int {
	switch remote {
	case "upstream":
		return 0
	case "origin":
		return 1
	}
	return 2
}

// gitHubError says what went wrong talking to GitHub, and how to sign in
// when that is what it needs.
func gitHubError(err error) string {
	var e *github.Error
	if errors.Is(err, github.ErrNoToken) || errors.As(err, &e) && (e.Status == 401 || e.Status == 403 && e.Message == "") {
		return loc.PullSignIn()
	}
	return err.Error()
}

// openPulls lists the open pull requests in a dialog.
func (a *app) openPulls() {
	a.pulls = pullsPlan{open: true, loading: true}
	type result struct {
		slug string
		list []github.Pull
	}
	work(a, func(ctx context.Context) (result, error) {
		client, _, err := a.gitHub(ctx)
		if err != nil {
			return result{}, err
		}
		list, err := client.Pulls(ctx)
		return result{client.Owner + "/" + client.Repo, list}, err
	}, func(r result, err error) {
		a.pulls.loading, a.pulls.slug, a.pulls.list = false, r.slug, r.list
		if err != nil {
			a.pulls.err = gitHubError(err)
		}
	})
}

func (a *app) pullsView(c *ui.Context) {
	p := &a.pulls
	t := c.Theme()
	ui.Modal(c, &p.open, func() {
		ui.Column(c).Width(600).Gap(10).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, loc.PullRequests()).Bold().FontSize(14)
				ui.Text(c, p.slug).TextColor(t.TextMuted)
				ui.Box(c).Grow(1)
				in := ui.TextInput(c, &p.number).Placeholder(loc.PullNumber()).Label(loc.PullNumber()).Width(120)
				n, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(p.number), "#"))
				ok := err == nil && n > 0
				if (ui.Button(c, loc.PullOpen()).Disabled(!ok).Clicked() || in.Submitted()) && ok {
					p.open = false
					a.openPull(n)
				}
			})
			switch {
			case p.loading:
				ui.Row(c).Padding(20).Justify(ui.Center).Children(func() { ui.Spinner(c).Size(18, 18) })
			case p.err != "":
				ui.Text(c, p.err).TextColor(t.Danger).Wrap()
			case len(p.list) == 0:
				ui.Text(c, loc.PullNone()).TextColor(t.TextMuted).Padding(12)
			default:
				ui.Scroll(c).MaxHeight(420).Gap(2).Children(func() {
					for _, pr := range p.list {
						row := ui.ButtonBase(c.Key(pr.Number)).Padding(8, 10).Radius(6).Cursor(ui.CursorPointer)
						if row.Hovered() {
							row.Background(t.Text.Alpha(0.08))
						}
						row.Children(func() {
							ui.Column(c).Gap(2).Grow(1).Children(func() {
								ui.Row(c).Gap(8).Children(func() {
									ui.Text(c, fmt.Sprintf("#%d", pr.Number)).Font("monospace").TextColor(t.Accent).Shrink(0)
									ui.Text(c, pr.Title).Bold().SingleLine()
								})
								meta := fmt.Sprintf("%s · %s → %s", pr.User.Login, pr.Head.Label, pr.Base.Ref)
								if pr.Draft {
									meta += " · " + loc.PullDraft()
								}
								ui.Text(c, meta).FontSize(11).TextColor(t.TextMuted).SingleLine()
							})
						})
						if row.Clicked() {
							p.open = false
							a.openPull(pr.Number)
						}
					}
				})
			}
		})
	})
}

// openPull fetches a pull request's commits and shows its changes, as a
// comparison of the commit it branched off with its head, keeping a
// review begun before.
func (a *app) openPull(n int) {
	if a.leaving(func() { a.openPull(n) }) || a.busy != "" {
		return
	}
	a.busy, a.errMsg = loc.OpOpenPull(n), ""
	type result struct {
		client     *github.Client
		remote     string
		pull       github.Pull
		base, head string
	}
	work(a, func(ctx context.Context) (result, error) {
		client, remote, err := a.gitHub(ctx)
		if err != nil {
			return result{}, err
		}
		pull, err := client.Pull(ctx, n)
		if err != nil {
			return result{}, errors.New(gitHubError(err))
		}
		base, head, err := a.repo.FetchPull(ctx, remote, n, pull.Base.Ref)
		return result{client, remote, pull, base, head}, err
	}, func(r result, err error) {
		a.busy = ""
		if err != nil {
			a.errMsg = loc.OpFailed(loc.OpOpenPull(n), err.Error())
			return
		}
		pr := a.pullReviews[n]
		if pr == nil {
			pr = &pullReview{expanded: map[int64]bool{}}
			a.pullReviews[n] = pr
		}
		pr.client, pr.remote, pr.pull, pr.base, pr.head = r.client, r.remote, r.pull, r.base, r.head
		pr.stale, pr.checked = false, time.Now()
		a.showPull(pr)
	})
}

// showPull shows a pull request's changes and reads its threads.
func (a *app) showPull(pr *pullReview) {
	c := git.Commit{Hash: pr.base + ".." + pr.head, Subject: pr.pull.Title}
	base, head := pr.base, pr.head
	a.detail.commit = nil // shown again, though the same commits
	a.showPatch(c, func(ctx context.Context) (string, error) {
		return a.repo.DiffFrom(ctx, base, head, prefs.IgnoreWhitespace)
	})
	a.detail.pr = pr
	a.loadThreads(pr)
}

func (a *app) loadThreads(pr *pullReview) {
	n := pr.pull.Number
	work(a, func(ctx context.Context) ([]github.Thread, error) { return pr.client.Threads(ctx, n) }, func(th []github.Thread, err error) {
		if err != nil {
			a.errMsg = gitHubError(err)
			return
		}
		pr.threads = th
	})
}

// checkPull looks, at most every half a minute, for commits pushed to the
// pull request shown since it was opened.
func (a *app) checkPull() {
	pr := a.detail.pr
	if pr == nil || pr.stale || time.Since(pr.checked) < 30*time.Second {
		return
	}
	pr.checked = time.Now()
	n := pr.pull.Number
	work(a, func(ctx context.Context) (github.Pull, error) { return pr.client.Pull(ctx, n) }, func(p github.Pull, err error) {
		if err == nil && p.Head.SHA != "" && p.Head.SHA != pr.head {
			pr.stale = true
		}
	})
}

// pullHeader is the pull request's, in place of a commit's: its number,
// title and branches, what's new, and the review.
func (a *app) pullHeader(c *ui.Context, pr *pullReview) {
	t := c.Theme()
	p := pr.pull
	ui.Column(c).Padding(8, 12).Gap(4).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, fmt.Sprintf("#%d", p.Number)).Font("monospace").TextColor(t.Accent).Shrink(0)
			ui.Text(c, p.Title).Bold().SingleLine().Grow(1).Basis(0)
			if p.HTMLURL != "" && ui.Button(c, loc.PullBrowser()).Clicked() {
				mygo.Shell.OpenExternal(p.HTMLURL)
			}
			label := loc.SubmitReview()
			if n := len(pr.pending); n > 0 {
				label = loc.SubmitReviewN(n)
			}
			if ui.PrimaryButton(c, label).Disabled(a.busy != "").Clicked() {
				a.submit = submitPlan{open: true, event: github.ReviewComment}
			}
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			state := loc.PullStateOpen()
			switch {
			case p.Merged:
				state = loc.PullStateMerged()
			case p.State == "closed":
				state = loc.PullStateClosed()
			case p.Draft:
				state = loc.PullDraft()
			}
			ui.Badge(c, state)
			ui.Text(c, fmt.Sprintf("%s · %s → %s", p.User.Login, p.Head.Label, p.Base.Ref)).TextColor(t.TextMuted).SingleLine()
			ui.Text(c, loc.FilesDiffer(len(a.detail.files))).TextColor(t.TextMuted).Shrink(0)
			if n := outdated(pr.threads); n > 0 {
				ui.Text(c, loc.PullOutdated(n)).TextColor(t.TextMuted).Shrink(0).Tooltip(loc.PullOutdatedHint())
			}
		})
		if pr.stale {
			ui.Row(c).Gap(8).Padding(4, 8).Radius(6).AlignItems(ui.Center).Background(t.Accent.Alpha(0.12)).Children(func() {
				ui.Text(c, loc.PullNewCommits()).Grow(1)
				if ui.Button(c, loc.PullReload()).Clicked() {
					a.openPull(p.Number)
				}
			})
		}
	})
}

// outdated counts the threads on lines the pull request's commits moved
// past, which show on GitHub but not in the diff.
func outdated(threads []github.Thread) int {
	n := 0
	for _, th := range threads {
		if th.Root().Line == nil {
			n++
		}
	}
	return n
}

// ghSide is GitHub's name of a side of a diff, and diffSide the other way.
func ghSide(s editor.DiffSide) string {
	if s == editor.DiffOld {
		return "LEFT"
	}
	return "RIGHT"
}

func diffSide(s string) editor.DiffSide {
	if s == "LEFT" {
		return editor.DiffOld
	}
	return editor.DiffNew
}

// draft is a comment as GitHub takes it in a review.
func draft(cm comment) github.DraftComment {
	d := github.DraftComment{Path: cm.Path, Body: cm.Body, Line: cm.End, Side: ghSide(cm.Side)}
	if cm.Start < cm.End {
		d.StartLine, d.StartSide = cm.Start, d.Side
	}
	return d
}

type (
	threadKey  int // a thread's room under its line, by its place in the threads
	pendingKey int // a comment of the review not yet submitted
)

// pullAnnotations add the pull request's threads and pending comments on
// the file shown under their lines.
func (a *app) pullAnnotations(d *editor.DiffState, pr *pullReview, name string) {
	cols := noteCols(d)
	for i, th := range pr.threads {
		root := th.Root()
		if root.Path != name || root.Line == nil {
			continue
		}
		side := diffSide(root.Side)
		if line, ok := a.toDiff(side, *root.Line); ok {
			d.Annotations = append(d.Annotations, editor.DiffAnnotation{Side: side, Line: line, Height: threadHeight(th, cols, pr.expanded[root.ID]), Key: threadKey(i)})
		}
	}
	for i, cm := range pr.pending {
		if cm.Path != name {
			continue
		}
		if line, ok := a.toDiff(cm.Side, cm.End); ok {
			d.Annotations = append(d.Annotations, editor.DiffAnnotation{Side: cm.Side, Line: line, Height: noteHeight(cm.Body, cols), Key: pendingKey(i)})
		}
	}
}

// threadHeight is about how high a thread shows: folded to a line once
// resolved, unless expanded.
func threadHeight(th github.Thread, cols int, expanded bool) float32 {
	if th.Resolved && !expanded {
		return 44
	}
	h := float32(52) // the padding and the actions
	for _, cm := range th.Comments {
		h += noteHeight(cm.Body, cols) - 30
	}
	return h
}

// threadCard is a review thread under its line: its comments, a reply,
// and resolving it.
func (a *app) threadCard(c *ui.Context, pr *pullReview, i int) {
	t := c.Theme()
	th := pr.threads[i]
	root := th.Root()
	ui.Column(c).Fill().Padding(6, 12).Children(func() {
		card := ui.Column(c).Grow(1).Padding(8, 10).Gap(6).Radius(8).Background(t.Surface).BorderWidth(1).BorderColor(t.Border)
		card.Children(func() {
			if th.Resolved && !pr.expanded[root.ID] {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Badge(c, loc.ThreadResolved())
					ui.Text(c, loc.ThreadSummary(root.User.Login, len(th.Comments))).FontSize(11).TextColor(t.TextMuted).Grow(1).SingleLine()
					if ui.Button(c, loc.ThreadShow()).Clicked() {
						pr.expanded[root.ID] = true
					}
				})
				return
			}
			for _, cm := range th.Comments {
				ui.Column(c).Gap(2).Children(func() {
					ui.Row(c).Gap(6).Children(func() {
						ui.Text(c, cm.User.Login).Bold().FontSize(11)
						if !cm.CreatedAt.IsZero() {
							ui.Text(c, cm.CreatedAt.Local().Format("2006-01-02 15:04")).FontSize(11).TextColor(t.TextMuted)
						}
					})
					ui.Text(c, cm.Body).Wrap()
				})
			}
			ui.Row(c).Gap(6).Children(func() {
				if ui.Button(c, loc.ThreadReply()).Disabled(a.busy != "").Clicked() {
					a.composer = composer{open: true, editing: -1, reply: i + 1, draft: comment{Path: root.Path}}
				}
				if th.ID != "" {
					label := loc.ThreadResolve()
					if th.Resolved {
						label = loc.ThreadUnresolve()
					}
					if ui.Button(c, label).Disabled(a.busy != "").Clicked() {
						a.resolveThread(pr, th)
					}
				}
				if th.Resolved && ui.Button(c, loc.ThreadHide()).Clicked() {
					pr.expanded[root.ID] = false
				}
			})
		})
	})
}

// pendingCard is a comment of the review not yet submitted.
func (a *app) pendingCard(c *ui.Context, pr *pullReview, i int) {
	t := c.Theme()
	cm := pr.pending[i]
	ui.Column(c).Fill().Padding(6, 12).Children(func() {
		ui.Column(c).Grow(1).Padding(8, 10).Gap(4).Radius(8).Background(t.Surface).BorderWidth(1).BorderColor(t.Warning.Alpha(0.6)).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Badge(c, loc.CommentPending())
				ui.Text(c, loc.CommentLines(cm.lines())).FontSize(11).TextColor(t.TextMuted)
				ui.Box(c).Grow(1)
				if iconButton(c.Key(i), "edit", loc.Edit()) {
					a.composer = composer{open: true, draft: cm, editing: i}
				}
				if iconButton(c.Key(i), "x", loc.Delete()) {
					pr.pending = append(pr.pending[:i:i], pr.pending[i+1:]...)
				}
			})
			ui.Text(c, cm.Body).Wrap()
		})
	})
}

// sendPull posts what the composer wrote to the pull request: a reply to a
// thread, or a comment alone, then reads the threads again.
func (a *app) sendPull(pr *pullReview, label string, post func(ctx context.Context) error) {
	a.composer.open = false
	a.busy, a.errMsg = label, ""
	work(a, func(ctx context.Context) (struct{}, error) { return struct{}{}, post(ctx) }, func(_ struct{}, err error) {
		a.busy = ""
		if err != nil {
			a.errMsg = loc.OpFailed(label, gitHubError(err))
			return
		}
		a.loadThreads(pr)
	})
}

func (a *app) resolveThread(pr *pullReview, th github.Thread) {
	a.sendPull(pr, loc.ThreadResolve(), func(ctx context.Context) error { return pr.client.Resolve(ctx, th.ID, !th.Resolved) })
}

func (a *app) submitView(c *ui.Context) {
	p := &a.submit
	pr := a.detail.pr
	if pr == nil {
		p.open = false
		return
	}
	ui.Modal(c, &p.open, func() {
		ui.Column(c).Width(480).Gap(12).Children(func() {
			ui.Text(c, loc.SubmitReviewTitle(pr.pull.Number)).Bold().FontSize(14)
			if n := len(pr.pending); n > 0 {
				ui.Text(c, loc.SubmitPending(n)).TextColor(c.Theme().TextMuted)
			}
			ui.TextArea(c, &p.body).Placeholder(loc.SubmitBody()).Label(loc.SubmitBody()).Lines(3, 10).AutoFocus()
			ui.RadioGroup(c, func() {
				ui.Radio(c, &p.event, github.ReviewComment, loc.ReviewComment())
				ui.Radio(c, &p.event, github.ReviewApprove, loc.ReviewApprove())
				ui.Radio(c, &p.event, github.ReviewRequestChanges, loc.ReviewRequestChanges())
			})
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, loc.Cancel()).Clicked() {
					p.open = false
				}
				// A comment or a request for changes says something.
				ok := p.event == github.ReviewApprove || strings.TrimSpace(p.body) != "" || len(pr.pending) > 0
				if ui.PrimaryButton(c, loc.SubmitReview()).Disabled(!ok || a.busy != "").Clicked() {
					a.submitReview(pr)
				}
			})
		})
	})
}

func (a *app) submitReview(pr *pullReview) {
	p := a.submit
	a.submit.open = false
	drafts := make([]github.DraftComment, len(pr.pending))
	for i, cm := range pr.pending {
		drafts[i] = draft(cm)
	}
	n, head, body := pr.pull.Number, pr.head, strings.TrimSpace(p.body)
	label := loc.SubmitReview()
	a.busy, a.errMsg = label, ""
	work(a, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, pr.client.Review(ctx, n, head, body, p.event, drafts)
	}, func(_ struct{}, err error) {
		a.busy = ""
		if err != nil {
			a.errMsg = loc.OpFailed(label, gitHubError(err))
			a.submit.open, a.submit.body = true, p.body // to try again
			return
		}
		pr.pending = nil
		a.notice = loc.ReviewSubmitted(n)
		a.loadThreads(pr)
	})
}
