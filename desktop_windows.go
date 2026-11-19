package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/lxn/walk"
	d "github.com/lxn/walk/declarative"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"time"
)

//go:embed windows-ui/Setup.ps1
var setupScript []byte

func installDesktop(exe, target, root string) error {
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := initialize(root); err != nil {
		return err
	}
	src, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	old, _ := os.ReadFile(target)
	if !bytes.Equal(src, old) {
		if err = os.WriteFile(target, src, 0700); err != nil {
			return fmt.Errorf("请先关闭已打开的 LANSend 窗口，再重试：%w", err)
		}
	}
	if err = os.WriteFile(filepath.Join(dir, "Setup.ps1"), append([]byte{0xef, 0xbb, 0xbf}, setupScript...), 0600); err != nil {
		return err
	}
	shell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	cmd := exec.Command(shell, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(dir, "Setup.ps1"))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if out, e := cmd.CombinedOutput(); e != nil {
		return fmt.Errorf("安装接收服务失败：%s (%w)", out, e)
	}
	return ready(root)
}
func desktopEntry(args []string) bool {
	if len(args) > 0 && args[0] != "menu" && !filepath.IsAbs(args[0]) {
		return false
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	exe, e := os.Executable()
	if e != nil {
		return true
	}
	dir, e := os.UserConfigDir()
	if e != nil {
		return true
	}
	root := filepath.Join(dir, "LANSend")
	target := filepath.Join(os.Getenv("LOCALAPPDATA"), "LANSend", "LANSend-Desktop.exe")
	if !strings.EqualFold(exe, target) {
		installed, _ := os.ReadFile(target)
		current, readErr := os.ReadFile(exe)
		if readErr == nil && bytes.Equal(installed, current) && ready(root) == nil {
			if err := exec.Command(target, args...).Start(); err != nil {
				walk.MsgBox(nil, "打开 LANSend", err.Error(), walk.MsgBoxIconError)
			}
			return true
		}
		if walk.MsgBox(nil, "安装 LANSend", "安装到当前用户并添加开始菜单、右键发送和后台接收功能？已有配对会保留。", walk.MsgBoxYesNo|walk.MsgBoxIconInformation) != walk.DlgCmdYes {
			return true
		}
		if e = installDesktop(exe, target, root); e == nil {
			e = exec.Command(target, args...).Start()
		}
	} else {
		if len(args) > 0 && args[0] == "menu" {
			args = args[1:]
		}
		e = dashboard(root, args)
	}
	if e != nil {
		walk.MsgBox(nil, "LANSend", e.Error(), walk.MsgBoxIconError)
	}
	return true
}
func dashboard(root string, initial []string) error {
	var w *walk.MainWindow
	var status, peerText, localAddr *walk.Label
	var bar *walk.ProgressBar
	var addr, code *walk.LineEdit
	var sendButton, pairButton *walk.PushButton
	var busy bool
	showPeer := func() {
		var c config
		b, _ := os.ReadFile(filepath.Join(root, "peer.json"))
		json.Unmarshal(b, &c)
		label := "尚未配对"
		if c.Address != "" {
			label = "已配对   ·   " + c.Address
		}
		if c.RemoteAddress != "" {
			label += "   /   " + c.RemoteAddress
		}
		peerText.SetText(label)
	}
	sendFiles := func(paths []string) {
		if busy || len(paths) == 0 {
			return
		}
		busy = true
		sendButton.SetEnabled(false)
		pairButton.SetEnabled(false)
		bar.SetValue(0)
		go func() {
			e := sendSaved(root, paths, func(i, total int, name string, percent int) {
				w.Synchronize(func() {
					status.SetText(fmt.Sprintf("%d / %d    %s    %d%%", i, total, name, percent))
					bar.SetValue(((i-1)*100 + percent) / total)
				})
			})
			w.Synchronize(func() {
				busy = false
				sendButton.SetEnabled(true)
				pairButton.SetEnabled(true)
				if e != nil {
					status.SetText("发送未完成")
					walk.MsgBox(w, "发送未完成", e.Error(), walk.MsgBoxIconWarning)
				} else {
					bar.SetValue(100)
					status.SetText(fmt.Sprintf("已发送 %d 个文件", len(paths)))
				}
			})
		}()
	}
	choose := func() {
		dlg := walk.FileDialog{Title: "选择要发送的文件"}
		ok, e := dlg.ShowOpenMultiple(w)
		if e != nil {
			walk.MsgBox(w, "选择文件", e.Error(), walk.MsgBoxIconWarning)
		} else if ok {
			sendFiles(dlg.FilePaths)
		}
	}
	mine := ""
	if c, e := loadIdentity(root); e == nil {
		_, hash, e := client(c, false)
		if e == nil {
			mine = hash[:12]
		}
	}
	formatLocalIP := func() string {
		if ip := primaryLocalIPv4(); ip != "" {
			return "本机地址    " + ip
		}
		return "本机地址    未检测到局域网地址"
	}
	showLocalIP := func() {
		if localAddr == nil {
			return
		}
		text := formatLocalIP()
		if localAddr.Text() != text {
			localAddr.SetText(text)
		}
	}
	pairNow := func() {
		if busy {
			return
		}
		address, key := strings.TrimSpace(addr.Text()), strings.TrimSpace(code.Text())
		if address == "" || len(key) != 12 {
			walk.MsgBox(w, "配对", "请填写对方地址和 12 位校验码。", walk.MsgBoxIconInformation)
			return
		}
		busy = true
		pairButton.SetEnabled(false)
		sendButton.SetEnabled(false)
		status.SetText("正在验证对方设备…")
		go func() {
			e := pair(filepath.Join(root, "peer.json"), address, func() string { return key })
			w.Synchronize(func() {
				busy = false
				pairButton.SetEnabled(true)
				sendButton.SetEnabled(true)
				if e != nil {
					status.SetText("配对未完成")
					walk.MsgBox(w, "配对未完成", e.Error(), walk.MsgBoxIconWarning)
				} else {
					showPeer()
					code.SetText("")
					status.SetText("已保存配对")
				}
			})
		}()
	}
	remote := func() {
		if busy {
			return
		}
		busy = true
		sendButton.SetEnabled(false)
		pairButton.SetEnabled(false)
		go func() {
			msg, e := remoteLogin(root)
			w.Synchronize(func() {
				busy = false
				sendButton.SetEnabled(true)
				pairButton.SetEnabled(true)
				if e != nil {
					msg = e.Error()
				}
				walk.MsgBox(w, "远程连接", msg, walk.MsgBoxIconInformation)
			})
		}()
	}
	var silent *walk.CheckBox
	var tabs *walk.TabWidget
	var picker *walk.ComboBox
	var hint *walk.Label
	var saveDirEdit *walk.LineEdit
	var resetDirButton *walk.PushButton
	var choices []accountDevice
	var scanning bool
	currentSaveDir := func() string {
		dir, err := receiveDir(root)
		if err != nil {
			return ""
		}
		return dir
	}
	showSaveDir := func() {
		if saveDirEdit != nil {
			dir := currentSaveDir()
			if saveDirEdit.Text() != dir {
				saveDirEdit.SetText(dir)
			}
		}
		if resetDirButton != nil {
			resetDirButton.SetEnabled(isCustomReceiveDir(root))
		}
	}
	chooseSaveDir := func() {
		if busy {
			return
		}
		dlg := walk.FileDialog{Title: "选择收件保存文件夹"}
		ok, err := dlg.ShowBrowseFolder(w)
		if err != nil {
			walk.MsgBox(w, "选择文件夹", err.Error(), walk.MsgBoxIconWarning)
			return
		}
		if !ok {
			return
		}
		if _, err = setReceiveDir(root, dlg.FilePath); err != nil {
			walk.MsgBox(w, "收件目录", err.Error(), walk.MsgBoxIconWarning)
			return
		}
		showSaveDir()
	}
	resetSaveDir := func() {
		if busy {
			return
		}
		if _, err := setReceiveDir(root, ""); err != nil {
			walk.MsgBox(w, "收件目录", err.Error(), walk.MsgBoxIconWarning)
			return
		}
		showSaveDir()
	}
	refresh := func() {
		if busy || scanning || tabs == nil || picker == nil || hint == nil || tabs.CurrentIndex() != 2 {
			return
		}
		scanning = true
		go func() {
			st, e := nodeRequest(root, "devices")
			w.Synchronize(func() {
				scanning = false
				if e != nil {
					choices = nil
					picker.SetModel([]string{})
					hint.SetText(e.Error())
					return
				}
				if !slices.Equal(choices, st.Devices) {
					selected := ""
					if i := picker.CurrentIndex(); i >= 0 && i < len(choices) {
						selected = choices[i].ID
					}
					choices = st.Devices
					names := make([]string, len(choices))
					idx := -1
					if len(choices) == 1 {
						idx = 0
					}
					for i, p := range choices {
						names[i] = p.Name
						if p.ID == selected {
							idx = i
						}
					}
					picker.SetModel(names)
					if len(choices) > 0 {
						picker.SetCurrentIndex(idx)
					}
				}
				if len(choices) == 0 {
					hint.SetText("等待另一台 LANSend 登录同一账号…")
				} else {
					hint.SetText("选择要双向配对的电脑")
				}
			})
		}()
	}
	connect := func() {
		if busy {
			return
		}
		i := picker.CurrentIndex()
		if i < 0 || i >= len(choices) {
			return
		}
		target := choices[i]
		busy = true
		hint.SetText("正在与 " + target.Name + " 配对…")
		go func() {
			_, e := nodeRequest(root, "connect?id="+url.QueryEscape(target.ID))
			w.Synchronize(func() {
				busy = false
				showPeer()
				if e != nil {
					walk.MsgBox(w, "配对未完成", e.Error(), walk.MsgBoxIconWarning)
				} else {
					walk.MsgBox(w, "配对完成", "两台电脑现在可以互传文件。", walk.MsgBoxIconInformation)
				}
				refresh()
			})
		}()
	}
	inbox := func() {
		p, err := receiveDir(root)
		if err == nil {
			err = os.MkdirAll(p, 0700)
		}
		if err != nil {
			walk.MsgBox(w, "打开收件箱", err.Error(), walk.MsgBoxIconWarning)
			return
		}
		if e := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "explorer.exe"), p).Start(); e != nil {
			walk.MsgBox(w, "打开收件箱", e.Error(), walk.MsgBoxIconWarning)
		}
	}
	dark := walk.RGB(19, 53, 61)
	white := walk.RGB(255, 255, 255)
	mw := d.MainWindow{AssignTo: &w, Title: "LANSend", Size: d.Size{Width: 580, Height: 525}, MinSize: d.Size{Width: 500, Height: 490}, Font: d.Font{Family: "Segoe UI", PointSize: 10}, Layout: d.VBox{MarginsZero: true, SpacingZero: true}, OnDropFiles: sendFiles, Children: []d.Widget{
		d.Composite{Background: d.SolidColorBrush{Color: dark}, Layout: d.VBox{Margins: d.Margins{Left: 24, Top: 16, Right: 24, Bottom: 16}}, Children: []d.Widget{
			d.Label{Text: "LANSend", Font: d.Font{Family: "Segoe UI", PointSize: 23, Bold: true}, TextColor: white}, d.Label{AssignTo: &peerText, TextColor: white}}},
		d.Composite{Layout: d.VBox{Margins: d.Margins{Left: 20, Top: 14, Right: 20, Bottom: 14}, Spacing: 10}, Children: []d.Widget{
			d.TabWidget{AssignTo: &tabs, OnCurrentIndexChanged: func() {
				refresh()
				if tabs != nil && tabs.CurrentIndex() == 1 {
					showLocalIP()
					showSaveDir()
				}
			}, Pages: []d.TabPage{
				{Title: "文件传输", Layout: d.VBox{Margins: d.Margins{Left: 16, Top: 16, Right: 16, Bottom: 14}, Spacing: 12}, Children: []d.Widget{
					d.Label{Text: "发送到已配对设备", Font: d.Font{Family: "Segoe UI", PointSize: 16, Bold: true}}, d.Label{Text: "可多选文件，或直接拖入此窗口发送。"},
					d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{d.PushButton{AssignTo: &sendButton, Text: "选择文件并发送…", OnClicked: choose}, d.PushButton{Text: "打开收件箱", OnClicked: inbox}, d.HSpacer{}}}, d.VSpacer{}, d.Label{AssignTo: &status, Text: "等待发送"}, d.ProgressBar{AssignTo: &bar, MaxValue: 100}}},
				{Title: "配对与设置", Layout: d.VBox{Margins: d.Margins{Left: 16, Top: 14, Right: 16, Bottom: 14}}, Children: []d.Widget{
					d.Label{AssignTo: &localAddr, Text: formatLocalIP()}, d.Label{Text: "本机校验码    " + mine}, d.HSeparator{}, d.LineEdit{AssignTo: &addr, CueBanner: "对方的局域网 IP 或 Tailscale 地址"}, d.LineEdit{AssignTo: &code, CueBanner: "对方的 12 位校验码", MaxLength: 12}, d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{d.PushButton{AssignTo: &pairButton, Text: "验证并保存配对", OnClicked: pairNow}, d.PushButton{Text: "远程连接…", OnClicked: remote}}}, d.CheckBox{AssignTo: &silent, Text: "静默接收（关闭时前台弹窗）", Checked: receiptsSilent(root), OnClicked: func() {
						if err := setReceiptsSilent(root, silent.Checked()); err != nil {
							silent.SetChecked(receiptsSilent(root))
							walk.MsgBox(w, "收件设置", err.Error(), walk.MsgBoxIconWarning)
						}
					}}, d.HSeparator{}, d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
						d.Label{Text: "收件目录"},
						d.LineEdit{AssignTo: &saveDirEdit, ReadOnly: true, Text: currentSaveDir()},
						d.PushButton{Text: "更改…", OnClicked: chooseSaveDir},
						d.PushButton{AssignTo: &resetDirButton, Text: "恢复默认", Enabled: isCustomReceiveDir(root), OnClicked: resetSaveDir},
					}}, d.VSpacer{}}},
				{Title: "我的电脑", Layout: d.VBox{Margins: d.Margins{Left: 16, Top: 16, Right: 16, Bottom: 14}, Spacing: 12}, Children: []d.Widget{
					d.Label{AssignTo: &hint, Text: "正在查找同一账号的电脑…"}, d.ComboBox{AssignTo: &picker},
					d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{d.PushButton{Text: "配对所选电脑", OnClicked: connect}, d.PushButton{Text: "远程连接…", OnClicked: remote}}}, d.VSpacer{}}}}},
		}}}}
	if e := mw.Create(); e != nil {
		return e
	}
	defer w.Dispose()
	showPeer()
	if x, e := os.Executable(); e == nil {
		if icon, e := walk.NewIconExtractedFromFile(x, 0, 32); e == nil {
			w.SetIcon(icon)
		}
	}
	w.Closing().Attach(func(cancel *bool, _ walk.CloseReason) {
		if busy {
			*cancel = true
			walk.MsgBox(w, "LANSend", "请在当前操作完成后关闭。", walk.MsgBoxIconInformation)
		}
	})
	if len(initial) > 0 {
		sendFiles(initial)
	}
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		tick := time.NewTicker(5 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				w.Synchronize(func() {
					showPeer()
					refresh()
					if tabs != nil && tabs.CurrentIndex() == 1 {
						showLocalIP()
					}
				})
			case <-stop:
				return
			}
		}
	}()
	w.Run()
	return nil
}
