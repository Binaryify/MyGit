package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mygit/internal/github"
)

// gitIn runs git in dir as Ada, and returns what it prints.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Ada", "GIT_AUTHOR_EMAIL=a@example.com",
		"GIT_COMMITTER_NAME=Ada", "GIT_COMMITTER_EMAIL=a@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// fakeGitHub is GitHub's API for pull request 7 of o/r, whose head is
// head, with a thread on thing.go's second line; it keeps what is posted.
type fakeGitHub struct {
	head   string
	mu     sync.Mutex
	posted map[string]map[string]any
}

func (f *fakeGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	if r.Method == "POST" && r.URL.Path != "/graphql" {
		var v map[string]any
		json.Unmarshal(body, &v)
		f.mu.Lock()
		f.posted[r.URL.Path] = v
		f.mu.Unlock()
	}
	switch r.URL.Path {
	case "/repos/o/r/pulls/7":
		fmt.Fprintf(w, `{"number":7,"title":"Add the thing","state":"open","user":{"login":"bo"},
		  "head":{"ref":"thing","label":"bo:thing","sha":%q},"base":{"ref":"main","label":"o:main"},
		  "html_url":"https://github.com/o/r/pull/7"}`, f.head)
	case "/repos/o/r/pulls":
		io.WriteString(w, `[{"number":7,"title":"Add the thing","user":{"login":"bo"},"head":{"label":"bo:thing"},"base":{"ref":"main"}}]`)
	case "/repos/o/r/pulls/7/comments":
		if r.Method == "GET" {
			io.WriteString(w, `[{"id":11,"path":"thing.go","line":2,"side":"RIGHT","body":"Name it better?","user":{"login":"cy"}}]`)
		} else {
			io.WriteString(w, `{"id":12}`)
		}
	case "/graphql":
		io.WriteString(w, `{"data":{"repository":{"pullRequest":{"reviewThreads":{"pageInfo":{"hasNextPage":false},
		  "nodes":[{"id":"T11","isResolved":false,"comments":{"nodes":[{"databaseId":11}]}}]}}}}}`)
	case "/repos/o/r/pulls/7/reviews", "/repos/o/r/pulls/7/comments/11/replies":
		io.WriteString(w, `{"id":13}`)
	default:
		w.WriteHeader(404)
		io.WriteString(w, `{"message":"Not Found"}`)
	}
}

// pullTester is a tester whose origin is a bare repository holding pull
// request 7 as GitHub does, and whose GitHub is fake.
func pullTester(t *testing.T) (*tester, *fakeGitHub) {
	tt := newTester(t)
	dir := tt.a.repo.Dir
	bare := filepath.Join(t.TempDir(), "r.git")
	gitIn(t, dir, "init", "-q", "--bare", bare)
	gitIn(t, dir, "remote", "add", "origin", bare)
	gitIn(t, dir, "push", "-q", "origin", "main")
	gitIn(t, dir, "switch", "-q", "-c", "thing")
	os.WriteFile(filepath.Join(dir, "thing.go"), []byte("package main\n\nfunc thing() {}\n"), 0o644)
	gitIn(t, dir, "add", "thing.go")
	gitIn(t, dir, "commit", "-q", "-m", "Add the thing")
	head := gitIn(t, dir, "rev-parse", "HEAD")
	gitIn(t, dir, "push", "-q", "origin", "HEAD:refs/pull/7/head")
	gitIn(t, dir, "switch", "-q", "main")
	gitIn(t, dir, "branch", "-q", "-D", "thing")

	f := &fakeGitHub{head: head, posted: map[string]map[string]any{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	oldRemote, oldNew := remoteRepo, newGitHub
	remoteRepo = func(url string) (string, string, bool) { return "o", "r", url == bare }
	newGitHub = func(ctx context.Context, owner, repo string) *github.Client {
		return &github.Client{Owner: owner, Repo: repo, Token: "tok", BaseURL: srv.URL}
	}
	t.Cleanup(func() { remoteRepo, newGitHub = oldRemote, oldNew })
	tt.a.refresh()
	tt.settle()
	return tt, f
}

func TestPullRequestReview(t *testing.T) {
	tt, f := pullTester(t)
	a := tt.a

	a.openPulls()
	tt.settle()
	if !tt.HasText("Add the thing") || !tt.HasText("o/r") {
		t.Fatalf("pull requests: %q", tt.Texts())
	}
	tt.click(t, "Add the thing")
	tt.settle()
	pr := a.detail.pr
	if pr == nil || pr.head != f.head || len(a.detail.files) != 1 || fileName(a.detail.files[0]) != "thing.go" {
		t.Fatalf("pull %+v, files %v, error %q", pr, a.detail.files, a.errMsg)
	}
	if !tt.HasText("#7") || !tt.HasText("bo · bo:thing → main") {
		t.Fatalf("header: %q", tt.Texts())
	}
	// The thread under its line, and a reply to it.
	tt.Frame()
	d := a.detail.diff
	if len(pr.threads) != 1 || len(d.Annotations) != 1 || !tt.HasText("Name it better?") {
		t.Fatalf("threads %+v, annotations %+v, texts %q", pr.threads, d.Annotations, tt.Texts())
	}
	if line, _ := a.toDiff(editor.DiffNew, 2); d.Annotations[0].Line != line {
		t.Errorf("thread on line %d of the diff, want %d", d.Annotations[0].Line, line)
	}
	tt.click(t, "回复")
	tt.Type("Done")
	tt.Key(ui.Cmd, ui.KeyEnter)
	tt.settle()
	if got := f.posted["/repos/o/r/pulls/7/comments/11/replies"]; got["body"] != "Done" {
		t.Errorf("reply posted %v", got)
	}

	// A comment into the review, then the review, approving.
	line, _ := a.toDiff(editor.DiffNew, 3)
	d.OnGutterButton(editor.DiffLineRange{Side: editor.DiffNew, Start: line, End: line})
	tt.Frame()
	tt.Type("Export it")
	tt.click(t, "加入审阅")
	if len(pr.pending) != 1 || len(a.comments) != 0 || !tt.HasText("待提交") || !tt.HasText("提交审阅（1）") {
		t.Fatalf("pending %+v, texts %q", pr.pending, tt.Texts())
	}
	tt.click(t, "提交审阅（1）")
	a.submit.event = github.ReviewApprove
	a.submit.body = "Ship it"
	tt.Frame()
	tt.click(t, "提交审阅")
	review := f.posted["/repos/o/r/pulls/7/reviews"]
	comments, _ := review["comments"].([]any)
	if review["event"] != "APPROVE" || review["commit_id"] != f.head || review["body"] != "Ship it" || len(comments) != 1 {
		t.Fatalf("review posted %v (error %q)", review, a.errMsg)
	}
	if cm := comments[0].(map[string]any); cm["path"] != "thing.go" || cm["line"] != float64(3) || cm["side"] != "RIGHT" {
		t.Errorf("review comment %v", cm)
	}
	if len(pr.pending) != 0 || !strings.Contains(a.notice, "#7") {
		t.Errorf("after the review: pending %v, notice %q", pr.pending, a.notice)
	}
}

func TestPullNeedsGitHub(t *testing.T) {
	tt := newTester(t)
	tt.a.openPulls()
	tt.settle()
	if !tt.HasText("这个仓库没有 GitHub 远程仓库。") {
		t.Fatalf("texts %q", tt.Texts())
	}
}
