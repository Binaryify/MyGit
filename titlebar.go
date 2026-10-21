package main

import (
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// titleBarHeight is the height of the title bars the windows draw.
const titleBarHeight = 40

// customTitleBar hides a window's title bar, keeping its controls, so the
// view draws the title bar: the traffic lights centered in it on macOS,
// the caption buttons filling its height on Windows.
func customTitleBar(o mygo.WindowOptions) mygo.WindowOptions {
	o.TitleBarStyle = mygo.TitleBarHidden
	o.TitleBarHeight = titleBarHeight
	o.TrafficLightPosition = &mygo.Point{X: 16, Y: (titleBarHeight - 14) / 2}
	return o
}

// dropMenuBar takes the menu bar off a window on Linux and Windows, where
// it would sit under the title bar the view draws: its items are in the
// window's own menu. macOS keeps its menu at the top of the screen.
func dropMenuBar(w *mygo.Window) {
	if runtime.GOOS != "darwin" {
		w.SetMenu(mygo.NewMenu(nil))
	}
}

// titleBar is a row the height of the title bar, clear of the window's
// controls, that drags the window and zooms it on a double click.
func titleBar(c *ui.Context) ui.Element {
	bar := c.TitleBar()
	left := float32(12)
	if bar.Left > 0 {
		left = bar.Left + 8
	}
	return ui.Row(c).Height(max(bar.Height, titleBarHeight)).Padding(0, bar.Right+8, 0, left).
		Gap(8).AlignItems(ui.Center).DragWindow()
}

// macOS tells whether the app runs on macOS, which has a menu bar where
// its menus are, other systems' windows carrying them, and writes keys as
// ⌘F, not Ctrl+F. Tests render other systems' windows by setting it.
var macOS = runtime.GOOS == "darwin"

// shiftShortcut writes a key with the command modifier and shift the way
// the platform does: ⇧⌘O on macOS, Ctrl+Shift+O elsewhere.
func shiftShortcut(key string) string {
	if macOS {
		return "⇧⌘" + key
	}
	return "Ctrl+Shift+" + key
}

// shiftKey writes a key with Shift the way the platform does: ⇧F7 on
// macOS, Shift+F7 elsewhere.
func shiftKey(key string) string {
	if macOS {
		return "⇧" + key
	}
	return "Shift+" + key
}

// shortcut writes a key with the command modifier the way the platform
// does: ⌘F on macOS, Ctrl+F elsewhere.
func shortcut(key string) string {
	if macOS {
		return "⌘" + key
	}
	if key == "↩" {
		key = "Enter"
	}
	return "Ctrl+" + key
}
