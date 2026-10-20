package agent

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// connect starts a server and a client talking over pipes, the server's
// output after what a chatty shell prints.
func connect(t *testing.T, ask func(tag, prompt string) (string, bool)) *Client {
	t.Helper()
	toServer, fromClient := io.Pipe()
	toClient, fromServer := io.Pipe()
	go func() {
		_, _ = io.WriteString(fromServer, "Welcome to the box!\n")
		_ = Serve(toServer, fromServer)
		fromServer.Close()
	}()
	c, err := NewClient("box", toClient, fromClient, ask)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fromClient.Close() })
	return c
}

func gitInit(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	return filepath.ToSlash(dir)
}

func TestExec(t *testing.T) {
	c := connect(t, nil)
	dir := gitInit(t)
	ctx := context.Background()
	var out, errOut bytes.Buffer
	if err := c.Exec(ctx, dir, nil, strings.NewReader("hello\n"), &out, &errOut, "hash-object", "--stdin"); err != nil {
		t.Fatal(err, errOut.String())
	}
	if got := strings.TrimSpace(out.String()); got != "ce013625030ba8dba906f756967f9e9ca394464a" {
		t.Errorf("hash = %q", got)
	}
	errOut.Reset()
	if err := c.Exec(ctx, dir, nil, nil, io.Discard, &errOut, "rev-parse", "--verify", "nope"); err == nil || !strings.Contains(errOut.String(), "fatal") {
		t.Errorf("err = %v, stderr %q", err, errOut.String())
	}
	out.Reset()
	if err := c.Exec(ctx, dir, []string{"GIT_AUTHOR_NAME=Ada"}, nil, &out, nil, "var", "GIT_AUTHOR_IDENT"); err != nil || !strings.HasPrefix(out.String(), "Ada ") {
		t.Errorf("env: %v %q", err, out.String())
	}
}

func TestBigOutput(t *testing.T) {
	c := connect(t, nil)
	dir := gitInit(t)
	big := bytes.Repeat([]byte("0123456789abcdef"), 100_000) // more than a frame
	ctx := context.Background()
	if err := c.WriteFile(ctx, dir+"/sub/big.txt", big); err != nil {
		t.Fatal(err)
	}
	got, err := c.ReadFile(ctx, dir+"/sub/big.txt")
	if err != nil || !bytes.Equal(got, big) {
		t.Fatalf("read %d bytes, %v", len(got), err)
	}
}

func TestFiles(t *testing.T) {
	c := connect(t, nil)
	dir := t.TempDir()
	ctx := context.Background()
	p := filepath.ToSlash(filepath.Join(dir, "run.sh"))
	if err := os.WriteFile(p, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := c.WriteFile(ctx, p, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(p); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o755 { // Windows has no modes to keep
		t.Errorf("mode = %v, want it kept", fi.Mode())
	}
	if !c.Exists(ctx, p) || c.Exists(ctx, p+".nope") {
		t.Error("exists is wrong")
	}
	if _, err := c.ReadFile(ctx, p+".nope"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want not exist", err)
	}
	if err := c.RemoveAll(ctx, p); err != nil || c.Exists(ctx, p) {
		t.Errorf("remove: %v", err)
	}
}

func TestCancel(t *testing.T) {
	c := connect(t, nil)
	dir := gitInit(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() {
		// Waits for input that never ends.
		done <- c.Exec(ctx, dir, nil, blockingReader{}, nil, nil, "hash-object", "--stdin")
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceling didn't stop the call")
	}
	// The connection goes on.
	if err := c.Exec(context.Background(), dir, nil, nil, nil, nil, "status"); err != nil {
		t.Error(err)
	}
}

type blockingReader struct{}

func (blockingReader) Read([]byte) (int, error) { select {} }

func TestWatch(t *testing.T) {
	c := connect(t, nil)
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changed := make(chan struct{}, 10)
	go func() { _ = c.Watch(ctx, filepath.ToSlash(dir), func() { changed <- struct{}{} }) }()
	time.Sleep(200 * time.Millisecond)
	if err := os.MkdirAll(filepath.Join(dir, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	<-changed
	time.Sleep(200 * time.Millisecond) // the new folder watched
	if err := os.WriteFile(filepath.Join(dir, "a", "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changed:
	case <-time.After(5 * time.Second):
		t.Fatal("a file changed in a new folder wasn't seen")
	}
}

func TestAsk(t *testing.T) {
	if os.Getenv("MYGIT_ASKPASS") != "" {
		t.Skip()
	}
	exe := buildAgent(t)
	cmd := exec.Command(exe, "serve")
	in, _ := cmd.StdinPipe()
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	asked := ""
	c, err := NewClient("box", out, in, func(tag, prompt string) (string, bool) {
		asked = tag + " " + prompt
		return "s3cret", true
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := gitInit(t)
	// A credential helper asks git's askpass, as a remote's password does.
	var stdout bytes.Buffer
	err = c.Exec(context.Background(), dir, nil, strings.NewReader("protocol=https\nhost=example.com\n\n"), &stdout, nil,
		"-c", "credential.helper=", "credential", "fill")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "password=s3cret") || !strings.HasPrefix(asked, dir+" ") {
		t.Errorf("asked %q, got %q", asked, stdout.String())
	}
}

func buildAgent(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "mygit-agent")
	if out, err := exec.Command("go", "build", "-o", exe, "mygit/cmd/mygit-agent").CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	return exe
}

func TestList(t *testing.T) {
	c := connect(t, nil)
	dir := t.TempDir()
	for _, d := range []string{"b-repo/.git", "A", ".hidden", "c"} {
		os.MkdirAll(filepath.Join(dir, d), 0o755)
	}
	os.WriteFile(filepath.Join(dir, "file.txt"), nil, 0o644)
	want := []Entry{{Name: "A"}, {Name: "b-repo", Repo: true}, {Name: "c"}}
	if os.Symlink(filepath.Join(dir, "c"), filepath.Join(dir, "link")) == nil { // Windows lets only some users make links
		want = append(want, Entry{Name: "link"})
	}
	l, err := c.List(context.Background(), filepath.ToSlash(dir))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(l.Entries, want) || l.Parent == "" {
		t.Errorf("listing %+v", l)
	}
	home, _ := os.Getwd() // the agent's folder, its home over SSH
	if l, err := c.List(context.Background(), ""); err != nil || l.Dir != filepath.ToSlash(home) {
		t.Errorf("home: %+v %v", l, err)
	}
	if _, err := c.List(context.Background(), dir+"/nope"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v", err)
	}
}
