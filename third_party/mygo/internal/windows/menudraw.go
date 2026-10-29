//go:build windows && (amd64 || arm64)

package windows

import (
	"math"
	"sync"
	"syscall"
	"unsafe"

	"github.com/egoist/mygo/internal/platform"
)

// Windows 11 still draws Win32 menus as Windows 10 did, while its own menus
// (WinUI's MenuFlyout and MenuBar, File Explorer's context menu) have more
// room, highlights with rounded corners inset from the edges, and
// shortcuts in a fainter color. MyGo draws the items of its popup menus so
// there (MFT_OWNERDRAW), and those of the menu bar (darkmenu.go). The menus
// themselves stay Windows': their windows, rounded corners, animations and
// keyboard, and the items keep their strings, which mnemonics, screen
// readers and the tests read. Windows 10 keeps its own menus.

const (
	wmInitMenuPopup = 0x0117
	wmMeasureItem   = 0x002C
	wmDrawItem      = 0x002B

	odtMenu                = 1
	odsChecked             = 0x0008
	miimData               = 0x0020
	mftOwnerDraw           = 0x0100
	mftSeparator           = 0x0800
	mimBackground          = 0x0002
	dtCalcRect             = 0x400
	dtRight                = 0x002
	srcCopy                = 0x00CC0020
	smCxMenuCheck          = 71
	spiGetNonClientMetrics = 0x0029
	layoutRTL              = 0x1
)

var (
	procBitBlt                     = gdi32.NewProc("BitBlt")
	procGetLayout                  = gdi32.NewProc("GetLayout")
	procExcludeClipRect            = gdi32.NewProc("ExcludeClipRect")
	procCreateFontIndirectW        = gdi32.NewProc("CreateFontIndirectW")
	procSetMenuInfo                = user32.NewProc("SetMenuInfo")
	procGetSysColor                = user32.NewProc("GetSysColor")
	procSystemParametersInfoForDpi = user32.NewProc("SystemParametersInfoForDpi")
)

// Popup items' metrics, in DIPs, after WinUI's MenuFlyoutItem: a highlight
// inset from the menu's sides with rounded corners, the text past its
// padding, and marks (checks, the submenus' chevrons) in a column of
// their own.
const (
	menuItemHeight      = 32
	menuSeparatorHeight = 9
	menuInset           = 4  // the highlight from the menu's sides
	menuPadding         = 12 // the text from the highlight's sides
	menuRadius          = 4
	menuMarkColumn      = 16 + 12 // a check, and the gap after it
	menuAccelGap        = 32      // the least between a label and its shortcut
	menuChevron         = 12
	menuGlyphSize       = 12
)

// The menu bar's, which grows to WinUI's room.
const (
	menuBarHeight = 30
	menuBarExtra  = 8 // wider than Windows' items
	menuBarInsetX = 2
	menuBarInsetY = 3
)

// fluentMenus reports whether MyGo draws the menus in Windows 11's style.
func fluentMenus() bool { return windows11() }

// menuLook is what an item MyGo draws shows.
type menuLook struct {
	owner     int
	label     string // with its mnemonic's &
	accel     string
	separator bool
	submenu   bool
	radio     bool
	// column is whether the menu has a column for the marks of checkable
	// items, which all its items leave room for.
	column bool
	// icon is the item's icon, and icons whether the menu has a column
	// for icons, after that of the marks (menuicon.go).
	icon  *platform.MenuIcon
	icons bool
	// first and last are whether the item is the menu's first or last,
	// which has the menu's padding above or below it.
	first, last bool
	// menu and pos are the menu holding the item and its position there.
	menu uintptr
	pos  int
}

// ftype returns the type the item has besides MFT_OWNERDRAW.
func (l *menuLook) ftype() uint32 {
	switch {
	case l.separator:
		return mftSeparator
	case l.radio:
		return mftRadioCheck
	}
	return 0
}

// padding returns the room above and below the item: the menu's padding,
// as much as its highlights' inset from its sides, above its first item
// and below its last.
func (l *menuLook) padding(dpi int) (top, bottom int32) {
	if l.first {
		top = toPx(menuInset, dpi)
	}
	if l.last {
		bottom = toPx(menuInset, dpi)
	}
	return top, bottom
}

