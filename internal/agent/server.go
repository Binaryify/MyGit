package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"

	"mygit/internal/askpass"
)

// server runs the app's calls on the agent's computer.
type server struct {
	w     *wire
	mu    sync.Mutex
	calls map[uint32]*call
	asks  map[uint32]chan answer
	askID atomic.Uint32
	env   func(dir string) []string // askpass's
}

type call struct {
	cancel context.CancelFunc
	stdin  *queue
}

// Serve runs the calls read from r, writing what they print to w, until r
// ends.
func Serve(r io.Reader, w io.Writer) error {
	if _, err := io.WriteString(w, Magic); err != nil {
		return err
	}
	s := &server{w: &wire{w: w}, calls: map[uint32]*call{}, asks: map[uint32]chan answer{}}
	// Git's prompts, and ssh's under it, go to the app.
	if ap, err := askpass.Listen(s.ask); err == nil {
		defer ap.Close()
		s.env = ap.Env
	}
	defer s.stopAll()
	for {
		kind, id, data, err := readFrame(r)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		switch kind {
		case kStart:
			var req Request
			if err := json.Unmarshal(data, &req); err != nil {
				_ = s.w.sendJSON(kExit, id, Exit{Err: err.Error()})
				continue
			}
			ctx, cancel := context.WithCancel(context.Background())
			c := &call{cancel: cancel, stdin: newQueue()}
			s.mu.Lock()
			s.calls[id] = c
			s.mu.Unlock()
			go s.run(ctx, id, c, req)
		case kStdin:
			if c := s.call(id); c != nil {
				if len(data) == 0 {
					c.stdin.CloseWithError(nil)
				} else {
					_, _ = c.stdin.Write(data)
				}
			}
		case kCancel:
			if c := s.call(id); c != nil {
				c.cancel()
				c.stdin.CloseWithError(context.Canceled) // git may be waiting for it
			}
		case kAnswer:
			var a answer
			_ = json.Unmarshal(data, &a)
			s.mu.Lock()
			ch := s.asks[id]
			delete(s.asks, id)
			s.mu.Unlock()
			if ch != nil {
				ch <- a
			}
		}
	}
}

func (s *server) call(id uint32) *call {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[id]
}

func (s *server) stopAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.calls {
		c.cancel()
		c.stdin.CloseWithError(io.ErrUnexpectedEOF)
	}
	for id, ch := range s.asks {
		ch <- answer{}
		delete(s.asks, id)
	}
}

// ask hands a prompt to the app and waits for its answer.
func (s *server) ask(tag, prompt string) (string, bool) {
	id := s.askID.Add(1)
	ch := make(chan answer, 1)
	s.mu.Lock()
	s.asks[id] = ch
	s.mu.Unlock()
	if err := s.w.sendJSON(kAsk, id, ask{tag, prompt}); err != nil {
		return "", false
	}
	a := <-ch
	return a.Answer, a.OK
}

func (s *server) run(ctx context.Context, id uint32, c *call, req Request) {
	err, no := s.do(ctx, id, c, req)
	s.mu.Lock()
	delete(s.calls, id)
	s.mu.Unlock()
	c.cancel()
	c.stdin.CloseWithError(nil)
	e := Exit{No: no}
	if err != nil {
		e.Err, e.NotExist = err.Error(), errors.Is(err, fs.ErrNotExist)
	}
	_ = s.w.sendJSON(kExit, id, e)
}

