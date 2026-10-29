package ui

import (
	"runtime"
	"slices"
	"strings"

	"github.com/egoist/mygo/internal/platform"
)

// PopoverMenu gives the element a context menu, as ContextMenu does, which
// MyGo draws in the window with the theme's look, as a popover, rather
// than the system showing one of its own. It opens where the element is
// clicked with the secondary button, or Control-clicked on macOS, and,
// while the element or one inside it has the focus, below it when the
// menu key or Shift+F10 is pressed. build fills it with the items
// ContextMenu's function takes, and the item chosen reports Chosen as
// there:
//
//	row.PopoverMenu(func(m *ui.Menu) {
//		if m.Item("Rename").Shortcut(0, ui.KeyF2).Chosen() {
//			app.renaming = i
//		}
//		if m.Item("Pinned").Checked(note.pinned).Chosen() {
//			note.pinned = !note.pinned
//		}
//		m.Separator()
//		if m.Item("Delete").Disabled(note.locked).Chosen() {
//			app.delete(i)
//		}
//	})
//
// The menu takes the focus as it opens and gives it back as it closes, to
// the element that had it. Up and Down move among its items, past those
// disabled, as do Home, End and typing the first letters of an item;
// Right opens a submenu and Left closes it, the other way round right to
// left (Direction); Enter and Space choose an item, Escape closes the
// submenu on top, and a press outside the menu closes it. It shows below
// and after where it opened, and on the other side where the window has
// no room. Assistive technology sees a menu of items, check box items and
// radio items, as the system's.
func (e *node) PopoverMenu(build func(m *Menu)) *node {
	e.flags |= flagContextMenu
	e.buildMenu(false, true, build)
	e.c.rt.drawMenu(e, build)
	return e
}

// drawnMenu is a menu drawn in the window (PopoverMenu) that shows, of the
// element id.
type drawnMenu struct {
	id uint64
	// dx and dy are where it opened, from the top left corner of the
	// element's box; text is the text it opened on.
	dx, dy float32
	text   uint64
	// opened is the frame it opened in, whose presses do not close it, as
	// the press opening it; built and pass are the frame and the pass that
	// last built it.
	opened, built uint64
	pass          int
	// levels are the menu and its submenus that show, outermost first.
	levels []menuLevel
	// focus is the level to give the focus to once the menu is built, -1
	// for none.
	focus int
	// pointer is where the pointer was when it last moved the highlight:
	// it moves it again once it moves.
	pointer [2]float32
}

// menuLevel is a menu that shows: the item, by its place among the menu's
// items (platform.MenuItem.ID), whose submenu it is, 0 for the menu, and
// the item highlighted, 0 for none and -1 for the first that can be
// chosen, which scrolls into view when reveal is set, as the keys moved
// to it.
type menuLevel struct {
	sub, hi int
	reveal  bool
}

// openDrawnMenu opens the drawn menu of e, at the point asked for.
func (rt *engine) openDrawnMenu(e *node) {
	mr := &rt.menu
	if !e.hasState() {
		return
	}
	s := e.st
	d := &drawnMenu{id: e.id, text: mr.text, opened: rt.frame, dx: mr.x - s.vx, dy: mr.y - s.vy, focus: 0}
	d.pointer = [2]float32{rt.pointerX, rt.pointerY}
	first := 0
	if mr.key {
		first = -1 // the keyboard opened it: it starts on the first item
	}
	d.levels = []menuLevel{{hi: first}}
	mr.drawn = d
}

// menuDraw is a drawn menu being built in a pass.
type menuDraw struct {
	rt     *engine
	c      *context
	d      *drawnMenu
	moved  bool    // the pointer moved since it last moved the highlight
	panels []*node // by level
	// chosen is the item chosen; closed is set as the menu closes.
	chosen *platform.MenuItem
	closed bool
}