// addLook registers the look of an item and returns its key, the item's
// data.
func (t *menuTable) addLook(l *menuLook) uintptr {
	if t.looks == nil {
		t.looks = map[uintptr]*menuLook{}
	}
	t.nextLook++
	t.looks[t.nextLook] = l
	return t.nextLook
}

// ownerDraw makes the item at pos of menu one MyGo draws, of type ftype
// (MFT_SEPARATOR, MFT_RADIOCHECK or 0) besides, with the look of key.
func ownerDraw(menu uintptr, pos int, ftype uint32, key uintptr) {
	mi := menuItemInfo{Mask: miimFType | miimData, Type: ftype | mftOwnerDraw, ItemData: key}
	mi.Size = uint32(unsafe.Sizeof(mi))
	procSetMenuItemInfoW.Call(menu, uintptr(pos), 1, uintptr(unsafe.Pointer(&mi)))
}

type menuItemInfo struct {
	Size         uint32
	Mask         uint32
	Type         uint32
	State        uint32
	ID           uint32
	SubMenu      uintptr
	BmpChecked   uintptr
	BmpUnchecked uintptr
	ItemData     uintptr
	TypeData     *uint16
	Cch          uint32
	BmpItem      uintptr
}

type measureItemStruct struct {
	CtlType, CtlID, ItemID, ItemWidth, ItemHeight uint32
	ItemData                                      uintptr
}

// menuPalette is the colors of popup menus, COLORREFs: the background and
// outline of Windows 11's menus, and WinUI's colors opaque over them. A
// contrast theme's are the system's.
type menuPalette struct {
	back, plate, text, plateText, secondary, disabled, line uint32
	// border is the menu's outline, which DWM draws around the rounded
	// window, DWMWA_COLOR_DEFAULT for its own.
	border uint32
}

const (
	colorMenu          = 4
	colorMenuText      = 7
	colorGrayText      = 17
	colorHighlight     = 13
	colorHighlightText = 14
)

func sysColor(i uintptr) uint32 {
	c, _, _ := procGetSysColor.Call(i)
	return uint32(c)
}

func (b *Backend) menuPalette() menuPalette {
	switch {
	case highContrastOn():
		text := sysColor(colorMenuText)
		return menuPalette{back: sysColor(colorMenu), plate: sysColor(colorHighlight), text: text,
			plateText: sysColor(colorHighlightText), secondary: text, disabled: sysColor(colorGrayText), line: text,
			border: dwmColorDefault}
	case b.isDark():
		return menuPalette{back: 0x2B2B2B, plate: 0x383838, text: 0xFFFFFF, plateText: 0xFFFFFF,
			secondary: 0xCFCFCF, disabled: 0x787878, line: 0x3D3D3D, border: 0x3E3E3E}
	}
	return menuPalette{back: 0xF9F9F9, plate: 0xF0F0F0, text: 0x1A1A1A, plateText: 0x1A1A1A,
		secondary: 0x5F5F5F, disabled: 0x9F9F9F, line: 0xEAEAEA, border: 0xE5E5E5}
}

// menuBrush returns a brush of a color, made once.
func (b *Backend) menuBrush(color uint32) uintptr {
	if br, ok := b.menuBrushes[color]; ok {
		return br
	}
	br, _, _ := procCreateSolidBrush.Call(uintptr(color))
	if b.menuBrushes == nil {
		b.menuBrushes = map[uint32]uintptr{}
	}
	b.menuBrushes[color] = br
	return br
}

// menuFonts are the fonts of popup items at a DPI: the text, and the
// glyphs of the marks.
type menuFonts struct {
	text, glyphs uintptr
	fluent       bool // the glyphs are Segoe Fluent Icons'
}

// logFont is LOGFONTW.
type logFont struct {
	Height, Width, Escapement, Orientation, Weight       int32
	Italic, Underline, StrikeOut, CharSet                uint8
	OutPrecision, ClipPrecision, Quality, PitchAndFamily uint8
	FaceName                                             [32]uint16
}

