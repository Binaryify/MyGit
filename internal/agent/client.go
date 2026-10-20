package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sync"
)

// Client calls an agent. It runs git and reaches files on the agent's
// computer for a repository there, as git.Runner.
type Client struct {
	host string
	w    *wire
	ask  func(tag, prompt string) (string, bool)

	mu     sync.Mutex
	next   uint32
	calls  map[uint32]*pending
	err    error         // why the connection ended
	closed chan struct{} // closed as it ends
}

type pending struct {
	stdout, stderr *queue
	exit           chan Exit
}

// ErrClosed says the connection to the agent ended.
var ErrClosed = errors.New("the connection to the remote computer ended")

// NewClient talks to an agent reading r and writing w, an agent on host.
// Ask answers git's prompts there, tagged with the folder git runs in; it
// is called on a goroutine of its own.
func NewClient(host string, r io.Reader, w io.Writer, ask func(tag, prompt string) (string, bool)) (*Client, error) {
	br := bufio.NewReader(r)
	if err := skipToMagic(br); err != nil {
		return nil, err
	}
	c := &Client{host: host, w: &wire{w: w}, ask: ask, calls: map[uint32]*pending{}, closed: make(chan struct{})}
	go c.read(br)
	return c, nil
}

// skipToMagic reads past what a shell may print before the agent starts.
func skipToMagic(r *bufio.Reader) error {
	var seen []byte
	for {
		line, err := r.ReadBytes('\n')
		if string(line) == Magic || bytes.HasSuffix(line, []byte(Magic)) {
			return nil
		}
		seen = append(seen, line...)
		if err != nil {
			if msg := bytes.TrimSpace(seen); len(msg) > 0 {
				return fmt.Errorf("the agent didn't start: %s", msg)
			}
			return fmt.Errorf("the agent didn't start: %w", err)
		}
		if len(seen) > 64<<10 {
			return errors.New("the agent didn't start")
		}
	}
}

func (c *Client) read(r io.Reader) {
	var err error
	for {
		var kind byte
		var id uint32
		var data []byte
		if kind, id, data, err = readFrame(r); err != nil {
			break
		}
		switch kind {
		case kStdout, kStderr:
			if p := c.pending(id); p != nil {
				if kind == kStdout {
					_, _ = p.stdout.Write(data)
				} else {
					_, _ = p.stderr.Write(data)
				}
			}
		case kExit:
			var e Exit
			if json.Unmarshal(data, &e) != nil {
				e.Err = "agent: bad reply"
			}
			c.mu.Lock()
			p := c.calls[id]
			delete(c.calls, id)
			c.mu.Unlock()
			if p != nil {
				p.exit <- e
			}
		case kAsk:
			var a ask
			_ = json.Unmarshal(data, &a)
			go func() {
				var ans answer
				if c.ask != nil {
					ans.Answer, ans.OK = c.ask(a.Tag, a.Prompt)
				}
				_ = c.w.sendJSON(kAnswer, id, ans)
			}()
		}
	}
	c.mu.Lock()
	c.err = fmt.Errorf("%w: %v", ErrClosed, err)
	calls := c.calls
	c.calls = map[uint32]*pending{}
	c.mu.Unlock()
	close(c.closed)
	for _, p := range calls {
		p.exit <- Exit{Err: ErrClosed.Error()}
	}
}

func (c *Client) pending(id uint32) *pending {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls[id]
}

// Done is closed as the connection ends.
func (c *Client) Done() <-chan struct{} { return c.closed }

