package vt

import (
	"bytes"
	"runtime"
	"unsafe"
)

// What a Mirror reads of a terminal besides its rows, which a render state
// reads.

const (
	rowDataWrap = 1

	dataKittyKeyboardFlags = 8
	dataScrollbackRows     = 15
)

// ScrollbackRows returns how many rows the scrollback holds.
func (t *Terminal) ScrollbackRows() int {
	var n uintptr
	t.get(dataScrollbackRows, unsafe.Pointer(&n))
	return int(n)
}

// KittyKeyboardFlags returns the flags of the kitty keyboard protocol
// programs set.
func (t *Terminal) KittyKeyboardFlags() int {
	var f uint8
	t.get(dataKittyKeyboardFlags, unsafe.Pointer(&f))
	return int(f)
}

// ModifyOtherKeys reports whether xterm's modifyOtherKeys mode 2 is on.
// The library tells it in no GhosttyTerminalData: its formatter writes it
// among the keyboard's modes (CSI > 4 ; 2 m), here after the one cell of
// a selection, which keeps the text it formats to that cell.
func (t *Terminal) ModifyOtherKeys() bool {
	ref, found := t.CellAt(0, 0)
	if !found {
		return false
	}
	sel := &Selection{size: unsafe.Sizeof(Selection{}), start: ref, end: ref}
	opts := formatterOptions{size: unsafe.Sizeof(formatterOptions{}), emit: formatVT, selection: uintptr(unsafe.Pointer(sel))}
	opts.extra.size = unsafe.Sizeof(opts.extra)
	opts.extra.keyboard = true
	opts.extra.screen.size = unsafe.Sizeof(opts.extra.screen)
	var f uintptr
	if formatterTerminalNew(0, &f, t.h, opts) != 0 {
		return false
	}
	defer call(fnFormatterFree, f)
	var p, n uintptr
	formatted := ok(call(fnFormatterFormatAlloc, f, 0, uintptr(unsafe.Pointer(&p)), uintptr(unsafe.Pointer(&n))))
	runtime.KeepAlive(sel)
	if !formatted {
		return false
	}
	// A cell's text holds no escape, so the sequence is the keyboard's.
	on := bytes.Contains([]byte(goString(p, n)), []byte("\x1b[>4;2m"))
	if p != 0 {
		call(fnFree, 0, p, n)
	}
	return on
}
