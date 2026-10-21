package main

import "github.com/egoist/mygo/ui"

// Line icons in the style of Lucide, drawn in the text color.
var icons = map[string]*ui.SVG{}

func init() {
	for name, body := range map[string]string{
		"plus":          `<path d="M5 12h14"/><path d="M12 5v14"/>`,
		"search":        `<circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/>`,
		"x":             `<path d="M18 6 6 18"/><path d="m6 6 12 12"/>`,
		"minus":         `<path d="M5 12h14"/>`,
		"check":         `<path d="M20 6 9 17l-5-5"/>`,
		"edit":          `<path d="M12 20h9"/><path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4Z"/>`,
		"circle":        `<circle cx="12" cy="12" r="9"/>`,
		"undo":          `<path d="M9 14 4 9l5-5"/><path d="M4 9h10.5a5.5 5.5 0 0 1 0 11H11"/>`,
		"folder":        `<path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/>`,
		"repo":          `<circle cx="6" cy="6" r="3"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="6" r="3"/><path d="M6 9v6"/><path d="M18 9a9 9 0 0 1-9 9"/>`,
		"server":        `<rect width="20" height="8" x="2" y="2" rx="2"/><rect width="20" height="8" x="2" y="14" rx="2"/><path d="M6 6h.01"/><path d="M6 18h.01"/>`,
		"chevron-down":  `<path d="m6 9 6 6 6-6"/>`,
		"chevron-right": `<path d="m9 18 6-6-6-6"/>`,
		"refresh":       `<path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M8 16H3v5"/>`,
		"more":          `<circle cx="5" cy="12" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/>`,
		"up":            `<path d="m18 15-6-6-6 6"/>`,
		"wand":          `<path d="m21.64 3.64-1.28-1.28a1.21 1.21 0 0 0-1.72 0L2.36 18.64a1.21 1.21 0 0 0 0 1.72l1.28 1.28a1.2 1.2 0 0 0 1.72 0L21.64 5.36a1.2 1.2 0 0 0 0-1.72"/><path d="m14 7 3 3"/><path d="M5 6v4"/><path d="M19 14v4"/><path d="M10 2v2"/><path d="M7 8H3"/><path d="M21 16h-4"/><path d="M11 3H9"/>`,
		"open":          `<path d="M15 3h6v6"/><path d="M10 14 21 3"/><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>`,
	} {
		icons[name] = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`))
	}
}

// iconButton is a small borderless button of an icon, as the actions on
// a row of VS Code's changes; its label names it to assistive technology
// and shows as its tooltip, or tip does when given.
func iconButton(c *ui.Context, name, label string, tip ...string) bool {
	tooltip := label
	if len(tip) > 0 {
		tooltip = tip[0]
	}
	b := ui.ButtonBase(c.Key(name)).Size(22, 22).Radius(5).Center().Tooltip(tooltip).Label(label).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(c.Theme().Text.Alpha(0.12))
	}
	b.Children(func() { ui.Icon(c, icons[name]).Size(14, 14) })
	return b.Clicked()
}

// iconMenu is an icon button opening a menu, as iconButton looks.
func iconMenu(c *ui.Context, name, label string, build func(m *ui.Menu)) {
	b := ui.ButtonBase(c.Key(name)).Size(22, 22).Radius(5).Center().Tooltip(label).Label(label).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(c.Theme().Text.Alpha(0.12))
	}
	b.Children(func() { ui.Icon(c, icons[name]).Size(14, 14) })
	b.Menu(build)
}