// drawMenu builds the menu drawn for e, while it shows: an overlay
// holding a panel for the menu and each submenu open, as a popover of e.
func (rt *engine) drawMenu(e *node, build func(m *Menu)) {
	d := rt.menu.drawn
	if d == nil || d.id != e.id || !e.hasState() || e.c.inert {
		return
	}
	d.built, d.pass = rt.frame, rt.pass
	if e.disabled() {
		rt.menu.drawn = nil
		return
	}
	run := &menuRun{collect: true}
	pm := &platform.Menu{}
	build(&Menu{run: run, menu: pm, rt: rt, text: rt.states[d.text]})
	if !slices.ContainsFunc(pm.Items, func(it *platform.MenuItem) bool { return it.Visible && it.Type != platform.MenuItemSeparator }) {
		rt.menu.drawn = nil
		return
	}
	c := e.c
	md := &menuDraw{rt: rt, c: c, d: d}
	p := [2]float32{rt.pointerX, rt.pointerY}
	md.moved = p != d.pointer
	coreOverlay(c, func() {
		// A point where the menu opened, which follows the element as it
		// moves: the menu goes after it, or before it where the window has
		// no room, as from a corner.
		at := coreBox(c).Key(mix(d.id, 0)).Size(0, 0).AttachTo(e, AnchorTopLeft, AnchorTopLeft).Margin(d.dy, 0, 0, d.dx)
		anchor, items := at, pm.Items
		for level := 0; level < len(d.levels) && !md.closed; level++ {
			panel, rows := md.panel(level, items, anchor)
			md.panels = append(md.panels, panel)
			if level+1 >= len(d.levels) || md.closed {
				break
			}
			sub := d.levels[level+1].sub
			i := slices.IndexFunc(items, func(it *platform.MenuItem) bool { return it.ID == sub })
			if i < 0 || items[i].Submenu == nil || !items[i].Enabled || rows[sub] == nil {
				// The item is gone, or can no longer be chosen.
				d.levels = d.levels[:level+1]
				break
			}
			anchor, items = rows[sub], items[i].Submenu.Items
		}
	})
	if md.closed {
		rt.menu.drawn = nil
	} else if f := d.focus; f >= 0 && f < len(md.panels) {
		// After the overlay noted the focus it came with, to give it back.
		md.panels[f].Focus()
		d.focus = -1
	}
	if it := md.chosen; it != nil {
		if name := run.commands[it.ID-1]; name != "" {
			target := d.text
			if target == 0 {
				target = d.id
			}
			rt.menuCommand(target, name)
			rt.requestFrame()
		} else {
			rt.menuChoice(build, it.ID, it.Label, d.text)
		}
	}
}

// panel builds the panel of the menu at level, of items, attached to
// anchor: the point where the menu opened, or the item opening the
// submenu. It returns the panel and its items' rows by place.
func (md *menuDraw) panel(level int, items []*platform.MenuItem, anchor *node) (*node, map[int]*node) {
	rt, c, d := md.rt, md.c, md.d
	t := c.theme
	panel := coreColumn(c).Key(mix(d.id, uint64(level)+1)).Role(RoleMenu)
	panel.widget = "PopoverMenu"
	// Presses on it stay in it; items past the room the window has scroll.
	panel.flags |= flagFocusable | flagClickable | flagTypeSelect | flagOwnRing | flagScrollY | flagHover
	panel.MinWidth(t.Space(45)).Padding(t.Space(1)).Radius(t.Radius+2).Background(t.Background).Border(1, t.Border)
	panel.Shadow(0, 6, 20, 0, RGBA(0, 0, 0, 0.18))
	if level == 0 {
		panel.AttachTo(anchor, AnchorTopStart, AnchorTopStart)
	} else {
		// After the item opening it, its first item lined up with it.
		panel.AttachTo(anchor, AnchorTopEnd, AnchorTopStart).Margin(-t.Space(1)-1, 0, 0, 0)
		panel.Label(anchor.label)
	}
	if d.levels[level].hi < 0 {
		d.levels[level].hi = 0
		if i := slices.IndexFunc(items, choosable); i >= 0 {
			d.levels[level].hi = items[i].ID
		}
	}
	marks := slices.ContainsFunc(items, func(it *platform.MenuItem) bool {
		return it.Type == platform.MenuItemCheckbox || it.Type == platform.MenuItemRadio
	})
	size := 0
	for _, it := range items {
		if it.Visible && it.Type != platform.MenuItemSeparator {
			size++
		}
	}
	rows := make(map[int]*node, size)
	var choices []*platform.MenuItem // those the keys move among
	panel.Children(func() {
		pos := 0
		for _, it := range items {
			if !it.Visible {
				continue
			}
			if it.Type == platform.MenuItemSeparator {
				coreBox(c).Height(1).Margin(t.Space(1), t.Space(2)).Background(t.Border).Shrink(0)
				continue
			}
			pos++
			row := md.item(level, it, marks)
			row.setPos, row.setSize = pos, size
			rows[it.ID] = row
			if choosable(it) {
				choices = append(choices, it)
			}
		}
	})
	if hi := rows[d.levels[level].hi]; hi != nil {
		panel.activeDescendant = hi
		if d.levels[level].reveal {
			hi.ScrollIntoView()
		}
	}
	d.levels[level].reveal = false
	if !md.closed {
		md.keys(level, panel, choices)
	}
	if level == 0 && d.opened != rt.frame && panel.PressedOutside() {
		md.closed = true
	}
	return panel, rows
}

