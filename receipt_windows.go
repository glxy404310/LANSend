package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/lxn/walk"
	d "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

func receiptsSilent(root string) bool {
	_, err := os.Stat(filepath.Join(root, "silent-receipts"))
	return !errors.Is(err, os.ErrNotExist)
}
func setReceiptsSilent(root string, silent bool) error {
	path := filepath.Join(root, "silent-receipts")
	if silent {
		return os.WriteFile(path, nil, 0600)
	}
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Keep the message loop in the login-session receiver, independent of the dashboard.
func receive(root string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var w *walk.MainWindow
	var files *walk.ComboBox
	var title *walk.Label
	var openFileBtn *walk.PushButton
	var names []string
	var paths []string
	open := func(path string) {
		target, err := syscall.UTF16PtrFromString(path)
		if err != nil || windows.ShellExecute(windows.Handle(w.Handle()), syscall.StringToUTF16Ptr("open"), target, nil, nil, win.SW_SHOWNORMAL) != nil {
			walk.MsgBox(w, "打开失败", "请确认文件仍在收件夹中，并已设置默认打开程序。", walk.MsgBoxIconWarning)
		}
	}
	ui := d.MainWindow{
		AssignTo:   &w,
		Title:      "LANSend 收件",
		Visible:    false,
		Size:       d.Size{Width: 430, Height: 196},
		MinSize:    d.Size{Width: 380, Height: 180},
		Font:       d.Font{Family: chineseUIFont, PointSize: 10},
		Background: themeBrush{&palette.cardColor},
		Layout:     d.VBox{Margins: d.Margins{Left: 20, Top: 16, Right: 20, Bottom: 18}, Spacing: 12},
		Children: []d.Widget{
			d.Label{MaxSize: d.Size{Height: 28}, AssignTo: &title, Font: d.Font{Family: chineseUIFont, PointSize: 13, Bold: true}, TextColor: palette.inkPrimary},
			d.ComboBox{Font: d.Font{Family: chineseUIFont, PointSize: 10}, MaxSize: d.Size{Height: 34}, AssignTo: &files},
			d.VSpacer{},
			d.Composite{
				Background: themeBrush{&palette.cardColor},
				Layout:     d.HBox{MarginsZero: true, Spacing: 10},
				Children: []d.Widget{
					d.PushButton{AssignTo: &openFileBtn, MaxSize: d.Size{Height: 34}, Text: "打开文件", MinSize: d.Size{Width: 112, Height: 34}, OnClicked: func() {
						i := files.CurrentIndex()
						if i >= 0 && i < len(paths) {
							open(paths[i])
						}
					}},
					d.PushButton{MaxSize: d.Size{Height: 34}, Text: "打开收件夹", MinSize: d.Size{Width: 112, Height: 34}, OnClicked: func() {
						if i := files.CurrentIndex(); i >= 0 && i < len(paths) {
							open(filepath.Dir(paths[i]))
							return
						}
						dir, err := receiveDir(root)
						if err != nil {
							walk.MsgBox(w, "打开收件夹", err.Error(), walk.MsgBoxIconWarning)
							return
						}
						open(dir)
					}},
					d.HSpacer{},
				},
			},
		},
	}
	if err := ui.Create(); err != nil {
		return err
	}
	defer w.Dispose()
	styleDesktopUI(w, desktopStyleConfig{
		buttonVariant: func(button *walk.PushButton) buttonVariant {
			if button == openFileBtn {
				return buttonPrimary
			}
			return buttonSecondary
		},
		buttonSurface: func(*walk.PushButton) walk.Color {
			return palette.cardColor
		},
	})
	watchDesktopAppearance(w)
	w.Closing().Attach(func(cancel *bool, _ walk.CloseReason) {
		*cancel = true
		w.Hide()
		names = nil
		paths = nil
		files.SetModel([]string{})
	})
	saved := func(path string) {
		if receiptsSilent(root) {
			return
		}
		w.Synchronize(func() {
			if receiptsSilent(root) {
				return
			}
			first := !w.Visible()
			index := files.CurrentIndex()
			names = append(names, filepath.Base(path))
			paths = append(paths, path)
			files.SetModel(names)
			if index < 0 {
				index = 0
			}
			files.SetCurrentIndex(index)
			title.SetText(fmt.Sprintf("已收到 %d 个文件", len(names)))
			if first {
				w.Show()
				win.SetWindowPos(w.Handle(), win.HWND_TOPMOST, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE)
				win.SetForegroundWindow(w.Handle())
			}
		})
	}
	result := make(chan error, 1)
	go func() { result <- serveFiles(root, saved); w.Synchronize(func() { walk.App().Exit(0) }) }()
	w.Run()
	select {
	case err := <-result:
		return err
	default:
		return nil
	}
}