// nonClientMetrics is NONCLIENTMETRICSW.
type nonClientMetrics struct {
	Size                                   uint32
	BorderWidth, ScrollWidth, ScrollHeight int32
	CaptionWidth, CaptionHeight            int32
	CaptionFont                            logFont
	SmCaptionWidth, SmCaptionHeight        int32
	SmCaptionFont                          logFont
	MenuWidth, MenuHeight                  int32
	MenuFont, StatusFont, MessageFont      logFont
	PaddedBorderWidth                      int32
}

// menuFace returns the fonts of popup items at dpi: the menu font of the
// system, which follows the size of text of Accessibility, from its 12
// pixels to WinUI's 14, in Segoe UI Variable where it is Segoe UI.
func (b *Backend) menuFace(dpi int) menuFonts {
	if f, ok := b.menuFonts[dpi]; ok {
		return f
	}
	var ncm nonClientMetrics
	ncm.Size = uint32(unsafe.Sizeof(ncm))
	lf := logFont{Height: -toPx(12, dpi), Weight: 400}
	copy(lf.FaceName[:], utf16z("Segoe UI"))
	if r, _, _ := procSystemParametersInfoForDpi.Call(spiGetNonClientMetrics, uintptr(ncm.Size), uintptr(unsafe.Pointer(&ncm)), 0, uintptr(dpi)); r != 0 {
		lf = ncm.MenuFont
	}
	lf.Height = int32(math.Round(float64(lf.Height) * 14 / 12))
	f := menuFonts{}
	if string(utf16Decode(lf.FaceName[:])) == "Segoe UI" {
		v := lf
		v.FaceName = [32]uint16{}
		copy(v.FaceName[:], utf16z("Segoe UI Variable Text"))
		f.text, _, _ = procCreateFontIndirectW.Call(uintptr(unsafe.Pointer(&v)))
		if !fontFace(f.text, "Segoe UI Variable Text") {
			procDeleteObject.Call(f.text)
			f.text = 0
		}
	}
	if f.text == 0 {
		f.text, _, _ = procCreateFontIndirectW.Call(uintptr(unsafe.Pointer(&lf)))
	}
	size := -toPx(menuGlyphSize, dpi)
	f.glyphs, f.fluent = createFont(size, "Segoe Fluent Icons"), true
	if !fontFace(f.glyphs, "Segoe Fluent Icons") {
		procDeleteObject.Call(f.glyphs)
		f.glyphs, f.fluent = createFont(size, "Segoe MDL2 Assets"), false
	}
	if b.menuFonts == nil {
		b.menuFonts = map[int]menuFonts{}
	}
	b.menuFonts[dpi] = f
	return f
}

// dropMenuFonts forgets the fonts of popup items, for the next menus to
// follow a change of the system's.
func (b *Backend) dropMenuFonts() {
	for _, f := range b.menuFonts {
		procDeleteObject.Call(f.text)
		procDeleteObject.Call(f.glyphs)
	}
	b.menuFonts = nil
}

// The glyphs of Segoe Fluent Icons, also in Segoe MDL2 Assets.
const (
	glyphCheck       = 0xE73E // CheckMark
	glyphBullet      = 0xE915 // RadioBullet
	glyphChevron     = 0xE76C // ChevronRight
	glyphChevronLeft = 0xE76B // ChevronLeft
)

// menuItemMessage handles the messages of the popup items MyGo draws, which
// the menus send the window they show for.
func (b *Backend) menuItemMessage(hwnd uintptr, m uint32, wp, lp uintptr) (uintptr, bool) {
	switch m {
	case wmEnterMenuLoop:
		b.hookMenus(true)
		return 0, false
	case wmExitMenuLoop:
		b.hookMenus(false)
		return 0, false
	case wmInitMenuPopup:
		// The menu's background in the appearance, around its items. The
		// window menu is Windows'.
		if fluentMenus() && hiword(lp) == 0 {
			b.setMenuBackground(wp)
		}
		return 0, false
	case wmMeasureItem:
		mis := (*measureItemStruct)(native(lp))
		if mis.CtlType != odtMenu {
			return 0, false
		}
		l := b.menus.looks[mis.ItemData]
		if l == nil {
			return 0, false
		}
		b.measureItem(hwnd, l, mis)
		return 1, true
	case wmDrawItem:
		d := (*drawItemStruct)(native(lp))
		if d.CtlType != odtMenu {
			return 0, false
		}
		l := b.menus.looks[d.ItemData]
		if l == nil {
			return 0, false
		}
		b.drawItem(hwnd, l, d)
		return 1, true
	}
	return 0, false
}

