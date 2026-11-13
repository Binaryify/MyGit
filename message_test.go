package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"mygit/internal/llm"
	"mygit/internal/sysproxy"
	loc "mygit/locales"
)

// fakeKey stands for the credential store, keeping the key in memory.
func fakeKey(t *testing.T) *string {
	key := ""
	get, set := aiKey, saveAIKey
	aiKey = func() (string, error) { return key, nil }
	saveAIKey = func(k string) error { key = k; return nil }
	t.Cleanup(func() { aiKey, saveAIKey = get, set })
	return &key
}

// TestGenerateMessage checks that the model set writes the message from
// the staged diff, with the recent messages to follow, and that its
// failures say why (#25).
func TestGenerateMessage(t *testing.T) {
	defer func(s aiSettings) { prefs.AI = s }(prefs.AI)
	*fakeKey(t) = "sk-test"
	status, reply := 200, `{"choices":[{"message":{"content":"Add the notes"}}]}`
	var asked, system, auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		asked, system, auth = body.Messages[len(body.Messages)-1].Content, body.Messages[0].Content, r.Header.Get("Authorization")
		w.WriteHeader(status)
		io.WriteString(w, reply)
	}))
	defer srv.Close()

	prefs.AI = aiSettings{}
	tt := newTester(t)
	if tt.HasText(loc.GenerateMessage()) {
		t.Fatal("a Generate button with no model set")
	}
	// The settings show the AI's line alone; its own dialog the rest.
	tt.a.settings.open = true
	tt.Frame()
	if !tt.HasText(loc.AiStatusOff()) || tt.HasText(loc.AiKeyNotice()) || tt.HasText(loc.BaseURL()) {
		t.Fatalf("settings: %q", tt.Texts())
	}
	tt.click(t, loc.AiSetUp())
	tt.Frame()
	if !tt.a.settings.ai.open || !tt.HasText(loc.AiKeyNotice()) || !tt.HasText(loc.BaseURL()) {
		t.Fatalf("AI dialog: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEscape) // closes the AI's dialog, back to the settings
	tt.Frame()
	if tt.a.settings.ai.open || !tt.a.settings.open {
		t.Fatalf("Escape: AI %v, settings %v", tt.a.settings.ai.open, tt.a.settings.open)
	}
	tt.a.settings.open = false
	tt.Frame()
	prefs.AI = aiSettings{API: llm.OpenAI, BaseURL: srv.URL, KeySaved: true} // no model: not set up
	tt.Frame()
	if tt.HasText(loc.GenerateMessage()) {
		t.Fatal("a Generate button with no model set")
	}
	prefs.AI.Model, prefs.AI.Instructions = "m", "Use Conventional Commits."
	tt.Frame()
	if !tt.HasText(loc.GenerateMessage()) || tt.a.canGenerateMessage() {
		t.Fatal("no button, or generating with nothing staged")
	}
	tt.a.stagePaths("notes.txt")
	tt.settle()
	tt.click(t, loc.GenerateMessage())
	if tt.a.message != "Add the notes" || tt.a.errMsg != "" {
		t.Fatalf("message %q, error %q", tt.a.message, tt.a.errMsg)
	}
	if !strings.HasSuffix(system, "\nUse Conventional Commits.") {
		t.Fatalf("system %q", system)
	}
	if auth != "Bearer sk-test" || !strings.Contains(asked, "+remember") || !strings.Contains(asked, "Merge the feature") {
		t.Fatalf("auth %q, asked %q", auth, asked)
	}

	status, reply = 401, `{"error":{"message":"Incorrect API key provided"}}`
	tt.click(t, loc.GenerateMessage())
	if !strings.Contains(tt.a.errMsg, "Incorrect API key") || tt.a.message != "Add the notes" {
		t.Fatalf("failed: message %q, error %q", tt.a.message, tt.a.errMsg)
	}
}

func TestMessagePrompt(t *testing.T) {
	diff := strings.Repeat("+a line of the change\n", maxPromptDiff/10)
	p := messagePrompt([]string{"Fix the log\n"}, " a.go | 2 +-\n", diff)
	if !strings.Contains(p, "---\nFix the log\n---") || !strings.Contains(p, "a.go | 2 +-") ||
		!strings.HasSuffix(p, "+a line of the change\n[the rest of the diff is left out]\n") || len(p) > maxPromptDiff+200 {
		t.Fatalf("prompt of %d bytes: %q…", len(p), p[:200])
	}
	if p := messagePrompt(nil, "", "+x\n"); strings.Contains(p, "Recent") {
		t.Fatalf("recent messages of none: %q", p)
	}
}

// TestSaveKey checks that the key typed goes to the credential store as
// the settings close, and stays out of the settings' file.
func TestSaveKey(t *testing.T) {
	defer func(s aiSettings) { prefs.AI = s }(prefs.AI)
	key := fakeKey(t)
	prefs.AI = aiSettings{Model: "m"}
	d := &aiDialog{key: " sk-new "}
	if !d.saveKey() || *key != "sk-new" || d.key != "" || !prefs.AI.KeySaved {
		t.Fatalf("saved %q, typed %q, %+v", *key, d.key, prefs.AI)
	}
	data, _ := json.Marshal(prefs)
	if strings.Contains(string(data), "sk-new") {
		t.Fatal("the key in the settings' file")
	}
	saveAIKey = func(string) error { return errors.New("locked") }
	d.key = "sk-other"
	if d.saveKey() || !strings.Contains(d.keyErr, "locked") || d.key != "sk-other" {
		t.Fatalf("failing: %+v", d)
	}
}

// TestMessageSystem checks that the user's instructions come after the
// rules, which keep the reply the message alone, and that none add nothing.
func TestMessageSystem(t *testing.T) {
	plain := messageSystem("  ")
	if strings.Contains(plain, "instructions") {
		t.Fatalf("instructions of none: %q", plain)
	}
	with := messageSystem(" 用中文写\n")
	if !strings.HasPrefix(with, plain) || !strings.HasSuffix(with, "reply with the message alone:\n用中文写") {
		t.Fatalf("with instructions: %q", with)
	}
}

// TestAIProxy checks that the model is reached through the proxy set, as
// the downloads, and past it when Direct.
func TestAIProxy(t *testing.T) {
	defer func(s aiSettings) { prefs.AI = s }(prefs.AI)
	*fakeKey(t) = "k"
	hits := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		io.WriteString(w, `{"choices":[{"message":{"content":"Via the proxy"}}]}`)
	}))
	defer proxy.Close()
	sysproxy.Install()
	if err := sysproxy.Set(proxy.URL); err != nil {
		t.Fatal(err)
	}
	defer sysproxy.Set("")
	// A host only the proxy knows: .test is never resolved.
	prefs.AI = aiSettings{BaseURL: "http://models.example.test/v1", Model: "m"}
	tt := newTester(t)
	tt.a.stagePaths("notes.txt")
	tt.settle()
	tt.a.generateMessage()
	tt.settle()
	if tt.a.message != "Via the proxy" || hits != 1 {
		t.Fatalf("through the proxy: message %q, error %q, hits %d", tt.a.message, tt.a.errMsg, hits)
	}
	prefs.AI.Direct = true
	tt.a.generateMessage()
	tt.settle()
	if hits != 1 || tt.a.errMsg == "" {
		t.Fatalf("direct: error %q, hits %d", tt.a.errMsg, hits)
	}
}
