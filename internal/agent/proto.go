// Package agent runs git for a window on another computer. The app starts
// the agent there over SSH and they talk over its standard input and
// output: the app asks for git runs and file reads and writes, each a
// call with an id, and the agent streams back what each prints and how it
// ended, and hands the app git's prompts for a password or a passphrase.
//
// The wire is frames: a kind, the call's id and a length, then that many
// bytes.
package agent

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"sync"
)

// Magic is the line the agent starts its output with, after which frames
// come: what a shell's startup files print before it is skipped.
const Magic = "MYGIT-AGENT 1\n"

const (
	// From the app.
	kStart  byte = 1 // a call: its request, JSON
	kStdin  byte = 2 // input of a call; empty at its end
	kCancel byte = 3 // stop a call
	kAnswer byte = 4 // the answer of a prompt, JSON

	// From the agent.
	kStdout byte = 10 // output of a call
	kStderr byte = 11
	kExit   byte = 12 // a call ended, JSON
	kAsk    byte = 13 // a prompt, JSON
)

// maxFrame bounds a frame, so a broken stream can't ask for any memory.
const maxFrame = 16 << 20

// chunk is the most a frame of output or input carries.
const chunk = 32 << 10

// Request is a call.
type Request struct {
	Op   string   // exec, read, write, remove, exists or watch
	Dir  string   `json:",omitempty"` // exec: the folder git runs in; watch: the tree
	Path string   `json:",omitempty"` // read, write, remove, exists
	Args []string `json:",omitempty"` // exec: git's
	Env  []string `json:",omitempty"` // exec: added to git's environment
	// Stdin says input comes for exec; write's is always the file.
	Stdin bool `json:",omitempty"`
}

// Exit is how a call ended.
type Exit struct {
	Err      string `json:",omitempty"`
	NotExist bool   `json:",omitempty"` // the file wasn't there
	No       bool   `json:",omitempty"` // exists: it isn't
}

type ask struct{ Tag, Prompt string }

type answer struct {
	Answer string
	OK     bool
}

// wire writes frames, one at a time.
type wire struct {
	mu sync.Mutex
	w  io.Writer
}

func (w *wire) send(kind byte, id uint32, data []byte) error {
	var h [9]byte
	h[0] = kind
	binary.BigEndian.PutUint32(h[1:], id)
	binary.BigEndian.PutUint32(h[5:], uint32(len(data)))
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := w.w.Write(h[:]); err != nil {
		return err
	}
	_, err := w.w.Write(data)
	return err
}

func (w *wire) sendJSON(kind byte, id uint32, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return w.send(kind, id, b)
}

// stream sends what is written as frames of kind.
type stream struct {
	w    *wire
	kind byte
	id   uint32
}

func (s stream) Write(p []byte) (int, error) {
	for n := 0; n < len(p); {
		m := min(len(p)-n, chunk)
		if err := s.w.send(s.kind, s.id, p[n:n+m]); err != nil {
			return n, err
		}
		n += m
	}
	return len(p), nil
}

func readFrame(r io.Reader) (kind byte, id uint32, data []byte, err error) {
	var h [9]byte
	if _, err = io.ReadFull(r, h[:]); err != nil {
		return
	}
	kind, id = h[0], binary.BigEndian.Uint32(h[1:])
	n := binary.BigEndian.Uint32(h[5:])
	if n > maxFrame {
		return 0, 0, nil, errors.New("agent: frame too big")
	}
	data = make([]byte, n)
	_, err = io.ReadFull(r, data)
	return
}

// queue is bytes written by one side and read by the other that never
// blocks the writer, so a call slow to read doesn't hold up the others
// sharing the wire.
type queue struct {
	mu     sync.Mutex
	cond   *sync.Cond
	buf    [][]byte
	closed bool
	err    error
}

func newQueue() *queue {
	q := &queue{}
	q.cond = sync.NewCond(&q.mu)
	return q
}

func (q *queue) Write(p []byte) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.closed && len(p) > 0 {
		q.buf = append(q.buf, append([]byte(nil), p...))
		q.cond.Broadcast()
	}
	return len(p), nil
}

// CloseWithError ends what is read, with err, or io.EOF when nil.
func (q *queue) CloseWithError(err error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.closed {
		q.closed, q.err = true, err
		q.cond.Broadcast()
	}
}

func (q *queue) Read(p []byte) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.buf) == 0 {
		if q.closed {
			if q.err != nil {
				return 0, q.err
			}
			return 0, io.EOF
		}
		q.cond.Wait()
	}
	n := copy(p, q.buf[0])
	if n == len(q.buf[0]) {
		q.buf = q.buf[1:]
	} else {
		q.buf[0] = q.buf[0][n:]
	}
	return n, nil
}