// call runs req, giving it stdin and copying what it prints to stdout and
// stderr, until it ends or ctx does.
func (c *Client) call(ctx context.Context, req Request, stdin io.Reader, stdout, stderr io.Writer) (Exit, error) {
	p := &pending{stdout: newQueue(), stderr: newQueue(), exit: make(chan Exit, 1)}
	c.mu.Lock()
	if c.err != nil {
		c.mu.Unlock()
		return Exit{}, c.err
	}
	c.next++
	id := c.next
	c.calls[id] = p
	c.mu.Unlock()
	if err := c.w.sendJSON(kStart, id, req); err != nil {
		return Exit{}, err
	}
	if stdin != nil {
		go func() {
			_, _ = io.Copy(stream{c.w, kStdin, id}, stdin)
			_ = c.w.send(kStdin, id, nil)
		}()
	}
	var copies sync.WaitGroup
	drain := func(q *queue, w io.Writer) {
		if w == nil {
			w = io.Discard
		}
		copies.Add(1)
		go func() {
			defer copies.Done()
			_, _ = io.Copy(w, q)
		}()
	}
	drain(p.stdout, stdout)
	drain(p.stderr, stderr)
	var e Exit
	select {
	case e = <-p.exit:
	case <-ctx.Done():
		_ = c.w.send(kCancel, id, nil)
		select {
		case e = <-p.exit:
		case <-c.closed:
		}
	}
	p.stdout.CloseWithError(nil)
	p.stderr.CloseWithError(nil)
	copies.Wait()
	if ctx.Err() != nil {
		return e, ctx.Err()
	}
	return e, nil
}

func (e Exit) error() error {
	switch {
	case e.Err == "":
		return nil
	case e.NotExist:
		return fmt.Errorf("%s: %w", e.Err, fs.ErrNotExist)
	}
	return errors.New(e.Err)
}

func (c *Client) do(ctx context.Context, req Request, stdin io.Reader, stdout io.Writer) error {
	e, err := c.call(ctx, req, stdin, stdout, nil)
	if err != nil {
		return err
	}
	return e.error()
}

// Exec runs git on the agent's computer.
func (c *Client) Exec(ctx context.Context, dir string, env []string, stdin io.Reader, stdout, stderr io.Writer, args ...string) error {
	e, err := c.call(ctx, Request{Op: "exec", Dir: dir, Args: args, Env: env, Stdin: stdin != nil}, stdin, stdout, stderr)
	if err != nil {
		return err
	}
	return e.error()
}

func (c *Client) ReadFile(ctx context.Context, path string) ([]byte, error) {
	var b bytes.Buffer
	err := c.do(ctx, Request{Op: "read", Path: path}, nil, &b)
	return b.Bytes(), err
}

func (c *Client) WriteFile(ctx context.Context, path string, data []byte) error {
	return c.do(ctx, Request{Op: "write", Path: path}, bytes.NewReader(data), nil)
}

func (c *Client) RemoveAll(ctx context.Context, path string) error {
	return c.do(ctx, Request{Op: "remove", Path: path}, nil, nil)
}

func (c *Client) Exists(ctx context.Context, path string) bool {
	e, err := c.call(ctx, Request{Op: "exists", Path: path}, nil, nil, nil)
	return err == nil && e.Err == "" && !e.No
}

// Join joins with slashes, which the agent reads on every system, as git
// prints paths on Windows too.
func (c *Client) Join(dir, rel string) string { return path.Join(dir, rel) }

func (c *Client) Host() string { return c.host }

// Watch calls changed as files of the tree at dir change, until ctx ends
// or the connection does.
func (c *Client) Watch(ctx context.Context, dir string, changed func()) error {
	return c.do(ctx, Request{Op: "watch", Dir: dir}, nil, writerFunc(func(p []byte) {
		for range bytes.Count(p, []byte{'\n'}) {
			changed()
		}
	}))
}

// List lists the folders in dir on the agent's computer, its home folder
// for "".
func (c *Client) List(ctx context.Context, dir string) (Listing, error) {
	var b bytes.Buffer
	if err := c.do(ctx, Request{Op: "list", Dir: dir}, nil, &b); err != nil {
		return Listing{}, err
	}
	var l Listing
	err := json.Unmarshal(b.Bytes(), &l)
	return l, err
}

type writerFunc func([]byte)

func (f writerFunc) Write(p []byte) (int, error) { f(p); return len(p), nil }
