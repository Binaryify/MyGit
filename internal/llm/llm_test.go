package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mygit/internal/sysproxy"
)

// serve answers one request with reply, keeping what was asked.
func serve(t *testing.T, status int, reply string) (*httptest.Server, *http.Request, map[string]any) {
	t.Helper()
	var got http.Request
	body := map[string]any{}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = *r
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		w.WriteHeader(status)
		io.WriteString(w, reply)
	}))
	t.Cleanup(s.Close)
	return s, &got, body
}

func TestOpenAI(t *testing.T) {
	s, req, body := serve(t, 200, `{"choices":[{"message":{"content":"<think>hmm</think>\n`+"```\\nAdd the notes\\n```"+`"}}]}`)
	got, err := Complete(context.Background(), Config{API: OpenAI, BaseURL: s.URL + "/v1/", Model: "m", Key: "k"}, "sys", "diff")
	if err != nil || got != "Add the notes" {
		t.Fatalf("got %q, %v", got, err)
	}
	if req.URL.Path != "/v1/chat/completions" || req.Header.Get("Authorization") != "Bearer k" || body["model"] != "m" {
		t.Fatalf("asked %s %q %v", req.URL.Path, req.Header.Get("Authorization"), body)
	}
	msgs := body["messages"].([]any)
	if len(msgs) != 2 || msgs[0].(map[string]any)["role"] != "system" || msgs[1].(map[string]any)["content"] != "diff" {
		t.Fatalf("messages %v", msgs)
	}
}

func TestAnthropic(t *testing.T) {
	s, req, body := serve(t, 200, `{"content":[{"type":"thinking","thinking":"hmm"},{"type":"text","text":"Fix the log\n\nIt crashed."}]}`)
	got, err := Complete(context.Background(), Config{API: Anthropic, BaseURL: s.URL, Model: "m", Key: "k"}, "sys", "diff")
	if err != nil || got != "Fix the log\n\nIt crashed." {
		t.Fatalf("got %q, %v", got, err)
	}
	if req.URL.Path != "/v1/messages" || req.Header.Get("x-api-key") != "k" || req.Header.Get("Authorization") != "Bearer k" || req.Header.Get("anthropic-version") == "" {
		t.Fatalf("asked %s %v", req.URL.Path, req.Header)
	}
	if body["system"] != "sys" || body["max_tokens"] == nil {
		t.Fatalf("body %v", body)
	}
}

func TestErrors(t *testing.T) {
	s, _, _ := serve(t, 401, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
	_, err := Complete(context.Background(), Config{API: Anthropic, BaseURL: s.URL, Model: "m"}, "", "")
	if err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "invalid x-api-key") {
		t.Fatalf("error %v", err)
	}
	s, _, _ = serve(t, 502, `<html>Bad Gateway</html>`)
	if _, err := Complete(context.Background(), Config{BaseURL: s.URL, Model: "m"}, "", ""); err == nil || !strings.Contains(err.Error(), "Bad Gateway") {
		t.Fatalf("error %v", err)
	}
	s, req, _ := serve(t, 200, `{"choices":[]}`)
	if _, err := Complete(context.Background(), Config{BaseURL: s.URL, Model: "m"}, "", ""); err == nil {
		t.Fatal("no choices, no error")
	}
	if req.Header.Get("Authorization") != "" {
		t.Fatal("a key sent with none set")
	}
}

// TestProxy checks that the requests go through the proxy the app is set
// to, as its downloads do, but those to a model on this computer straight.
func TestProxy(t *testing.T) {
	var through []string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		through = append(through, r.URL.Host)
		io.WriteString(w, `{"choices":[{"message":{"content":"via proxy"}}]}`)
	}))
	defer proxy.Close()
	local, _, _ := serve(t, 200, `{"choices":[{"message":{"content":"direct"}}]}`)
	sysproxy.Install()
	if err := sysproxy.Set(proxy.URL); err != nil {
		t.Fatal(err)
	}
	defer sysproxy.Set("")
	got, err := Complete(context.Background(), Config{BaseURL: "http://models.example.test/v1", Model: "m"}, "", "")
	if err != nil || got != "via proxy" || len(through) != 1 || through[0] != "models.example.test" {
		t.Fatalf("remote: %q, %v, through %q", got, err, through)
	}
	got, err = Complete(context.Background(), Config{BaseURL: local.URL, Model: "m"}, "", "")
	if err != nil || got != "direct" || len(through) != 1 {
		t.Fatalf("local: %q, %v, through %q", got, err, through)
	}
}