func (s *server) do(ctx context.Context, id uint32, c *call, req Request) (err error, no bool) {
	stdout := stream{s.w, kStdout, id}
	path := filepath.FromSlash(req.Path)
	switch req.Op {
	case "exec":
		cmd := exec.CommandContext(ctx, "git", req.Args...)
		cmd.Dir = filepath.FromSlash(req.Dir)
		cmd.Env = append(os.Environ(), req.Env...)
		if s.env != nil {
			cmd.Env = append(cmd.Env, s.env(req.Dir)...)
		}
		if req.Stdin {
			cmd.Stdin = c.stdin
		}
		cmd.Stdout, cmd.Stderr = stdout, stream{s.w, kStderr, id}
		cmd.WaitDelay = 5 * time.Second // a child left holding the output, as ssh may
		return cmd.Run(), false
	case "read":
		f, err := os.Open(path)
		if err != nil {
			return err, false
		}
		defer f.Close()
		_, err = io.Copy(stdout, f)
		return err, false
	case "write":
		data, err := io.ReadAll(c.stdin)
		if err != nil {
			return err, false
		}
		mode := fs.FileMode(0o644)
		if fi, err := os.Stat(path); err == nil {
			mode = fi.Mode()
		} else if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err, false
		}
		return os.WriteFile(path, data, mode), false
	case "remove":
		return os.RemoveAll(path), false
	case "exists":
		_, err := os.Stat(path)
		return nil, err != nil
	case "watch":
		return watch(ctx, filepath.FromSlash(req.Dir), func() { _, _ = stdout.Write([]byte{'\n'}) }), false
	case "list":
		l, err := list(req.Dir)
		if err != nil {
			return err, false
		}
		return json.NewEncoder(stdout).Encode(l), false
	}
	return errors.New("agent: unknown call " + req.Op), false
}

// watch calls changed, a little after files of the tree change, until ctx
// ends. The folders git and tools fill a lot aren't watched.
func watch(ctx context.Context, root string, changed func()) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()
	skip := func(p string) bool {
		base, parent := filepath.Base(p), filepath.Base(filepath.Dir(p))
		return base == "node_modules" || parent == ".git" && (base == "objects" || base == "logs")
	}
	add := func(dir string) {
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			if skip(p) {
				return filepath.SkipDir
			}
			if w.Add(p) != nil {
				return filepath.SkipAll // past the system's limit: the window refreshes as it's focused
			}
			return nil
		})
	}
	add(root)
	var timer <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return nil
		case e, ok := <-w.Events:
			if !ok {
				return nil
			}
			if e.Has(fsnotify.Create) {
				if fi, err := os.Stat(e.Name); err == nil && fi.IsDir() && !skip(e.Name) {
					add(e.Name)
				}
			}
			if timer == nil {
				timer = time.After(200 * time.Millisecond)
			}
		case <-w.Errors:
		case <-timer:
			timer = nil
			changed()
		}
	}
}

// Listing is a folder's folders, to choose a repository from.
type Listing struct {
	Dir     string  // the folder, whole, with slashes
	Parent  string  `json:",omitempty"` // "" at the top
	Entries []Entry `json:",omitempty"`
}

// Entry is a folder in a listing.
type Entry struct {
	Name string
	Repo bool `json:",omitempty"` // a repository's working tree
}

// list lists the folders in dir, the home folder for "", leaving out
// hidden ones, as a file dialog does.
func list(dir string) (Listing, error) {
	if dir == "" {
		dir = "."
	}
	abs, err := filepath.Abs(filepath.FromSlash(dir))
	if err != nil {
		return Listing{}, err
	}
	des, err := os.ReadDir(abs)
	if err != nil {
		return Listing{}, err
	}
	l := Listing{Dir: filepath.ToSlash(abs)}
	if parent := filepath.Dir(abs); parent != abs {
		l.Parent = filepath.ToSlash(parent)
	}
	for _, d := range des {
		name := d.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if !d.IsDir() {
			if d.Type()&fs.ModeSymlink == 0 {
				continue
			}
			if fi, err := os.Stat(filepath.Join(abs, name)); err != nil || !fi.IsDir() { // a link to a folder
				continue
			}
		}
		_, err := os.Stat(filepath.Join(abs, name, ".git"))
		l.Entries = append(l.Entries, Entry{Name: name, Repo: err == nil})
	}
	slices.SortFunc(l.Entries, func(a, b Entry) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return l, nil
}
