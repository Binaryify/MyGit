package main

import "github.com/egoist/mygo/ui"

// Line icons in the style of Lucide, drawn in the text color.
var icons = map[string]*ui.SVG{}

func init() {
	for name, body := range map[string]string{
		"plus":  `<path d="M5 12h14"/><path d="M12 5v14"/>`,
		"minus": `<path d="M5 12h14"/>`,
		"undo":  `<path d="M9 14 4 9l5-5"/><path d="M4 9h10.5a5.5 5.5 0 0 1 0 11H11"/>`,
		"open":  `<path d="M15 3h6v6"/><path d="M10 14 21 3"/><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>`,
	} {
		icons[name] = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`))
	}
}

// iconButton is a small borderless button of an icon, as the actions on
// a row of VS Code's changes; its label names it to assistive technology
// and shows as its tooltip.
func iconButton(c *ui.Context, name, label string) bool {
	b := ui.ButtonBase(c.Key(name)).Size(22, 22).Radius(5).Center().Tooltip(label).Label(label).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(c.Theme().Text.Alpha(0.12))
	}
	b.Children(func() { ui.Icon(c, icons[name]).Size(14, 14) })
	return b.Clicked()
}
