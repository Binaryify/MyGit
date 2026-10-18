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
