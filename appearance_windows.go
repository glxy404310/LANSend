package main

import (
	"math"
	"syscall"
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

type desktopPalette struct {
	dark, highContrast                                                                                           bool
	canvasColor, cardColor, cardBorderColor, headerColor, dividerColor, segTrackColor                            walk.Color
	inkPrimary, inkSecondary, inkMuted                                                                           walk.Color
	accentColor, actionColor, actionHover, actionPressed, accentFocusRing                                        walk.Color
	btnSecondaryBg, btnSecondaryHover, btnSecondaryPressed, btnSecondaryBorder, btnDisabledBg, btnDisabledBorder walk.Color
}

var palette = readDesktopPalette()

func readableInk(bg walk.Color) walk.Color {
	channel := func(v byte) float64 {
		x := float64(v) / 255
		if x <= .04045 {
			return x / 12.92
		}
		return math.Pow((x+.055)/1.055, 2.4)
	}
	light := .2126*channel(byte(bg)) + .7152*channel(byte(bg>>8)) + .0722*channel(byte(bg>>16))
	if light > .179 {
		return 0
	}
	return 0xffffff
}
func registryColorSetting(path, name string) (uint32, bool) {
	key, e := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if e != nil {
		return 0, false
	}
	defer key.Close()
	v, _, e := key.GetIntegerValue(name)
	return uint32(v), e == nil
}
func readDesktopPalette() desktopPalette {
	var hc win.HIGHCONTRAST
	hc.CbSize = uint32(unsafe.Sizeof(hc))
	contrast := win.SystemParametersInfo(win.SPI_GETHIGHCONTRAST, hc.CbSize, unsafe.Pointer(&hc), 0) && hc.DwFlags&win.HCF_HIGHCONTRASTON != 0
	light, known := registryColorSetting(`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, "AppsUseLightTheme")
	dark := known && light == 0 && !contrast
	face := walk.Color(win.GetSysColor(win.COLOR_BTNFACE))
	card := walk.Color(win.GetSysColor(win.COLOR_WINDOW))
	ink := walk.Color(win.GetSysColor(win.COLOR_WINDOWTEXT))
	accent := walk.Color(win.GetSysColor(win.COLOR_HIGHLIGHT))
	if dark {
		face = walk.RGB(31, 31, 31)
		card = walk.RGB(44, 44, 44)
		ink = walk.RGB(245, 245, 245)
	}
	if !contrast {
		if v, ok := registryColorSetting(`Software\Microsoft\Windows\DWM`, "AccentColor"); ok {
			accent = walk.Color(v & 0xffffff)
		}
	}
	border := blendCoverage(card, ink, .24)
	secondary := blendCoverage(card, ink, .73)
	muted := blendCoverage(card, ink, .58)
	if contrast {
		border = ink
		secondary = ink
		muted = walk.Color(win.GetSysColor(win.COLOR_GRAYTEXT))
	}
	action := walk.RGB(0, 103, 192)
	if dark {
		action = walk.RGB(96, 177, 255)
	}
	if contrast {
		action = accent
	}
	return desktopPalette{dark: dark, highContrast: contrast, canvasColor: face, cardColor: card, headerColor: card, cardBorderColor: border, dividerColor: border,
		segTrackColor: face, inkPrimary: ink, inkSecondary: secondary, inkMuted: muted, accentColor: accent, actionColor: action, actionHover: blendCoverage(action, ink, .10), actionPressed: blendCoverage(action, ink, .21), accentFocusRing: accent,
		btnSecondaryBg: card, btnSecondaryHover: blendCoverage(card, ink, .08), btnSecondaryPressed: blendCoverage(card, ink, .15), btnSecondaryBorder: border, btnDisabledBg: face, btnDisabledBorder: border}
}

// Retain the semantic role when Walk creates a native brush from the declaration.
type themeBrush struct{ role *walk.Color }
type boundBrush struct {
	*walk.SolidColorBrush
	role *walk.Color
}

func (b themeBrush) Create() (walk.Brush, error) {
	brush, e := walk.NewSolidColorBrush(*b.role)
	if e != nil {
		return nil, e
	}
	return &boundBrush{brush, b.role}, nil
}

var (
	appearanceProc    = syscall.NewCallback(desktopAppearanceProc)
	appearanceUpdates = map[win.HWND]func(){}
)

const refreshAppearanceMessage = win.WM_APP + 403

func desktopAppearanceProc(hwnd win.HWND, msg uint32, w, l, id, data uintptr) uintptr {
	result, _, _ := defaultControlProc.Call(uintptr(hwnd), uintptr(msg), w, l)
	switch msg {
	case win.WM_SETTINGCHANGE, win.WM_THEMECHANGED, win.WM_SYSCOLORCHANGE, 0x0320: // WM_DWMCOLORIZATIONCOLORCHANGED
		win.PostMessage(hwnd, refreshAppearanceMessage, 0, 0)
	case refreshAppearanceMessage:
		if update := appearanceUpdates[hwnd]; update != nil {
			update()
		}
	case win.WM_NCDESTROY:
		delete(appearanceUpdates, hwnd)
	}
	return result
}
func redrawDesktop(w walk.Form) {
	win.RedrawWindow(w.Handle(), nil, 0, win.RDW_INVALIDATE|win.RDW_ERASE|win.RDW_ALLCHILDREN|win.RDW_UPDATENOW)
}

func watchDesktopAppearance(w walk.Form) {
	var updates []func()
	var bind func(walk.Window)
	bind = func(c walk.Window) {
		if b, ok := c.Background().(*boundBrush); ok {
			role := b.role
			updates = append(updates, func() {
				if brush, e := (themeBrush{role}).Create(); e == nil {
					c.SetBackground(brush)
				}
			})
		}
		if field, ok := c.(interface {
			TextColor() walk.Color
			SetTextColor(walk.Color)
		}); ok {
			secondary := field.TextColor() == palette.inkSecondary
			updates = append(updates, func() {
				if secondary {
					field.SetTextColor(palette.inkSecondary)
				} else {
					field.SetTextColor(palette.inkPrimary)
				}
			})
		}
		switch field := c.(type) {
		case *walk.ComboBox:
			installControlPainter.Call(uintptr(field.Handle()), controlPaintCallback, 1, 0)
			updates = append(updates, func() {
				if palette.dark && !field.Editable() {
					flatPainters[field.Handle()] = func(dc win.HDC) { drawDarkPicker(dc, field) }
				} else {
					delete(flatPainters, field.Handle())
				}
			})
		}
		switch c.(type) {
		case *walk.LineEdit, *walk.ListBox, *walk.ComboBox:
			inputWindows[c.Handle()] = true
		}
		if _, container := c.(walk.Container); container || inputWindows[c.Handle()] {
			installControlPainter.Call(uintptr(c.Handle()), controlColorsProc, 3, 0)
		}
		switch c.(type) {
		case *walk.ListBox, *walk.LineEdit, *walk.ComboBox, *walk.CheckBox:
			updates = append(updates, func() {
				var theme *uint16
				if palette.dark {
					theme = syscall.StringToUTF16Ptr("DarkMode_Explorer")
				}
				win.SetWindowTheme(c.Handle(), theme, nil)
			})
		}
		if container, ok := c.(walk.Container); ok {
			children := container.Children()
			for i := 0; i < children.Len(); i++ {
				bind(children.At(i))
			}
		}
	}
	bind(w)
	apply := func() {
		for _, update := range updates {
			update()
		}
		applyWindowChrome(w, palette.headerColor, palette.inkPrimary, palette.cardBorderColor)
		redrawDesktop(w)
	}
	applied := palette
	appearanceUpdates[w.Handle()] = func() {
		p := readDesktopPalette()
		if p != applied {
			applied = p
			palette = p
			apply()
		}
	}
	installControlPainter.Call(uintptr(w.Handle()), appearanceProc, 2, 0)
	apply()
}

// Native edit and list controls request their colors from the parent window.
var inputWindows = map[win.HWND]bool{}
var controlColorsProc = syscall.NewCallback(inputColorProc)
var setDCBrushColor = windows.NewLazySystemDLL("gdi32.dll").NewProc("SetDCBrushColor")

func inputColorProc(hwnd win.HWND, msg uint32, w, l, id, data uintptr) uintptr {
	if palette.dark && (msg == win.WM_CTLCOLOREDIT || msg == win.WM_CTLCOLORLISTBOX || (msg == win.WM_CTLCOLORSTATIC && inputWindows[win.HWND(l)])) {
		dc := win.HDC(w)
		bg := palette.canvasColor
		if msg == win.WM_CTLCOLORSTATIC && win.GetWindowLong(win.HWND(l), win.GWL_STYLE)&win.ES_READONLY != 0 {
			bg = palette.cardColor
		}
		fg := palette.inkPrimary
		if !win.IsWindowEnabled(win.HWND(l)) {
			fg = palette.inkMuted
		}
		win.SetBkColor(dc, win.COLORREF(bg))
		win.SetTextColor(dc, win.COLORREF(fg))
		setDCBrushColor.Call(w, uintptr(bg))
		return uintptr(win.GetStockObject(win.DC_BRUSH))
	}
	if msg == win.WM_NCDESTROY {
		delete(inputWindows, hwnd)
	}
	result, _, _ := defaultControlProc.Call(uintptr(hwnd), uintptr(msg), w, l)
	return result
}

func drawDarkPicker(dc win.HDC, picker *walk.ComboBox) {
	saved := win.SaveDC(dc)
	defer win.RestoreDC(dc, saved)
	var r win.RECT
	win.GetClientRect(picker.Handle(), &r)
	border := palette.cardBorderColor
	if win.GetFocus() == picker.Handle() {
		border = palette.accentFocusRing
	}
	paintRoundedRect(dc, r, palette.canvasColor, border, palette.cardColor, float64(picker.IntFrom96DPI(4)), 1)
	win.SelectObject(dc, win.HGDIOBJ(win.SendMessage(picker.Handle(), win.WM_GETFONT, 0, 0)))
	win.SetBkMode(dc, win.TRANSPARENT)
	win.SetTextColor(dc, win.COLORREF(palette.inkPrimary))
	arrow := r
	arrow.Left = r.Right - int32(picker.IntFrom96DPI(24))
	win.DrawTextEx(dc, syscall.StringToUTF16Ptr("⌄"), -1, &arrow, win.DT_CENTER|win.DT_VCENTER|win.DT_SINGLELINE, nil)
	r.Right = arrow.Left
	r.Left += int32(picker.IntFrom96DPI(6))
	text, _ := windows.UTF16FromString(picker.Text())
	if len(text) > 0 {
		win.DrawTextEx(dc, &text[0], int32(len(text)-1), &r, win.DT_VCENTER|win.DT_SINGLELINE|win.DT_END_ELLIPSIS|win.DT_NOPREFIX, nil)
	}
}

var editFrameProc = syscall.NewCallback(editFrameWindowProc)
var getWindowDC = user32DLL.NewProc("GetWindowDC")

func editFrameWindowProc(hwnd win.HWND, msg uint32, w, l, id, data uintptr) uintptr {
	result, _, _ := defaultControlProc.Call(uintptr(hwnd), uintptr(msg), w, l)
	if msg == win.WM_GETDLGCODE && w == win.VK_ESCAPE && data != 0 {
		return result | win.DLGC_WANTMESSAGE
	}
	if !palette.dark {
		return result
	}
	switch msg {
	case win.WM_SETFOCUS, win.WM_KILLFOCUS:
		win.RedrawWindow(hwnd, nil, 0, win.RDW_INVALIDATE|win.RDW_FRAME)
	case win.WM_NCPAINT, win.WM_PRINT:
		var dc win.HDC
		if msg == win.WM_PRINT {
			if l&win.PRF_NONCLIENT == 0 {
				return result
			}
			dc = win.HDC(w)
		} else {
			handle, _, _ := getWindowDC.Call(uintptr(hwnd))
			dc = win.HDC(handle)
			if dc == 0 {
				return result
			}
			defer win.ReleaseDC(hwnd, dc)
		}
		var outer, client win.RECT
		var origin win.POINT
		win.GetWindowRect(hwnd, &outer)
		win.GetClientRect(hwnd, &client)
		win.ClientToScreen(hwnd, &origin)
		x, y := origin.X-outer.Left, origin.Y-outer.Top
		saved := win.SaveDC(dc)
		defer win.RestoreDC(dc, saved)
		win.ExcludeClipRect(dc, x, y, x+client.Right, y+client.Bottom)
		bounds := win.RECT{Right: outer.Right - outer.Left, Bottom: outer.Bottom - outer.Top}
		border := palette.cardBorderColor
		if win.GetFocus() == hwnd {
			border = palette.accentFocusRing
		}
		paintSolidRect(dc, bounds, border)
		bounds.Left++
		bounds.Top++
		bounds.Right--
		bounds.Bottom--
		paintSolidRect(dc, bounds, palette.canvasColor)
	}
	return result
}
