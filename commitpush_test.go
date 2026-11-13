package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	loc "mygit/locales"
)

// TestCommitAndPush checks that the staged changes are committed and the
// branch pushed at once, with the message typed or, the box empty, the
// model's, and that a push failing keeps the commit, saying so (#27).
func TestCommitAndPush(t *testing.T) {
	defer func(s aiSettings) { prefs.AI = s }(prefs.AI)
	prefs.AI = aiSettings{}
	*fakeKey(t) = "k"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"choices":[{"message":{"content":"Write down what to remember"}}]}`)
	}))
	defer srv.Close()

	tt := newTester(t)
	origin := filepath.Join(t.TempDir(), "origin.git")
	gitIn(t, tt.a.repo.Dir, "init", "-q", "--bare", "-b", "main", origin)
	gitIn(t, tt.a.repo.Dir, "remote", "add", "origin", origin)
	pushed := func() string { return gitIn(t, origin, "log", "-1", "--format=%s", "main") }

	// Nothing staged, nothing to write the message with, amending: not.
	tt.a.message = "Add the notes"
	if tt.a.canCommitAndPush() {
		t.Fatal("with nothing staged")
	}
	tt.a.stagePaths("notes.txt")
	tt.settle()
	tt.a.message = ""
	if tt.a.canCommitAndPush() {
		t.Fatal("with no message and no model")
	}
	tt.a.message, tt.a.amend = "Add the notes", true
	if tt.a.canCommitAndPush() {
		t.Fatal("amending")
	}
	tt.a.message, tt.a.amend = "", false

	// The message typed, then the shortcut: the branch's first push sets
	// its upstream.
	tt.click(t, loc.Message())
	tt.Type("Add the notes")
	tt.Key(ui.Cmd|ui.Shift, ui.KeyEnter)
	tt.settle()
	if got := pushed(); got != "Add the notes" || tt.a.message != "" || tt.a.errMsg != "" {
		t.Fatalf("typed: pushed %q, box %q, error %q", got, tt.a.message, tt.a.errMsg)
	}
	if tt.a.status.Upstream != "origin/main" || tt.a.status.Ahead != 0 {
		t.Fatalf("upstream %q, ahead %d", tt.a.status.Upstream, tt.a.status.Ahead)
	}

	// The box empty: the model writes the message.
	prefs.AI = aiSettings{BaseURL: srv.URL, Model: "m"}
	tt.write(t, map[string]string{"notes.txt": "remember\nmore\n"})
	tt.a.stagePaths("notes.txt")
	tt.settle()
	if !tt.a.canCommitAndPush() {
		t.Fatal("not with a model to write the message")
	}
	tt.click(t, loc.MoreCommitActions()) // the arrow of the Commit button
	if err := tt.ChooseMenuItem(loc.CommitAndPush()); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	tt.settle()
	if got := pushed(); got != "Write down what to remember" || tt.a.errMsg != "" {
		t.Fatalf("written: pushed %q, error %q", got, tt.a.errMsg)
	}

	// The push failing: the commit stays, and the error says it is made.
	gitIn(t, tt.a.repo.Dir, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone.git"))
	tt.write(t, map[string]string{"notes.txt": "remember\nmore\nstill\n"})
	tt.a.stagePaths("notes.txt")
	tt.settle()
	tt.a.message = "Remember still"
	tt.a.commitAndPush()
	tt.settle()
	head := gitIn(t, tt.a.repo.Dir, "log", "-1", "--format=%s")
	if head != "Remember still" || tt.a.message != "" || !strings.HasPrefix(tt.a.errMsg, "已经提交，但推送失败：") {
		t.Fatalf("push failing: head %q, box %q, error %q", head, tt.a.message, tt.a.errMsg)
	}
}