// choosable reports whether the keys and the pointer can highlight an
// item.
func choosable(it *platform.MenuItem) bool {
	return it.Visible && it.Enabled && it.Type != platform.MenuItemSeparator
}

// item builds the row of item it of the menu at level, with room for the
// marks of check box and radio items when marks is set.
func (md *menuDraw) item(level int, it *platform.MenuItem, marks bool) *node {
	c, d := md.c, md.d
	t := c.theme
	row := coreRow(c).AlignItems(Center).Gap(t.Space(2)).Padding(t.Space(1.5), t.Space(2)).Radius(t.Radius).Shrink(0)
	row.Label(it.Label).Disabled(!it.Enabled)
	row.flags |= flagClickable | flagHover
	switch it.Type {
	case platform.MenuItemCheckbox:
		row.Role(RoleMenuItemCheckBox).Checked(it.Checked)
	case platform.MenuItemRadio:
		row.Role(RoleMenuItemRadio).Checked(it.Checked)
	default:
		row.Role(RoleMenuItem)
	}
	sub := it.Type == platform.MenuItemSubmenu && it.Submenu != nil
	open := sub && level+1 < len(d.levels) && d.levels[level+1].sub == it.ID
	if sub {
		row.expandable, row.expanded = true, open
	}
	keys := shortcutText(it.Accelerator)
	row.description = keys
	color, muted := t.Text, t.TextMuted
	switch hi := d.levels[level].hi == it.ID; {
	case !it.Enabled:
		color = t.TextMuted
		row.TextColor(color)
	case hi && open && d.levels[level+1].hi != 0:
		// The way to the item highlighted in the submenu.
		row.Background(t.SurfaceHover)
	case hi:
		color, muted = t.AccentText, t.AccentText
		row.Background(t.Accent).TextColor(color)
	}
	rtl := row.rtl()
	row.Children(func() {
		if marks {
			mark := coreBox(c).Size(t.Space(4), t.Space(4)).Shrink(0)
			if it.Checked {
				radio := it.Type == platform.MenuItemRadio
				mark.Draw(func(p *Painter, r Rect) {
					if radio {
						d := r.W / 4
						p.Fill(Rect{X: r.X + r.W/2 - d/2, Y: r.Y + r.H/2 - d/2, W: d, H: d}, color, d/2)
					} else {
						p.StrokePath(checkPath(r), 1.5, color)
					}
				})
			}
		}
		coreText(c, it.Label).SingleLine().Grow(1)
		if keys != "" {
			coreText(c, keys).SingleLine().TextColor(muted).MarginStart(t.Space(4))
		}
		if sub {
			coreBox(c).Size(t.Space(4), t.Space(4)).Shrink(0).Draw(func(p *Painter, r Rect) {
				p.StrokePath(disclosurePath(r, 0, rtl), 1.5, muted)
			})
		}
	})
	if !it.Enabled || md.closed {
		return row
	}
	if md.moved && row.Hovered() {
		// The pointer moved onto it: it is highlighted, and its submenu
		// opens while those of the others close. The keys go on from it.
		md.moved = false
		d.pointer = [2]float32{md.rt.pointerX, md.rt.pointerY}
		switch {
		case sub && !open:
			md.highlight(level, it.ID)
			md.openSubmenu(level, it, false)
		case !sub && (d.levels[level].hi != it.ID || len(d.levels) > level+1):
			md.highlight(level, it.ID)
		}
		d.focus = level
	}
	if row.Clicked() {
		if sub {
			md.highlight(level, it.ID)
			md.openSubmenu(level, it, false)
		} else {
			md.chosen, md.closed = it, true
		}
	}
	return row
}

// highlight highlights the item at place id of the menu at level, closing
// the submenus open in it.
func (md *menuDraw) highlight(level, id int) {
	d := md.d
	d.levels[level].hi = id
	d.levels = d.levels[:level+1]
	md.rt.consumed = true
}

// openSubmenu opens the submenu of item it of the menu at level; the
// keyboard opening it, it takes the focus on its first item.
func (md *menuDraw) openSubmenu(level int, it *platform.MenuItem, keyboard bool) {
	d := md.d
	d.levels = append(d.levels[:level+1], menuLevel{sub: it.ID})
	if keyboard {
		d.levels[level+1].hi = -1
		d.focus = level + 1
	}
	md.rt.consumed = true
}

