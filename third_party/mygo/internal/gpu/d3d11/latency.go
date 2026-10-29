//go:build windows

package d3d11

import (
	"sync"
	"syscall"

	"github.com/egoist/mygo/internal/gpu/d3d11/device"
)

// The swap chain's frame latency object keeps one frame waiting ahead of
// the screen without Present blocking the main thread: DXGI signals it as
// the screen takes a frame, and a frame presents only once a wait on it
// succeeded. A frame drawn when it is not signaled yet presents later
// (pending), as a goroutine waiting on it has the main thread present it,
// rather than block in Present until the screen takes the last; frames
// paced by the display wait meanwhile (Ready), as they would have. Every
// present takes one wait's success, its token, so the object's count
// stays that of the frames the screen can take.

var (
	kernel32          = syscall.NewLazyDLL(device.SystemDir() + `\kernel32.dll`)
	procWaitForObject = kernel32.NewProc("WaitForSingleObject")
	procCloseHandle   = kernel32.NewProc("CloseHandle")
)

// latencyTimeout is the longest a wait on the object takes, in
// milliseconds: the screen takes a frame each refresh, but takes none while
// the window shows nowhere, when a frame presents without its token.
var latencyTimeout uintptr = 1000

const (
	waitObject0 = 0
	// swapChainWaitable is DXGI_SWAP_CHAIN_FLAG_FRAME_LATENCY_WAITABLE_OBJECT.
	swapChainWaitable = 0x40
)

type latency struct {
	// handle is the object, 0 without one: frames then present at once.
	handle uintptr

	mu sync.Mutex
	// token tells that a wait succeeded that no present took yet; waiting
	// that a goroutine waits on the object, which done tells the end of,
	// and then what it calls; lapsed that the goroutine's wait timed out,
	// so the present it calls on does not wait again, on the main thread.
	token, waiting, lapsed bool
	done                   chan struct{}
	then                   func()
}

// waitObject waits on the object, at most ms milliseconds, and reports
// whether it was signaled.
func (l *latency) waitObject(ms uintptr) bool {
	r, _, _ := procWaitForObject.Call(l.handle, ms)
	return r == waitObject0
}

// ready reports whether a present can take its token at once, waiting for
// nothing.
func (l *latency) ready() bool {
	if l.handle == 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.token && !l.waiting {
		l.token = l.waitObject(0)
	}
	return l.token
}

// waitThen has a goroutine wait for the object and call then, on its own
// goroutine, once it is signaled (or after latencyTimeout).
func (l *latency) waitThen(then func()) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.then = then
	if l.waiting {
		return
	}
	l.waiting, l.done = true, make(chan struct{})
	go func() {
		signaled := l.waitObject(latencyTimeout)
		l.mu.Lock()
		l.token = l.token || signaled
		l.lapsed = !l.token
		l.waiting = false
		then := l.then
		l.then = nil
		close(l.done)
		l.mu.Unlock()
		if then != nil {
			then()
		}
	}()
}

// take takes the token a present needs, waiting for it on the calling
// thread (a resize's frame, one waited for): at most latencyTimeout, after
// which the frame presents without one. A present whose goroutine's wait
// timed out already waited as long, while the window shows nowhere, and
// presents without waiting again.
func (l *latency) take() {
	if l.handle == 0 {
		return
	}
	l.mu.Lock()
	done, waiting := l.done, l.waiting
	l.mu.Unlock()
	if waiting {
		<-done
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.token && !l.lapsed {
		l.waitObject(latencyTimeout)
	}
	l.token, l.lapsed = false, false
}

// release waits for a goroutine waiting on the object and closes it.
func (l *latency) release() {
	if l.handle == 0 {
		return
	}
	l.mu.Lock()
	done, waiting := l.done, l.waiting
	l.then = nil
	l.mu.Unlock()
	if waiting {
		<-done
	}
	procCloseHandle.Call(l.handle)
	l.handle, l.token = 0, false
}
