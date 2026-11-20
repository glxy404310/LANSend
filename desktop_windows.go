package main

import (
	"bytes"
	_ "embed"
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
	"unsafe"

	"golang.org/x/sys/windows"
)

const chineseUIFont = "Microsoft YaHei UI"

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
	var status, peerText *walk.Label
	var bar *walk.ProgressBar
	var sendButton, pairButton *walk.PushButton
	var busy bool
	var checking bool
	showPeer := func() {
		if checking {
			return
		}
		checking = true
		go func() { label := peerStatus(root); w.Synchronize(func() { checking = false; peerText.SetText(label) }) }()
	}
	type queueItem struct{ path, state, detail string }
	var items []*queueItem
	var queueList *walk.ListBox
	var retryButton *walk.PushButton
	var sending bool
	refreshQueue := func() {
		rows := make([]string, len(items))
		failed := false
		for i, item := range items {
			rows[i] = item.state + "   " + filepath.Base(item.path)
			if item.detail != "" {
				rows[i] += "   " + item.detail
			}
			if item.state == "失败" {
				failed = true
			}
		}
		queueList.SetModel(rows)
		retryButton.SetEnabled(failed && !busy)
	}
	var nextItem func()
	nextItem = func() {
		var item *queueItem
		for _, candidate := range items {
			if candidate.state == "等待" {
				item = candidate
				break
			}
		}
		if item == nil {
			busy = false
			sending = false
			pairButton.SetEnabled(true)
			failed := 0
			for _, it := range items {
				if it.state == "失败" {
					failed++
				}
			}
			status.SetText(fmt.Sprintf("已完成 %d 项，失败 %d 项", len(items)-failed, failed))
			refreshQueue()
			return
		}
		busy = true
		sending = true
		pairButton.SetEnabled(false)
		item.state = "发送中"
		status.SetText("正在发送 " + filepath.Base(item.path))
		bar.SetValue(0)
		refreshQueue()
		go func() {
			e := sendSaved(root, []string{item.path}, func(_, _ int, _ string, percent int) { w.Synchronize(func() { bar.SetValue(percent) }) })
			w.Synchronize(func() {
				if e != nil {
					item.state = "失败"
					item.detail = e.Error()
				} else {
					item.state = "已完成"
					bar.SetValue(100)
				}
				nextItem()
			})
		}()
	}
	sendFiles := func(paths []string) {
		if (busy && !sending) || len(paths) == 0 {
			return
		}
		for _, path := range paths {
			items = append(items, &queueItem{path: path, state: "等待"})
		}
		if sending {
			refreshQueue()
		} else {
			nextItem()
		}
	}
	retryFailed := func() {
		if busy {
			return
		}
		for _, item := range items {
			if item.state == "失败" {
				item.state = "等待"
				item.detail = ""
			}
		}
		nextItem()
	}
	var nameField, nicknameField *walk.LineEdit
	var nicknameFor string
	loadNickname := func() {
		paired, _ := readPeer(filepath.Join(root, "peer.json"))
		nicknameFor = paired.PeerHash
		nicknameField.SetEnabled(nicknameFor != "")
		nicknameField.SetText(peerNickname(root, nicknameFor))
	}
	saveName := func() {
		if err := setDeviceName(root, nameField.Text()); err != nil {
			walk.MsgBox(w, "设备名称", err.Error(), walk.MsgBoxIconWarning)
			return
		}
		nameField.SetText(deviceName(root))
		walk.MsgBox(w, "设备名称", "已保存，配对设备上线后会显示新名称。", walk.MsgBoxIconInformation)
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
	useInvite := func(copyCode bool) {
		if busy {
			return
		}
		pasted := ""
		var err error
		if !copyCode {
			pasted, err = walk.Clipboard().Text()
			if err == nil {
				_, _, _, err = parseInvite(pasted)
			}
			if err != nil {
				walk.MsgBox(w, "LANSend 码", err.Error(), walk.MsgBoxIconWarning)
				return
			}
		}
		old, err := readPeer(filepath.Join(root, "peer.json"))
		if err != nil {
			walk.MsgBox(w, "配对未完成", err.Error(), walk.MsgBoxIconWarning)
			return
		}
		if old.PeerHash != "" && walk.MsgBox(w, "已有配对", "本机已有配对。是否允许在本次配对成功后更换设备？", walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
			return
		}
		busy = true
		go func() {
			var invite string
			var err error
			if copyCode {
				invite, err = makeInvite(root, old.PeerHash)
			} else {
				err = joinInvite(root, pasted, old.PeerHash)
			}
			w.Synchronize(func() {
				busy = false
				if copyCode && err == nil {
					err = walk.Clipboard().SetText(invite)
				}
				if err != nil {
					walk.MsgBox(w, "配对未完成", err.Error(), walk.MsgBoxIconWarning)
					return
				}
				showPeer()
				loadNickname()
				msg := "已双向配对，两台电脑可以互传文件。"
				if copyCode {
					msg = "已复制 LANSend 码，请在另一台电脑点击「从剪贴板配对」。配对码 5 分钟内有效。"
				}
				walk.MsgBox(w, "LANSend 码", msg, walk.MsgBoxIconInformation)
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
	var pages [3]*walk.Composite
	var nav [3]*walk.PushButton
	currentPage := 0
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
		if busy || scanning || picker == nil || hint == nil || currentPage != 2 {
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
	const fieldLabelWidth = 88
	switchPage := func(index int) {
		currentPage = index
		for i, page := range pages {
			page.SetVisible(i == index)
			nav[i].Invalidate()
		}
		if index == 1 {
			loadNickname()
			showSaveDir()
		}
		refresh()
	}
	var links []d.Widget
	for i, title := range []string{"文件传输", "配对与设置", "我的电脑"} {
		links = append(links, d.PushButton{MaxSize: d.Size{Height: 32}, AssignTo: &nav[i], Text: title, MinSize: d.Size{Width: 98, Height: 32}, OnClicked: func() { switchPage(i) }})
	}
	links = append(links, d.HSpacer{})
	mw := d.MainWindow{
		AssignTo:    &w,
		Title:       "LANSend",
		Size:        d.Size{Width: 600, Height: 560},
		MinSize:     d.Size{Width: 520, Height: 520},
		Font:        d.Font{Family: chineseUIFont, PointSize: 10},
		Layout:      d.VBox{MarginsZero: true, SpacingZero: true},
		OnDropFiles: sendFiles,
		Children: []d.Widget{
			d.Composite{
				Background: d.SolidColorBrush{Color: dark},
				Layout:     d.HBox{Margins: d.Margins{Left: 24, Top: 16, Right: 24, Bottom: 16}, SpacingZero: true},
				Children: []d.Widget{
					d.Label{MaxSize: d.Size{Height: 38}, Text: "LANSend", Font: d.Font{Family: "Cambria Math", PointSize: 19, Bold: true}, TextColor: white},
					d.HSpacer{},
					d.Label{MaxSize: d.Size{Height: 22}, AssignTo: &peerText, TextColor: walk.RGB(202, 231, 225)},
				},
			},
			d.Composite{
				Background: d.SolidColorBrush{Color: surfaceColor},
				Layout:     d.VBox{Margins: d.Margins{Left: 22, Top: 14, Right: 18, Bottom: 16}, Spacing: 10},
				Children: []d.Widget{
					d.Composite{Layout: d.HBox{MarginsZero: true, Spacing: 6}, Children: links},
					d.Composite{AssignTo: &pages[0],
						Layout: d.VBox{Margins: d.Margins{Top: 16, Bottom: 16}, Spacing: 12},
						Children: []d.Widget{
							d.Composite{

								Layout: d.VBox{Margins: d.Margins{Top: 20, Bottom: 20}, Spacing: 10},
								Children: []d.Widget{
									d.Label{MaxSize: d.Size{Height: 36}, Text: "发送文件", Font: d.Font{Family: chineseUIFont, PointSize: 18, Bold: true}, TextColor: dark},
									d.Label{MaxSize: d.Size{Height: 22}, Text: "将文件或文件夹拖入窗口，或点击下方按钮选择。", TextColor: walk.RGB(58, 86, 86)},
									d.Composite{
										Layout: d.HBox{MarginsZero: true, Spacing: 10},
										Children: []d.Widget{
											d.PushButton{MaxSize: d.Size{Height: 34}, AssignTo: &sendButton, Text: "选择文件并发送…", MinSize: d.Size{Width: 148, Height: 34}, OnClicked: choose},
											d.PushButton{Text: "选择文件夹…", MinSize: d.Size{Width: 110, Height: 34}, MaxSize: d.Size{Height: 34}, OnClicked: func() {
												dlg := walk.FileDialog{Title: "选择要发送的文件夹"}
												ok, e := dlg.ShowBrowseFolder(w)
												if e != nil {
													walk.MsgBox(w, "选择文件夹", e.Error(), walk.MsgBoxIconWarning)
												} else if ok {
													sendFiles([]string{dlg.FilePath})
												}
											}},
											d.HSpacer{},
										},
									},
								},
							},
							d.Composite{

								Layout: d.VBox{Margins: d.Margins{Top: 10, Bottom: 10}, Spacing: 7},
								Children: []d.Widget{
									d.Label{MaxSize: d.Size{Height: 22}, Text: "传输队列", Font: d.Font{Family: chineseUIFont, PointSize: 10, Bold: true}, TextColor: dark},
									d.ListBox{AssignTo: &queueList, MinSize: d.Size{Height: 75}, MaxSize: d.Size{Height: 130}},
									d.Label{MaxSize: d.Size{Height: 22}, AssignTo: &status, Text: "等待发送"},
									d.ProgressBar{AssignTo: &bar, MaxValue: 100, MaxSize: d.Size{Height: 10}, MinSize: d.Size{Height: 10}},
								},
							},
							d.Composite{
								Layout: d.HBox{MarginsZero: true},
								Children: []d.Widget{
									d.PushButton{AssignTo: &retryButton, Text: "重试失败项", Enabled: false, MaxSize: d.Size{Height: 30}, OnClicked: retryFailed},
									d.HSpacer{},
									d.PushButton{MaxSize: d.Size{Height: 30}, Text: "打开收件箱", MinSize: d.Size{Width: 112, Height: 30}, OnClicked: inbox},
								},
							},
							d.VSpacer{},
						},
					},
					d.Composite{AssignTo: &pages[1], Visible: false,
						Layout: d.VBox{Margins: d.Margins{Top: 14, Bottom: 16}, Spacing: 12},
						Children: []d.Widget{
							d.Composite{Layout: d.HBox{MarginsZero: true, Spacing: 8}, Children: []d.Widget{
								d.Label{Text: "本机名称"}, d.LineEdit{AssignTo: &nameField, Text: deviceName(root), StretchFactor: 1}, d.PushButton{Text: "保存名称", MaxSize: d.Size{Height: 32}, OnClicked: saveName},
							}},
							d.Composite{Layout: d.HBox{MarginsZero: true, Spacing: 8}, Children: []d.Widget{
								d.Label{Text: "对方备注"}, d.LineEdit{AssignTo: &nicknameField, CueBanner: "留空使用对方的设备名称", StretchFactor: 1}, d.PushButton{Text: "保存备注", MaxSize: d.Size{Height: 32}, OnClicked: func() {
									if err := setPeerNickname(root, nicknameFor, nicknameField.Text()); err != nil {
										walk.MsgBox(w, "对方备注", err.Error(), walk.MsgBoxIconWarning)
										loadNickname()
										return
									}
									showPeer()
								}}}},
							d.Composite{

								Layout: d.VBox{Margins: d.Margins{Top: 10, Bottom: 10}, Spacing: 7},
								Children: []d.Widget{
									d.Label{MaxSize: d.Size{Height: 22}, Text: "局域网配对", Font: d.Font{Family: chineseUIFont, PointSize: 14, Bold: true}, TextColor: dark},
									d.Label{Text: "在一台电脑复制 LANSend 码，在另一台电脑粘贴即可双向配对。"},
									d.Composite{Layout: d.HBox{MarginsZero: true, Spacing: 8}, Children: []d.Widget{
										d.PushButton{Text: "复制 LANSend 码", MinSize: d.Size{Width: 132, Height: 32}, MaxSize: d.Size{Height: 32}, OnClicked: func() { useInvite(true) }},
										d.PushButton{AssignTo: &pairButton, Text: "从剪贴板配对", MinSize: d.Size{Width: 132, Height: 32}, MaxSize: d.Size{Height: 32}, OnClicked: func() { useInvite(false) }},
										d.HSpacer{},
									}},
								},
							},
							d.Composite{

								Layout: d.VBox{Margins: d.Margins{Top: 10, Bottom: 10}, Spacing: 7},
								Children: []d.Widget{
									d.Label{MaxSize: d.Size{Height: 22}, Text: "接收设置", Font: d.Font{Family: chineseUIFont, PointSize: 10, Bold: true}, TextColor: dark},
									d.Composite{
										Layout: d.HBox{MarginsZero: true, SpacingZero: true},
										Children: []d.Widget{
											d.CheckBox{MaxSize: d.Size{Height: 32},
												AssignTo: &silent,
												Text:     "静默接收（关闭时前台弹窗）",
												Checked:  receiptsSilent(root),
												OnClicked: func() {
													if err := setReceiptsSilent(root, silent.Checked()); err != nil {
														silent.SetChecked(receiptsSilent(root))
														walk.MsgBox(w, "收件设置", err.Error(), walk.MsgBoxIconWarning)
													}
												},
											},
											d.HSpacer{},
										},
									},
									d.Composite{
										Layout: d.HBox{MarginsZero: true, Spacing: 8},
										Children: []d.Widget{
											d.Label{MaxSize: d.Size{Height: 22}, Text: "收件目录", MinSize: d.Size{Width: fieldLabelWidth}},
											d.LineEdit{Font: d.Font{Family: chineseUIFont, PointSize: 10}, MaxSize: d.Size{Height: 32}, AssignTo: &saveDirEdit, ReadOnly: true, StretchFactor: 1, Text: currentSaveDir()},
											d.PushButton{MaxSize: d.Size{Height: 30}, Text: "更改…", MinSize: d.Size{Width: 76, Height: 30}, OnClicked: chooseSaveDir},
											d.PushButton{MaxSize: d.Size{Height: 30}, AssignTo: &resetDirButton, Text: "恢复默认", MinSize: d.Size{Width: 88, Height: 30}, Enabled: isCustomReceiveDir(root), OnClicked: resetSaveDir},
										},
									},
								},
							},
							d.VSpacer{},
						},
					},
					d.Composite{AssignTo: &pages[2], Visible: false,
						Layout: d.VBox{Margins: d.Margins{Top: 16, Bottom: 16}, Spacing: 12},
						Children: []d.Widget{
							d.Label{MaxSize: d.Size{Height: 32}, Text: "跨网络同账号设备", Font: d.Font{Family: chineseUIFont, PointSize: 16, Bold: true}},
							d.Composite{

								Layout: d.VBox{Margins: d.Margins{Top: 10, Bottom: 10}, Spacing: 7},
								Children: []d.Widget{
									d.Label{MaxSize: d.Size{Height: 22}, Text: "在线设备", Font: d.Font{Family: chineseUIFont, PointSize: 10, Bold: true}, TextColor: dark},
									d.Label{MaxSize: d.Size{Height: 22}, AssignTo: &hint, Text: "正在查找同一账号的电脑…"},
									d.ComboBox{Font: d.Font{Family: chineseUIFont, PointSize: 10}, MaxSize: d.Size{Height: 32}, AssignTo: &picker},
									d.Composite{
										Layout: d.HBox{MarginsZero: true, Spacing: 10},
										Children: []d.Widget{
											d.PushButton{MaxSize: d.Size{Height: 34}, Text: "配对所选电脑", MinSize: d.Size{Width: 136, Height: 34}, OnClicked: connect},
											d.PushButton{MaxSize: d.Size{Height: 34}, Text: "远程连接…", MinSize: d.Size{Width: 112, Height: 34}, OnClicked: remote},
											d.HSpacer{},
										},
									},
								},
							},
							d.VSpacer{},
						},
					},
				},
			},
		},
	}
	if e := mw.Create(); e != nil {
		return e
	}
	defer w.Dispose()
	styleDesktopControls(w, func(button *walk.PushButton) bool {
		return button == nav[currentPage] || button == sendButton || button == pairButton
	})
	// The compositor restores square edges automatically when maximized.
	const dwmWindowCornerPreference = 33
	round := uint32(2) // DWMWCP_ROUND
	setAttribute := windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
	if setAttribute.Find() == nil {
		setAttribute.Call(uintptr(w.Handle()), dwmWindowCornerPreference, uintptr(unsafe.Pointer(&round)), unsafe.Sizeof(round))
	}
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
				})
			case <-stop:
				return
			}
		}
	}()
	w.Run()
	return nil
}