func (b *Backend) setMenuBackground(menu uintptr) {
	type menuInfo struct {
		Size, Mask, Style, CyMax uint32
		Back                     uintptr
		ContextHelpID            uint32
		MenuData                 uintptr
	}
	mi := menuInfo{Mask: mimBackground, Back: b.menuBrush(b.menuPalette().back)}
	mi.Size = uint32(unsafe.Sizeof(mi))
	procSetMenuInfo.Call(menu, uintptr(unsafe.Pointer(&mi)))
}

// textWidth measures text in a font, with its mnemonic's & left out.
func textWidth(dc, font uintptr, text string) int32 {
	if text == "" {
		return 0
	}
	old, _, _ := procSelectObject.Call(dc, font)
	defer procSelectObject.Call(dc, old)
	var r rect
	s := u16(text)
	procDrawTextW.Call(dc, uintptr(unsafe.Pointer(s)), ^uintptr(0), uintptr(unsafe.Pointer(&r)), dtCalcRect|dtSingleLine)
	return r.Right - r.Left
}

func (b *Backend) measureItem(hwnd uintptr, l *menuLook, mis *measureItemStruct) {
	dpi := dpiOf(hwnd)
	top, bottom := l.padding(dpi)
	if l.separator {
		mis.ItemWidth, mis.ItemHeight = 0, uint32(toPx(menuSeparatorHeight, dpi)+top+bottom)
		return
	}
	f := b.menuFace(dpi)
	dc, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, dc)
	w := toPx(menuInset+menuPadding, dpi)*2 + textWidth(dc, f.text, l.label)
	if l.column {
		w += toPx(menuMarkColumn, dpi)
	}
	if l.icons {
		w += toPx(menuIconColumn, dpi)
	}
	if l.accel != "" {
		w += toPx(menuAccelGap, dpi) + textWidth(dc, f.text, l.accel)
	}
	if l.submenu {
		w += toPx(menuAccelGap/2+menuChevron, dpi)
	}
	// Windows adds the width of its checks to that of every item.
	check, _, _ := procGetSystemMetricsForDpi.Call(smCxMenuCheck, uintptr(dpi))
	w = max(w-int32(check)+1, 0)
	mis.ItemWidth, mis.ItemHeight = uint32(w), uint32(toPx(menuItemHeight, dpi)+top+bottom)
}

