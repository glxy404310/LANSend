package main

import (
	"math"
	"syscall"
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

var surfaceColor = walk.RGB(248, 249, 247)
var (
	stretchPixels         = windows.NewLazySystemDLL("gdi32.dll").NewProc("StretchDIBits")
	commonControls        = windows.NewLazySystemDLL("comctl32.dll")
	installControlPainter = commonControls.NewProc("SetWindowSubclass")
	defaultControlProc    = commonControls.NewProc("DefSubclassProc")
	controlPaintCallback  = syscall.NewCallback(flatControlProc)
	flatPainters          = map[win.HWND]func(win.HDC){}
)

func styleDesktopControls(parent walk.Container, primary func(*walk.PushButton) bool) {
	list := parent.Children()
	for i := 0; i < list.Len(); i++ {
		child := list.At(i)
		var painter func(win.HDC)
		switch c := child.(type) {
		case *walk.Composite:
			styleDesktopControls(c, primary)
		case *walk.PushButton:
			painter = func(dc win.HDC) { drawFlatButton(dc, c, primary(c)) }
		case *walk.ProgressBar:
			painter = func(dc win.HDC) {
				var r win.RECT
				win.GetClientRect(c.Handle(), &r)
				paintShape(dc, r, surfaceColor, surfaceColor, 0)
				paintShape(dc, r, walk.RGB(216, 226, 221), surfaceColor, r.Bottom)
				r.Right = r.Right * int32(c.Value()) / 100
				if r.Right > 0 {
					paintShape(dc, r, walk.RGB(27, 104, 90), walk.RGB(216, 226, 221), r.Bottom)
				}
			}
		}
		if painter != nil {
			if ok, _, _ := installControlPainter.Call(uintptr(child.Handle()), controlPaintCallback, 1, 0); ok != 0 {
				flatPainters[child.Handle()] = painter
			}
		}
	}
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
			return 1
		case win.WM_NCDESTROY:
			delete(flatPainters, hwnd)
		}
	}
	result, _, _ := defaultControlProc.Call(uintptr(hwnd), uintptr(msg), w, l)
	switch msg {
	case win.WM_ENABLE, win.WM_SETFOCUS, win.WM_KILLFOCUS, win.WM_UPDATEUISTATE, win.WM_LBUTTONDOWN, win.WM_LBUTTONUP, win.WM_KEYDOWN, win.WM_KEYUP, win.PBM_SETPOS:
		win.InvalidateRect(hwnd, nil, false)
	}
	return result
}

func paintShape(dc win.HDC, r win.RECT, color, background walk.Color, diameter int32) {
	width, height := r.Right-r.Left, r.Bottom-r.Top
	if width <= 0 || height <= 0 {
		return
	}
	if diameter == 0 {
		brush := win.CreateBrushIndirect(&win.LOGBRUSH{LbColor: win.COLORREF(color)})
		old := win.SelectObject(dc, win.HGDIOBJ(brush))
		pen := win.SelectObject(dc, win.GetStockObject(win.NULL_PEN))
		win.Rectangle_(dc, r.Left, r.Top, r.Right+1, r.Bottom+1)
		win.SelectObject(dc, pen)
		win.SelectObject(dc, old)
		win.DeleteObject(win.HGDIOBJ(brush))
		return
	}
	// A top-down DIB paints the face and its coverage-weighted edges together.
	pixels := make([]uint32, int(width)*int(height))
	solid := dibColor(color)
	for i := range pixels {
		pixels[i] = solid
	}
	radius := float64(min(diameter, width, height)) / 2
	for y := int32(0); float64(y) < radius; y++ {
		for x := int32(0); float64(x) < radius; x++ {
			coverage := radius + 0.5 - math.Hypot(radius-float64(x)-0.5, radius-float64(y)-0.5)
			if coverage >= 1 {
				break
			}
			shade := dibColor(blendCoverage(background, color, max(0, coverage)))
			for _, px := range [2]int32{x, width - 1 - x} {
				pixels[y*width+px] = shade
				pixels[(height-1-y)*width+px] = shade
			}
		}
	}
	info := win.BITMAPINFO{}
	info.BmiHeader = win.BITMAPINFOHEADER{BiSize: uint32(unsafe.Sizeof(info.BmiHeader)), BiWidth: width, BiHeight: -height, BiPlanes: 1, BiBitCount: 32}
	stretchPixels.Call(uintptr(dc), uintptr(r.Left), uintptr(r.Top), uintptr(width), uintptr(height), 0, 0, uintptr(width), uintptr(height), uintptr(unsafe.Pointer(&pixels[0])), uintptr(unsafe.Pointer(&info)), 0, win.SRCCOPY)

}

func dibColor(c walk.Color) uint32 { return uint32(c&255)<<16 | uint32(c&0xff00) | uint32(c>>16)&255 }

func blendCoverage(base, ink walk.Color, alpha float64) walk.Color {
	var result uint32
	for shift := 0; shift < 24; shift += 8 {
		a, b := float64((base>>shift)&255), float64((ink>>shift)&255)
		result |= uint32(math.Round(a+(b-a)*alpha)) << shift
	}
	return walk.Color(result)
}

func drawFlatButton(dc win.HDC, b *walk.PushButton, primary bool) {
	saved := win.SaveDC(dc)
	defer win.RestoreDC(dc, saved)
	var r win.RECT
	win.GetClientRect(b.Handle(), &r)
	paintShape(dc, r, surfaceColor, surfaceColor, 0)
	// Walk may give a math-font button a taller native client area than its visible row.
	// Center the painted face in the same row as the single-line caption.
	height := int32(b.IntFrom96DPI(b.MaxSize().Height))
	if height > 0 && height < r.Bottom {
		r.Top = (r.Bottom - height) / 2
		r.Bottom = r.Top + height
	}
	bg, fg := walk.RGB(226, 234, 230), walk.RGB(35, 61, 54)
	if primary {
		bg, fg = walk.RGB(31, 99, 84), walk.RGB(255, 255, 255)
	}
	if !b.Enabled() {
		bg, fg = walk.RGB(236, 239, 236), walk.RGB(130, 138, 133)
	} else if win.SendMessage(b.Handle(), win.BM_GETSTATE, 0, 0)&win.BST_PUSHED != 0 {
		bg, fg = walk.RGB(19, 72, 61), walk.RGB(255, 255, 255)
	}
	diameter := int32(b.IntFrom96DPI(16))
	underlay := surfaceColor
	if win.GetFocus() == b.Handle() && win.SendMessage(b.Handle(), win.WM_QUERYUISTATE, 0, 0)&1 == 0 {
		underlay = walk.RGB(92, 152, 139)
		paintShape(dc, r, underlay, surfaceColor, diameter)
		r.Left += 2
		r.Top += 2
		r.Right -= 2
		r.Bottom -= 2
	}
	paintShape(dc, r, bg, underlay, diameter)
	win.SelectObject(dc, win.HGDIOBJ(win.SendMessage(b.Handle(), win.WM_GETFONT, 0, 0)))
	win.SetBkMode(dc, win.TRANSPARENT)
	win.SetTextColor(dc, win.COLORREF(fg))
	label, _ := windows.UTF16FromString(b.Text())
	win.DrawTextEx(dc, (*uint16)(unsafe.Pointer(&label[0])), int32(len(label)-1), &r, win.DT_CENTER|win.DT_VCENTER|win.DT_SINGLELINE|win.DT_NOPREFIX, nil)
}
