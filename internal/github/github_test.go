package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseRemote(t *testing.T) {
	for url, want := range map[string]string{
		"git@github.com:Binaryify/MyGit.git":    "Binaryify/MyGit",
		"https://github.com/egoist/mygo":        "egoist/mygo",
		"https://github.com/egoist/mygo.git/":   "egoist/mygo",
		"ssh://git@github.com/nkzw-tech/codiff": "nkzw-tech/codiff",
		"https://gitlab.com/a/b.git":            "",
		"/Users/ada/repos/bare.git":             "",
	} {
		o, r, ok := ParseRemote(url)
		got := ""
		if ok {
			got = o + "/" + r
		}
		if got != want {
			t.Errorf("ParseRemote(%q) = %q, want %q", url, got, want)
		}
	}
}

// fake is GitHub's API for one repository's pull request 7, keeping what
// is posted.
type fake struct {
	posted map[string]map[string]any
}

func (f *fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	if r.Method == "POST" {
		var v map[string]any
		json.Unmarshal(body, &v)
		f.posted[r.URL.Path] = v
	}
	switch {
	case r.URL.Path == "/repos/o/r/pulls" && r.Method == "GET":
		io.WriteString(w, `[{"number":7,"title":"Add the thing","user":{"login":"ada"},"head":{"ref":"thing","sha":"abc"},"base":{"ref":"main","sha":"def"}}]`)
	case r.URL.Path == "/repos/o/r/pulls/7/comments" && r.Method == "GET":
		io.WriteString(w, `[
		  {"id":1,"path":"a.go","line":3,"side":"RIGHT","body":"Why?","user":{"login":"bo"}},
		  {"id":2,"in_reply_to_id":1,"path":"a.go","line":3,"side":"RIGHT","body":"Because.","user":{"login":"ada"}},
		  {"id":3,"path":"b.go","line":null,"side":"LEFT","body":"Old","user":{"login":"bo"}}]`)
	case r.URL.Path == "/graphql":
		if strings.Contains(string(body), "reviewThreads") {
			io.WriteString(w, `{"data":{"repository":{"pullRequest":{"reviewThreads":{"pageInfo":{"hasNextPage":false},
			  "nodes":[{"id":"T1","isResolved":true,"comments":{"nodes":[{"databaseId":1}]}},
			           {"id":"T3","isResolved":false,"comments":{"nodes":[{"databaseId":3}]}}]}}}}}`)
		} else {
			io.WriteString(w, `{"data":{}}`)
		}
	case r.URL.Path == "/repos/o/r/pulls/7/reviews":
		io.WriteString(w, `{"id":9}`)
	case r.URL.Path == "/repos/o/r/pulls/7/comments/1/replies":
		io.WriteString(w, `{"id":4,"in_reply_to_id":1,"body":"Thanks"}`)
	default:
		w.WriteHeader(404)
		io.WriteString(w, `{"message":"Not Found"}`)
	}
}

func TestPullsAndThreads(t *testing.T) {
	f := &fake{posted: map[string]map[string]any{}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := &Client{Owner: "o", Repo: "r", Token: "tok", BaseURL: srv.URL}
	ctx := context.Background()

	pulls, err := c.Pulls(ctx)
	if err != nil || len(pulls) != 1 || pulls[0].Number != 7 || pulls[0].Head.Ref != "thing" {
		t.Fatalf("pulls %+v, %v", pulls, err)
	}
	threads, err := c.Threads(ctx, 7)
	if err != nil || len(threads) != 2 {
		t.Fatalf("threads %+v, %v", threads, err)
	}
	if th := threads[0]; len(th.Comments) != 2 || th.ID != "T1" || !th.Resolved || *th.Root().Line != 3 {
		t.Errorf("first thread %+v", th)
	}
	if th := threads[1]; th.Root().Line != nil || th.Resolved || th.ID != "T3" {
		t.Errorf("outdated thread %+v", th)
	}

	if _, err := c.Reply(ctx, 7, 1, "Thanks"); err != nil {
		t.Fatal(err)
	}
	err = c.Review(ctx, 7, "abc", "Looks good", ReviewApprove, []DraftComment{{Path: "a.go", Body: "Nice", Line: 5, Side: "RIGHT", StartLine: 4, StartSide: "RIGHT"}})
	if err != nil {
		t.Fatal(err)
	}
	review := f.posted["/repos/o/r/pulls/7/reviews"]
	comments, _ := review["comments"].([]any)
	if review["event"] != "APPROVE" || review["commit_id"] != "abc" || len(comments) != 1 {
		t.Errorf("review posted %v", review)
	}
	if cm := comments[0].(map[string]any); cm["start_line"] != float64(4) || cm["line"] != float64(5) {
		t.Errorf("review comment %v", cm)
	}
}

func TestErrors(t *testing.T) {
	f := &fake{posted: map[string]map[string]any{}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	ctx := context.Background()
	// Without a token: reading works, writing asks for one.
	c := &Client{Owner: "o", Repo: "r", BaseURL: srv.URL}
	threads, err := c.Threads(ctx, 7)
	if err != nil || len(threads) != 2 || threads[0].ID != "" {
		t.Fatalf("threads without a token %+v, %v", threads, err)
	}
	if err := c.Review(ctx, 7, "abc", "", ReviewComment, nil); !errors.Is(err, ErrNoToken) {
		t.Errorf("review without a token: %v", err)
	}
	var e *Error
	if _, err := c.Pull(ctx, 404); !errors.As(err, &e) || e.Status != 404 || e.Message != "Not Found" {
		t.Errorf("missing pull: %v", err)
	}
}