func (b *Backend) drawItem(hwnd uintptr, l *menuLook, d *drawItemStruct) {
	dpi := dpiOf(hwnd)
	p := b.menuPalette()
	r := d.Item
	state := d.ItemState
	selected := state&odsSelected != 0 && !l.separator
	inset := toPx(menuInset, dpi)
	top, bottom := l.padding(dpi)
	var plate *rect
	if selected {
		plate = &rect{inset, top, r.Right - r.Left - inset, r.Bottom - r.Top - bottom}
	}
	paintItem(d.DC, r, p.back, plate, p.plate, float64(toPx(menuRadius, dpi)), func(img *bitmap, r rect) {
		dc := img.dc
		r.Top, r.Bottom = r.Top+top, r.Bottom-bottom
		if l.separator {
			thick := max(toPx(1, dpi), 1)
			y := (r.Bottom - r.Top - thick) / 2
			line := rect{r.Left + inset, y, r.Right - inset, y + thick}
			fillColor(dc, &line, p.line)
			return
		}
		fore, secondary := p.text, p.secondary
		if selected {
			fore = p.plateText
			if highContrastOn() {
				secondary = fore
			}
		}
		if state&(odsGrayed|odsDisabled) != 0 {
			fore, secondary = p.disabled, p.disabled
		}
		f := b.menuFace(dpi)
		procSetBkMode.Call(dc, 1) // TRANSPARENT
		x := r.Left + toPx(menuInset+menuPadding, dpi)
		right := r.Right - toPx(menuInset+menuPadding, dpi)
		glyph := func(g uint16, box rect, color uint32) {
			old, _, _ := procSelectObject.Call(dc, f.glyphs)
			procSetTextColor.Call(dc, uintptr(color))
			procDrawTextW.Call(dc, uintptr(unsafe.Pointer(&g)), 1, uintptr(unsafe.Pointer(&box)), dtCenter|dtVCenter|dtSingleLine|dtNoPrefix)
			procSelectObject.Call(dc, old)
		}
		if l.column {
			if state&odsChecked != 0 {
				g := uint16(glyphCheck)
				if l.radio {
					g = glyphBullet
				}
				glyph(g, rect{x, r.Top, x + toPx(16, dpi), r.Bottom}, fore)
			}
			x += toPx(menuMarkColumn, dpi)
		}
		if l.icons {
			box := toPx(platform.MenuIconSize, dpi)
			if ic := l.icon; ic != nil && drawMenuIcon != nil {
				// A template icon in the text's color; one in color
				// faded when the item is disabled.
				opacity := 1.0
				if state&(odsGrayed|odsDisabled) != 0 && !ic.Template {
					opacity = 0.4
				}
				// In a mirrored DC, which lays the item out from the
				// right, x counts from the right edge of the bitmap.
				at := x
				if layout, _, _ := procGetLayout.Call(dc); layout&layoutRTL != 0 {
					at = r.Right - x - box
				}
				drawMenuIcon(img.px, r.Right-r.Left, ic, at, r.Top+(r.Bottom-r.Top-box)/2, box, dibColor(fore), opacity)
			}
			x += toPx(menuIconColumn, dpi)
		}
		if l.submenu {
			// A menu laid out right to left has a mirrored DC, which
			// places the glyphs from the right but draws them unmirrored:
			// its submenus open to the left.
			g := uint16(glyphChevron)
			if layout, _, _ := procGetLayout.Call(d.DC); layout&layoutRTL != 0 {
				g = glyphChevronLeft
			}
			glyph(g, rect{right - toPx(menuChevron, dpi), r.Top, right, r.Bottom}, fore)
		}
		old, _, _ := procSelectObject.Call(dc, f.text)
		defer procSelectObject.Call(dc, old)
		flags := uintptr(dtSingleLine | dtVCenter)
		if state&odsNoAccel != 0 {
			flags |= dtHidePrefix
		}
		procSetTextColor.Call(dc, uintptr(fore))
		text := rect{x, r.Top, right, r.Bottom}
		s := u16(l.label)
		procDrawTextW.Call(dc, uintptr(unsafe.Pointer(s)), ^uintptr(0), uintptr(unsafe.Pointer(&text)), flags)
		if l.accel != "" {
			procSetTextColor.Call(dc, uintptr(secondary))
			s := u16(l.accel)
			procDrawTextW.Call(dc, uintptr(unsafe.Pointer(s)), ^uintptr(0), uintptr(unsafe.Pointer(&text)), dtSingleLine|dtVCenter|dtRight|dtNoPrefix)
		}
	})
	// Windows draws the arrow of a submenu after the item, in its colors:
	// the chevron is drawn already.
	procExcludeClipRect.Call(d.DC, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom))
}

// paintItem draws an item in a bitmap its size, then copies it to the
// menu's DC at r: the background, a highlight with rounded corners
// (plate, in the item's coordinates, nil for none) antialiased over it,
// and what draw draws over them, in the bitmap's coordinates.
func paintItem(dc uintptr, r rect, back uint32, plate *rect, plateColor uint32, radius float64, draw func(img *bitmap, r rect)) {
	w, h := r.Right-r.Left, r.Bottom-r.Top
	if w <= 0 || h <= 0 {
		return
	}
	img := newBitmap(dc, w, h)
	if img == nil {
		return
	}
	defer img.free()
	bg, fg := dibColor(back), dibColor(plateColor)
	for i := range img.px {
		img.px[i] = bg
	}
	if plate != nil {
		for y := max(plate.Top, 0); y < min(plate.Bottom, h); y++ {
			for x := max(plate.Left, 0); x < min(plate.Right, w); x++ {
				img.px[y*w+x] = blend(fg, bg, roundedCoverage(float64(x)+0.5, float64(y)+0.5, plate, radius))
			}
		}
	}
	procGdiFlush.Call()
	draw(img, rect{0, 0, w, h})
	procGdiFlush.Call()
	procBitBlt.Call(dc, uintptr(r.Left), uintptr(r.Top), uintptr(w), uintptr(h), img.dc, 0, 0, srcCopy)
}