// keys moves the highlight among choices, the items of the menu at level
// that can be chosen, opens and closes submenus and chooses items, for the
// keys pressed while the panel has the focus.
func (md *menuDraw) keys(level int, panel *node, choices []*platform.MenuItem) {
	d := md.d
	n := len(choices)
	at := slices.IndexFunc(choices, func(it *platform.MenuItem) bool { return it.ID == d.levels[level].hi })
	move := func(i int) {
		if n > 0 {
			md.highlight(level, choices[(i%n+n)%n].ID)
			d.levels[level].reveal = true
		}
	}
	into, out := KeyRight, KeyLeft
	if panel.rtl() {
		into, out = out, into
	}
	switch {
	case panel.Shortcut(0, KeyDown):
		move(at + 1) // from none, the first
	case panel.Shortcut(0, KeyUp):
		if at < 0 {
			at = 0
		}
		move(at - 1)
	case panel.Shortcut(0, KeyHome):
		move(0)
	case panel.Shortcut(0, KeyEnd):
		move(n - 1)
	case panel.Shortcut(0, into):
		if at >= 0 && choices[at].Type == platform.MenuItemSubmenu && choices[at].Submenu != nil {
			md.openSubmenu(level, choices[at], true)
		}
	case panel.Shortcut(0, out):
		if level > 0 {
			md.closeLevel(level)
		}
	case panel.Shortcut(0, KeyEnter), panel.Shortcut(0, KeySpace):
		switch {
		case at < 0:
		case choices[at].Type == platform.MenuItemSubmenu:
			if choices[at].Submenu != nil {
				md.openSubmenu(level, choices[at], true)
			}
		default:
			md.chosen, md.closed = choices[at], true
		}
	case panel.Shortcut(0, KeyTab), panel.Shortcut(Shift, KeyTab):
		md.closed = true
	}
	if s := panel.st; s.typing && s.typed != "" && n > 0 {
		// From the item after the one highlighted, round to it: the same
		// letter again goes to the next item starting with it.
		start := at
		if len([]rune(s.typed)) > 1 && at >= 0 {
			start = at - 1
		}
		typed := strings.ToLower(s.typed)
		for k := 1; k <= n; k++ {
			if j := (start + k + n) % n; strings.HasPrefix(strings.ToLower(choices[j].Label), typed) {
				move(j)
				break
			}
		}
	}
	if panel.OverlayShortcut(0, KeyEscape) {
		if level == 0 {
			md.closed = true
		} else {
			md.closeLevel(level)
		}
	}
	if md.closed {
		md.rt.consumed = true
	}
}

// closeLevel closes the submenu at level, giving the focus to the menu
// holding it.
func (md *menuDraw) closeLevel(level int) {
	d := md.d
	d.levels = d.levels[:level]
	d.focus = level - 1
	md.rt.consumed = true
}

// shortcutText writes an accelerator (accelerator) as a menu shows it by
// its item: with the symbols of macOS there, as "⇧⌘Z", and as "Ctrl+Shift+Z"
// elsewhere.
func shortcutText(a string) string {
	if a == "" {
		return ""
	}
	parts := strings.Split(a, "+")
	key, mods := parts[len(parts)-1], parts[:len(parts)-1]
	if len(key) == 1 {
		key = strings.ToUpper(key)
	}
	var b strings.Builder
	if runtime.GOOS == "darwin" {
		// In the order of macOS's menus.
		for _, m := range [...][2]string{{"Ctrl", "⌃"}, {"Alt", "⌥"}, {"Shift", "⇧"}, {"Super", "⌘"}} {
			if slices.Contains(mods, m[0]) {
				b.WriteString(m[1])
			}
		}
		switch key {
		case "Enter":
			key = "↩"
		case "Escape":
			key = "⎋"
		case "Backspace":
			key = "⌫"
		case "Delete":
			key = "⌦"
		case "Tab":
			key = "⇥"
		case "Left":
			key = "←"
		case "Right":
			key = "→"
		case "Up":
			key = "↑"
		case "Down":
			key = "↓"
		}
		b.WriteString(key)
		return b.String()
	}
	super := "Super"
	if runtime.GOOS == "windows" {
		super = "Win"
	}
	for _, m := range mods {
		if m == "Super" {
			m = super
		}
		b.WriteString(m)
		b.WriteByte('+')
	}
	b.WriteString(key)
	return b.String()
}
