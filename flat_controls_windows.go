package main

import (
	"fmt"
	"math"
	"syscall"
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

type buttonVariant int

const (
	buttonSecondary buttonVariant = iota
	buttonPrimary
	buttonNav
)

type trackMouseEventStruct struct {
	cbSize      uint32
	dwFlags     uint32
	hwndTrack   win.HWND
	dwHoverTime uint32
}

const (
	tmeLeave     = 0x00000002
	wmMouseLeave = 0x02A3
	emSetMargins = 0x00D3
	ecLeftRight  = 0x0003
	lbSetItemH   = 0x01A0
)

var (
	stretchPixels         = windows.NewLazySystemDLL("gdi32.dll").NewProc("StretchDIBits")
	user32DLL             = windows.NewLazySystemDLL("user32.dll")
	trackMouseEventProc   = user32DLL.NewProc("TrackMouseEvent")
	commonControls        = windows.NewLazySystemDLL("comctl32.dll")
	installControlPainter = commonControls.NewProc("SetWindowSubclass")
	defaultControlProc    = commonControls.NewProc("DefSubclassProc")
	controlPaintCallback  = syscall.NewCallback(flatControlProc)
	flatPainters          = map[win.HWND]func(win.HDC){}
	hoverTrackers         = map[win.HWND]bool{}
	hoveredControls       = map[win.HWND]bool{}
)

type desktopStyleConfig struct {
	nameEditor    *walk.LineEdit
	pathField     *walk.LineEdit
	buttonVariant func(*walk.PushButton) buttonVariant
	isNavActive   func(*walk.PushButton) bool
	buttonSurface func(*walk.PushButton) walk.Color
	cards         map[*walk.Composite]bool
	insetFrames   map[*walk.Composite]bool
	segContainers map[*walk.Composite]bool
	statusBadges  map[*walk.CustomWidget]func() int
}

func applyWindowChrome(w walk.Form, captionColor, textColor, borderColor walk.Color) {
	dwm := windows.NewLazySystemDLL("dwmapi.dll")
	setAttribute := dwm.NewProc("DwmSetWindowAttribute")
	if setAttribute.Find() != nil {
		return
	}
	const (
		dwmWindowCornerPreference = 33
		dwmBorderColor            = 34
		dwmCaptionColor           = 35
		dwmTextColor              = 36
	)
	round := uint32(2) // DWMWCP_ROUND
	setAttribute.Call(uintptr(w.Handle()), dwmWindowCornerPreference, uintptr(unsafe.Pointer(&round)), unsafe.Sizeof(round))
	capRef := uint32(captionColor)
	setAttribute.Call(uintptr(w.Handle()), dwmCaptionColor, uintptr(unsafe.Pointer(&capRef)), unsafe.Sizeof(capRef))
	txtRef := uint32(textColor)
	setAttribute.Call(uintptr(w.Handle()), dwmTextColor, uintptr(unsafe.Pointer(&txtRef)), unsafe.Sizeof(txtRef))
	bdrRef := uint32(borderColor)
	setAttribute.Call(uintptr(w.Handle()), dwmBorderColor, uintptr(unsafe.Pointer(&bdrRef)), unsafe.Sizeof(bdrRef))
}

func styleDesktopUI(parent walk.Container, cfg desktopStyleConfig) {
	list := parent.Children()
	for i := 0; i < list.Len(); i++ {
		child := list.At(i)
		var painter func(win.HDC)
		trackHover := false
		switch c := child.(type) {
		case *walk.Composite:
			if cfg.cards != nil && cfg.cards[c] {
				comp := c
				ensureClipChildren(comp.Handle())
				painter = func(dc win.HDC) {
					var r win.RECT
					win.GetClientRect(comp.Handle(), &r)
					radius := float64(comp.IntFrom96DPI(12))
					paintRoundedRect(dc, r, palette.cardColor, palette.cardBorderColor, palette.canvasColor, radius, 1.0)
				}
			} else if cfg.insetFrames != nil && cfg.insetFrames[c] {
				comp := c
				ensureClipChildren(comp.Handle())
				painter = func(dc win.HDC) {
					var r win.RECT
					win.GetClientRect(comp.Handle(), &r)
					radius := float64(comp.IntFrom96DPI(8))
					paintRoundedRect(dc, r, palette.cardColor, palette.cardBorderColor, palette.cardColor, radius, 1.0)
				}
			} else if cfg.segContainers != nil && cfg.segContainers[c] {
				comp := c
				ensureClipChildren(comp.Handle())
				painter = func(dc win.HDC) {
					var r win.RECT
					win.GetClientRect(comp.Handle(), &r)
					radius := float64(comp.IntFrom96DPI(10))
					paintRoundedRect(dc, r, palette.segTrackColor, palette.segTrackColor, palette.canvasColor, radius, 0)
				}
			}
			styleDesktopUI(c, cfg)
		case *walk.ScrollView:
			styleDesktopUI(c, cfg)
		case *walk.CustomWidget:
			if cfg.statusBadges != nil {
				if countFn, ok := cfg.statusBadges[c]; ok && countFn != nil {
					cw := c
					painter = func(dc win.HDC) {
						drawStatusBadge(dc, cw, countFn())
					}
				}
			}
		case *walk.PushButton:
			btn := c
			trackHover = true
			painter = func(dc win.HDC) {
				variant := buttonSecondary
				if cfg.buttonVariant != nil {
					variant = cfg.buttonVariant(btn)
				}
				active := false
				if cfg.isNavActive != nil {
					active = cfg.isNavActive(btn)
				}
				bgSurface := palette.cardColor
				if cfg.buttonSurface != nil {
					bgSurface = cfg.buttonSurface(btn)
				}
				drawModernButton(dc, btn, variant, active, bgSurface)
			}
		case *walk.ProgressBar:
			bar := c
			painter = func(dc win.HDC) {
				var r win.RECT
				win.GetClientRect(bar.Handle(), &r)
				radius := float64(r.Bottom-r.Top) / 2.0
				track := palette.segTrackColor
				paintRoundedRect(dc, r, track, track, palette.cardColor, radius, 0)
				fillW := r.Right * int32(bar.Value()) / 100
				if fillW > 0 {
					fr := r
					fr.Right = fillW
					if fr.Right < fr.Bottom {
						fr.Right = fr.Bottom
					}
					paintRoundedRect(dc, fr, palette.accentColor, palette.accentColor, track, radius, 0)
				}
			}
		case *walk.LineEdit:
			if c == cfg.pathField {
				stripBorder(c.Handle())
				installControlPainter.Call(uintptr(c.Handle()), controlPaintCallback, 1, 0)
				update := func() {
					if c.Focused() {
						delete(flatPainters, c.Handle())
					} else {
						flatPainters[c.Handle()] = func(dc win.HDC) { drawPathText(dc, c) }
					}
					c.Invalidate()
				}
				c.FocusedChanged().Attach(update)
				update()
				continue
			}
			var wantsEscape uintptr
			if c == cfg.nameEditor {
				wantsEscape = 1
			}
			installControlPainter.Call(uintptr(c.Handle()), editFrameProc, 4, wantsEscape)
			pad := uintptr(c.IntFrom96DPI(8))
			win.SendMessage(c.Handle(), emSetMargins, ecLeftRight, (pad<<16)|pad)
		case *walk.ListBox:
			stripBorder(c.Handle())
			itemH := uintptr(c.IntFrom96DPI(28))
			win.SendMessage(c.Handle(), lbSetItemH, 0, itemH)
		}
		if painter != nil {
			if ok, _, _ := installControlPainter.Call(uintptr(child.Handle()), controlPaintCallback, 1, 0); ok != 0 {
				flatPainters[child.Handle()] = painter
				if trackHover {
					hoverTrackers[child.Handle()] = true
				}
			}
		}
	}
}

func ensureClipChildren(hwnd win.HWND) {
	style := win.GetWindowLong(hwnd, win.GWL_STYLE)
	if style&win.WS_CLIPCHILDREN == 0 {
		win.SetWindowLong(hwnd, win.GWL_STYLE, style|win.WS_CLIPCHILDREN)
	}
}

func stripBorder(hwnd win.HWND) {
	style := win.GetWindowLong(hwnd, win.GWL_STYLE)
	if style&win.WS_BORDER != 0 {
		win.SetWindowLong(hwnd, win.GWL_STYLE, style&^win.WS_BORDER)
	}
	exStyle := win.GetWindowLong(hwnd, win.GWL_EXSTYLE)
	if exStyle&win.WS_EX_CLIENTEDGE != 0 {
		win.SetWindowLong(hwnd, win.GWL_EXSTYLE, exStyle&^win.WS_EX_CLIENTEDGE)
	}
	win.SetWindowPos(hwnd, 0, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_NOZORDER|win.SWP_FRAMECHANGED)
}

func flatControlProc(hwnd win.HWND, msg uint32, w, l, id, data uintptr) uintptr {
	paint := flatPainters[hwnd]
	if paint != nil {
		switch msg {
		case win.WM_PAINT:
			var ps win.PAINTSTRUCT
			dc := win.BeginPaint(hwnd, &ps)
			paint(dc)
			win.EndPaint(hwnd, &ps)
			return 0
		case win.WM_PRINT, win.WM_PRINTCLIENT:
			paint(win.HDC(w))
			return 0
		case win.WM_ERASEBKGND:
			paint(win.HDC(w))
			return 1
		case win.WM_MOUSEMOVE:
			if hoverTrackers[hwnd] && !hoveredControls[hwnd] {
				hoveredControls[hwnd] = true
				tme := trackMouseEventStruct{
					cbSize:    uint32(unsafe.Sizeof(trackMouseEventStruct{})),
					dwFlags:   tmeLeave,
					hwndTrack: hwnd,
				}
				trackMouseEventProc.Call(uintptr(unsafe.Pointer(&tme)))
				win.InvalidateRect(hwnd, nil, false)
			}
		case wmMouseLeave:
			if hoveredControls[hwnd] {
				delete(hoveredControls, hwnd)
				win.InvalidateRect(hwnd, nil, false)
			}
		case win.WM_NCDESTROY:
			delete(flatPainters, hwnd)
			delete(hoverTrackers, hwnd)
			delete(hoveredControls, hwnd)
		}
	}
	result, _, _ := defaultControlProc.Call(uintptr(hwnd), uintptr(msg), w, l)
	switch msg {
	case win.WM_ENABLE, win.WM_SETFOCUS, win.WM_KILLFOCUS, win.WM_UPDATEUISTATE,
		win.WM_LBUTTONDOWN, win.WM_LBUTTONUP, win.WM_KEYDOWN, win.WM_KEYUP,
		win.PBM_SETPOS, win.WM_SETTEXT, win.WM_SIZE:
		win.InvalidateRect(hwnd, nil, false)
	}
	return result
}

func paintSolidRect(dc win.HDC, r win.RECT, color walk.Color) {
	if r.Right <= r.Left || r.Bottom <= r.Top {
		return
	}
	brush := win.CreateBrushIndirect(&win.LOGBRUSH{LbColor: win.COLORREF(color)})
	old := win.SelectObject(dc, win.HGDIOBJ(brush))
	pen := win.SelectObject(dc, win.GetStockObject(win.NULL_PEN))
	win.Rectangle_(dc, r.Left, r.Top, r.Right+1, r.Bottom+1)
	win.SelectObject(dc, pen)
	win.SelectObject(dc, old)
	win.DeleteObject(win.HGDIOBJ(brush))
}

// paintRoundedRect renders a crisp anti-aliased rounded rectangle with an optional 1px hairline border.
func paintRoundedRect(dc win.HDC, r win.RECT, fill, border, background walk.Color, radius, borderWidth float64) {
	width, height := r.Right-r.Left, r.Bottom-r.Top
	if width <= 0 || height <= 0 {
		return
	}
	if radius <= 0 && borderWidth <= 0 {
		paintSolidRect(dc, r, fill)
		return
	}
	maxR := math.Min(float64(width), float64(height)) / 2.0
	if radius > maxR {
		radius = maxR
	}
	pixels := make([]uint32, int(width)*int(height))
	solidFill := dibColor(fill)
	for i := range pixels {
		pixels[i] = solidFill
	}

	hx := float64(width) / 2.0
	hy := float64(height) / 2.0
	ix := hx - radius
	iy := hy - radius
	band := int32(math.Ceil(radius + borderWidth + 2.0))

	for y := int32(0); y < height; y++ {
		nearY := y < band || y >= height-band
		py := math.Abs(float64(y)+0.5-hy) - iy
		for x := int32(0); x < width; x++ {
			if !nearY && x >= band && x < width-band {
				x = width - band - 1
				continue
			}
			px := math.Abs(float64(x)+0.5-hx) - ix
			ox := math.Max(px, 0)
			oy := math.Max(py, 0)
			d := math.Hypot(ox, oy) + math.Min(math.Max(px, py), 0) - radius

			outerAlpha := clamp01(0.5 - d)
			if outerAlpha <= 0 {
				pixels[y*width+x] = dibColor(background)
				continue
			}
			var c walk.Color
			if borderWidth > 0 {
				innerAlpha := clamp01(0.5 - (d + borderWidth))
				if innerAlpha >= 1 {
					continue
				}
				c = blendCoverage(background, border, outerAlpha)
				if innerAlpha > 0 {
					c = blendCoverage(c, fill, innerAlpha)
				}
			} else {
				if outerAlpha >= 1 {
					continue
				}
				c = blendCoverage(background, fill, outerAlpha)
			}
			pixels[y*width+x] = dibColor(c)
		}
	}

	info := win.BITMAPINFO{}
	info.BmiHeader = win.BITMAPINFOHEADER{
		BiSize:     uint32(unsafe.Sizeof(info.BmiHeader)),
		BiWidth:    width,
		BiHeight:   -height,
		BiPlanes:   1,
		BiBitCount: 32,
	}
	stretchPixels.Call(
		uintptr(dc),
		uintptr(r.Left), uintptr(r.Top), uintptr(width), uintptr(height),
		0, 0, uintptr(width), uintptr(height),
		uintptr(unsafe.Pointer(&pixels[0])),
		uintptr(unsafe.Pointer(&info)),
		0, win.SRCCOPY,
	)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func dibColor(c walk.Color) uint32 {
	return uint32(c&255)<<16 | uint32(c&0xff00) | uint32(c>>16)&255
}

func blendCoverage(base, ink walk.Color, alpha float64) walk.Color {
	var result uint32
	for shift := 0; shift < 24; shift += 8 {
		a, b := float64((base>>shift)&255), float64((ink>>shift)&255)
		result |= uint32(math.Round(a+(b-a)*alpha)) << shift
	}
	return walk.Color(result)
}

func drawModernButton(dc win.HDC, b *walk.PushButton, variant buttonVariant, navActive bool, surface walk.Color) {
	saved := win.SaveDC(dc)
	defer win.RestoreDC(dc, saved)

	var full win.RECT
	win.GetClientRect(b.Handle(), &full)
	paintSolidRect(dc, full, surface)

	r := full
	height := int32(b.IntFrom96DPI(b.MaxSize().Height))
	if height > 0 && height < r.Bottom {
		r.Top = (r.Bottom - height) / 2
		r.Bottom = r.Top + height
	}

	enabled := b.Enabled()
	pressed := enabled && (win.SendMessage(b.Handle(), win.BM_GETSTATE, 0, 0)&win.BST_PUSHED != 0)
	hovered := enabled && hoveredControls[b.Handle()]

	var bg, border, fg walk.Color
	borderW := 1.0
	radius := float64(b.IntFrom96DPI(8))

	switch variant {
	case buttonNav:
		radius = float64(b.IntFrom96DPI(7))
		if navActive {
			bg, border, fg = palette.cardColor, palette.cardBorderColor, palette.inkPrimary
			borderW = 1.0
		} else if pressed {
			bg, border, fg = palette.btnSecondaryPressed, palette.btnSecondaryPressed, palette.inkPrimary
			borderW = 0
		} else if hovered {
			bg, border, fg = palette.btnSecondaryHover, palette.btnSecondaryHover, palette.inkPrimary
			borderW = 0
		} else {
			bg, border, fg = surface, surface, palette.inkSecondary
			borderW = 0
		}
	case buttonPrimary:
		borderW = 0
		if !enabled {
			bg, border, fg = palette.btnDisabledBg, palette.btnDisabledBorder, palette.inkMuted
			borderW = 1.0
		} else if pressed {
			bg, border, fg = palette.actionPressed, palette.actionPressed, readableInk(palette.actionPressed)
		} else if hovered {
			bg, border, fg = palette.actionHover, palette.actionHover, readableInk(palette.actionHover)
		} else {
			bg, border, fg = palette.actionColor, palette.actionColor, readableInk(palette.actionColor)
		}
	default: // buttonSecondary
		if !enabled {
			bg, border, fg = palette.btnDisabledBg, palette.btnDisabledBorder, palette.inkMuted
		} else if pressed {
			bg, border, fg = palette.btnSecondaryPressed, palette.btnSecondaryBorder, palette.inkPrimary
		} else if hovered {
			bg, border, fg = palette.btnSecondaryHover, palette.btnSecondaryBorder, palette.inkPrimary
		} else {
			bg, border, fg = palette.btnSecondaryBg, palette.btnSecondaryBorder, palette.inkPrimary
		}
	}

	if win.GetFocus() == b.Handle() && win.SendMessage(b.Handle(), win.WM_QUERYUISTATE, 0, 0)&1 == 0 {
		paintRoundedRect(dc, r, palette.accentFocusRing, palette.accentFocusRing, surface, radius, 0)
		fr := r
		fr.Left += 2
		fr.Top += 2
		fr.Right -= 2
		fr.Bottom -= 2
		paintRoundedRect(dc, fr, bg, border, palette.accentFocusRing, math.Max(2, radius-2), borderW)
	} else {
		paintRoundedRect(dc, r, bg, border, surface, radius, borderW)
	}

	win.SelectObject(dc, win.HGDIOBJ(win.SendMessage(b.Handle(), win.WM_GETFONT, 0, 0)))
	win.SetBkMode(dc, win.TRANSPARENT)
	win.SetTextColor(dc, win.COLORREF(fg))
	label, _ := windows.UTF16FromString(b.Text())
	win.DrawTextEx(dc, (*uint16)(unsafe.Pointer(&label[0])), int32(len(label)-1), &r, win.DT_CENTER|win.DT_VCENTER|win.DT_SINGLELINE|win.DT_NOPREFIX, nil)
}

func drawStatusBadge(dc win.HDC, comp *walk.CustomWidget, count int) {
	saved := win.SaveDC(dc)
	defer win.RestoreDC(dc, saved)

	var r win.RECT
	win.GetClientRect(comp.Handle(), &r)
	paintSolidRect(dc, r, palette.headerColor)

	text := fmt.Sprintf("已连接 %d 台设备", count)
	if count < 0 {
		text = "已连接设备：—"
	}
	bg := palette.segTrackColor
	border := palette.cardBorderColor
	dot := palette.inkMuted
	fg := palette.inkSecondary
	if count > 0 {
		bg = blendCoverage(palette.cardColor, palette.accentColor, .16)
		border = palette.accentFocusRing
		dot = palette.accentColor
		fg = palette.inkPrimary
	}

	hfont := win.HFONT(win.SendMessage(comp.Handle(), win.WM_GETFONT, 0, 0))
	var createdFont win.HFONT
	if hfont == 0 {
		lf := win.LOGFONT{
			LfHeight:  -int32(comp.IntFrom96DPI(12)),
			LfWeight:  win.FW_NORMAL,
			LfCharSet: win.DEFAULT_CHARSET,
		}
		face, _ := windows.UTF16FromString(chineseUIFont)
		copy(lf.LfFaceName[:], face)
		createdFont = win.CreateFontIndirect(&lf)
		hfont = createdFont
	}
	if hfont != 0 {
		win.SelectObject(dc, win.HGDIOBJ(hfont))
	}
	if createdFont != 0 {
		defer win.DeleteObject(win.HGDIOBJ(createdFont))
	}
	win.SetBkMode(dc, win.TRANSPARENT)

	utf16Text, _ := windows.UTF16FromString(text)
	var textRect win.RECT
	win.DrawTextEx(dc, (*uint16)(unsafe.Pointer(&utf16Text[0])), int32(len(utf16Text)-1), &textRect, win.DT_CALCRECT|win.DT_SINGLELINE|win.DT_NOPREFIX, nil)

	padX := int32(comp.IntFrom96DPI(11))
	dotSize := int32(comp.IntFrom96DPI(7))
	dotGap := int32(comp.IntFrom96DPI(6))
	pillH := int32(comp.IntFrom96DPI(26))
	if pillH > r.Bottom-r.Top {
		pillH = r.Bottom - r.Top
	}
	pillW := padX + dotSize + dotGap + (textRect.Right - textRect.Left) + padX
	if pillW > r.Right-r.Left {
		pillW = r.Right - r.Left
	}

	pr := win.RECT{
		Left:   r.Right - pillW,
		Top:    (r.Bottom - pillH) / 2,
		Right:  r.Right,
		Bottom: (r.Bottom-pillH)/2 + pillH,
	}
	paintRoundedRect(dc, pr, bg, border, palette.headerColor, float64(pillH)/2.0, 1.0)

	dr := win.RECT{
		Left:   pr.Left + padX,
		Top:    pr.Top + (pillH-dotSize)/2,
		Right:  pr.Left + padX + dotSize,
		Bottom: pr.Top + (pillH-dotSize)/2 + dotSize,
	}
	paintRoundedRect(dc, dr, dot, dot, bg, float64(dotSize)/2.0, 0)

	tr := win.RECT{
		Left:   dr.Right + dotGap,
		Top:    pr.Top,
		Right:  pr.Right - padX + 2,
		Bottom: pr.Bottom,
	}
	win.SetTextColor(dc, win.COLORREF(fg))
	win.DrawTextEx(dc, (*uint16)(unsafe.Pointer(&utf16Text[0])), int32(len(utf16Text)-1), &tr, win.DT_LEFT|win.DT_VCENTER|win.DT_SINGLELINE|win.DT_NOPREFIX, nil)
}

func drawPathText(dc win.HDC, field *walk.LineEdit) {
	saved := win.SaveDC(dc)
	defer win.RestoreDC(dc, saved)
	var bounds win.RECT
	win.GetClientRect(field.Handle(), &bounds)
	paintSolidRect(dc, bounds, palette.cardColor)
	win.SelectObject(dc, win.HGDIOBJ(win.SendMessage(field.Handle(), win.WM_GETFONT, 0, 0)))
	win.SetBkMode(dc, win.TRANSPARENT)
	win.SetTextColor(dc, win.COLORREF(palette.inkPrimary))
	path, _ := windows.UTF16FromString(field.Text())
	if len(path) > 1 {
		win.DrawTextEx(dc, &path[0], int32(len(path)-1), &bounds, win.DT_PATH_ELLIPSIS|win.DT_END_ELLIPSIS|win.DT_NOPREFIX|win.DT_VCENTER|win.DT_SINGLELINE, nil)
	}
}

func arrangeDesktopChildren(container walk.ContainerLayoutItem, size walk.Size) {
	container.Geometry().ClientSize = size
	for _, child := range container.PerformLayout() {
		b := child.Bounds
		if hwnd := child.Item.Handle(); hwnd != 0 {
			win.MoveWindow(hwnd, int32(b.X), int32(b.Y), int32(b.Width), int32(b.Height), false)
		}
		if inner, ok := child.Item.(walk.ContainerLayoutItem); ok {
			arrangeDesktopChildren(inner, b.Size())
		}
	}
}