// roundedCoverage returns how much of the pixel centered at (x, y) a
// rectangle with corners rounded by radius covers, from 0 to 1.
func roundedCoverage(x, y float64, r *rect, radius float64) float64 {
	cx := min(max(x, float64(r.Left)+radius), float64(r.Right)-radius)
	cy := min(max(y, float64(r.Top)+radius), float64(r.Bottom)-radius)
	d := math.Hypot(x-cx, y-cy)
	return min(max(radius-d+0.5, 0), 1)
}

// dibColor returns a COLORREF (0x00BBGGRR) as a DIB's pixel (0x00RRGGBB).
func dibColor(c uint32) uint32 { return c&0xFF<<16 | c&0xFF00 | c>>16&0xFF }

// blend mixes two DIB pixels, a over b by t from 0 to 1.
func blend(a, b uint32, t float64) uint32 {
	mix := func(shift uint) uint32 {
		x, y := float64(a>>shift&0xFF), float64(b>>shift&0xFF)
		return uint32(math.Round(y+(x-y)*t)) << shift
	}
	return mix(16) | mix(8) | mix(0)
}

// A menu with items its app draws loses Windows 11's frame: its window is
// square, under the drop shadow of its class (CS_DROPSHADOW) along two
// sides. While the menus show, a hook of the main thread rounds the windows
// of menus as they are created, as Windows rounds its own, outlined in
// their colors, and takes the class's shadow off, for DWM's around the
// rounded window.

const (
	whCallWndProc = 4
	wmCreate      = 0x0001
	gclStyle      = ^uintptr(25) // -26
	gcwAtom       = ^uintptr(31) // -32
	menuClassAtom = 0x8000       // #32768
	csDropShadow  = 0x00020000

	dwmwcpRoundSmall = 3
	dwmwaBorderColor = 34
	dwmColorDefault  = 0xFFFFFFFF
)

var (
	procSetWindowsHookExW   = user32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx      = user32.NewProc("CallNextHookEx")
	procGetClassLongPtrW    = user32.NewProc("GetClassLongPtrW")
	procSetClassLongPtrW    = user32.NewProc("SetClassLongPtrW")

	// menuHookCallback is made once: callbacks are never freed.
	menuHookCallback = sync.OnceValue(func() uintptr { return syscall.NewCallback(menuHook) })
)

// cwpStruct is CWPSTRUCT.
type cwpStruct struct {
	LParam, WParam uintptr
	Message        uint32
	Hwnd           uintptr
}

// hookMenus hooks the main thread's messages while its menus show (on), or
// unhooks them.
func (b *Backend) hookMenus(on bool) {
	switch {
	case !fluentMenus():
	case on && b.menuHook == 0:
		b.menuHook, _, _ = procSetWindowsHookExW.Call(whCallWndProc, menuHookCallback(), 0, uintptr(currentThreadID()))
	case !on && b.menuHook != 0:
		procUnhookWindowsHookEx.Call(b.menuHook)
		b.menuHook = 0
	}
}

func menuHook(code, wp, lp uintptr) uintptr {
	if int32(code) >= 0 {
		if m := (*cwpStruct)(native(lp)); m.Message == wmCreate {
			if atom, _, _ := procGetClassLongPtrW.Call(m.Hwnd, gcwAtom); atom == menuClassAtom {
				roundMenu(m.Hwnd, theBackend.menuPalette().border)
			}
		}
	}
	r, _, _ := procCallNextHookEx.Call(0, code, wp, lp)
	return r
}

// roundMenu gives a menu's window, being created, Windows 11's frame.
func roundMenu(hwnd uintptr, border uint32) {
	pref := int32(dwmwcpRoundSmall)
	procDwmSetWindowAttribute.Call(hwnd, dwmwaWindowCornerPreference, uintptr(unsafe.Pointer(&pref)), 4)
	procDwmSetWindowAttribute.Call(hwnd, dwmwaBorderColor, uintptr(unsafe.Pointer(&border)), 4)
	if style, _, _ := procGetClassLongPtrW.Call(hwnd, gclStyle); style&csDropShadow != 0 {
		procSetClassLongPtrW.Call(hwnd, gclStyle, style&^csDropShadow)
	}
}
