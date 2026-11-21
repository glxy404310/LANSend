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
	if len(args) >= 3 && args[0] == "send-to" {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if e := run(args); e != nil {
			walk.MsgBox(nil, "发送未完成", e.Error(), walk.MsgBoxIconWarning)
		}
		return true
	}
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
	var status *walk.Label
	var peerBadge *walk.CustomWidget
	connectedCount := -1
	var bar *walk.ProgressBar
	var sendButton, pairButton *walk.PushButton
	var busy bool
	var checking bool
	showPeer := func() {
		if checking {
			return
		}
		checking = true
		go func() {
			count, err := connectedPeerCount(root)
			if err != nil {
				count = -1
			}
			w.Synchronize(func() {
				checking = false
				connectedCount = count
				if peerBadge != nil {
					peerBadge.Invalidate()
				}
			})
		}()
	}
	type queueItem struct{ path, state, detail, hash string }
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
			e := sendTo(root, item.hash, []string{item.path}, func(_, _ int, _ string, percent int) { w.Synchronize(func() { bar.SetValue(percent) }) })
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
		target, _ := readPeer(filepath.Join(root, "peer.json"))
		if !target.Enabled {
			walk.MsgBox(w, "请选择设备", "请先在设备管理中勾选启用发送目标。", walk.MsgBoxIconInformation)
			return
		}
		for _, path := range paths {
			items = append(items, &queueItem{hash: target.PeerHash, path: path, state: "等待"})
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
	var nameField *walk.LineEdit
	var nameDisplay *walk.Composite
	var nameLabel, pairedCount *walk.Label
	var editName, cancelName *walk.PushButton
	setNameEditing := func(editing bool) {
		w.SetSuspended(true)
		if editing {
			nameField.SetText(deviceName(root))
		}
		nameDisplay.SetVisible(!editing)
		nameField.SetVisible(editing)
		cancelName.SetVisible(editing)
		if editing {
			editName.SetText("保存")
		} else {
			editName.SetText("编辑")
		}
		w.SetSuspended(false)
		if editing {
			nameField.SetFocus()
			nameField.SetTextSelection(0, -1)
		} else {
			editName.SetFocus()
		}
	}
	saveName := func() {
		if !nameField.Visible() {
			setNameEditing(true)
			return
		}
		if err := setDeviceName(root, nameField.Text()); err != nil {
			walk.MsgBox(w, "设备名称", err.Error(), walk.MsgBoxIconWarning)
			nameField.SetFocus()
			return
		}
		nameLabel.SetText(deviceName(root))
		setNameEditing(false)
	}
	refreshCount := func() {
		all, err := peersAt(filepath.Join(root, "peer.json"))
		value := fmt.Sprintf("%d 台", len(all))
		if err != nil {
			value = "读取失败"
		}
		if pairedCount.Text() != value {
			pairedCount.SetText(value)
		}
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
				refreshCount()
				msg := "已双向配对。请在两端的设备管理中勾选启用对方。"
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
			saveDirEdit.SetToolTipText(dir)
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
	const fieldLabelWidth = 72
	var segBar, queueFrame *walk.Composite
	var sendCard, queueCard, deviceInfoCard, lanPairCard, receiveCard, remoteCard *walk.Composite
	var connectButton, inboxButton *walk.PushButton
	switchPage := func(index int) {
		if index != currentPage {
			// A hidden page has no geometry on its first visit. Arrange it before
			// replacing the visible page so the compositor has a complete frame.
			bounds := pages[currentPage].BoundsPixels()
			next := pages[index]
			next.SetBoundsPixels(bounds)
			arrangeDesktopChildren(walk.CreateLayoutItemsForContainer(next), bounds.Size())
		}
		w.SetSuspended(true)
		currentPage = index
		for i, page := range pages {
			page.SetVisible(i == index)
			nav[i].Invalidate()
		}
		w.SetSuspended(false)
		if index == 1 {
			refreshCount()
			showSaveDir()
		}
		refresh()
	}
	var links []d.Widget
	for i, title := range []string{"文件传输", "配对与设置", "我的电脑"} {
		idx := i
		links = append(links, d.PushButton{
			AssignTo:  &nav[idx],
			Text:      title,
			MinSize:   d.Size{Width: 102, Height: 30},
			MaxSize:   d.Size{Height: 30},
			Font:      d.Font{Family: chineseUIFont, PointSize: 9},
			OnClicked: func() { switchPage(idx) },
		})
	}
	mw := d.MainWindow{
		Visible:         false,
		DoubleBuffering: true,
		AssignTo:        &w,
		Background:      themeBrush{&palette.canvasColor},
		Title:           "LANSend",
		Size:            d.Size{Width: 620, Height: 590},
		MinSize:         d.Size{Width: 540, Height: 540},
		Font:            d.Font{Family: chineseUIFont, PointSize: 10},
		Layout:          d.VBox{MarginsZero: true, SpacingZero: true},
		OnDropFiles:     sendFiles,
		Children: []d.Widget{
			// Seamless minimalist light header
			d.Composite{
				Background: themeBrush{&palette.headerColor},
				MaxSize:    d.Size{Height: 58},
				Layout:     d.HBox{Margins: d.Margins{Left: 24, Top: 12, Right: 24, Bottom: 14}, SpacingZero: true},
				Children: []d.Widget{
					d.Label{
						Text:      "LANSend",
						Font:      d.Font{Family: "Segoe UI", PointSize: 16, Bold: true},
						TextColor: palette.inkPrimary,
						MaxSize:   d.Size{Height: 32},
					},
					d.HSpacer{},
					d.CustomWidget{
						AssignTo:            &peerBadge,
						Background:          themeBrush{&palette.headerColor},
						Font:                d.Font{Family: chineseUIFont, PointSize: 9},
						MinSize:             d.Size{Width: 220, Height: 28},
						MaxSize:             d.Size{Height: 28},
						InvalidatesOnResize: true,
						Paint:               func(*walk.Canvas, walk.Rectangle) error { return nil },
					},
				},
			},
			// Hairline header divider
			d.Composite{
				Background: themeBrush{&palette.dividerColor},
				MinSize:    d.Size{Height: 1},
				MaxSize:    d.Size{Height: 1},
			},
			// Main canvas
			d.Composite{
				Background:    themeBrush{&palette.canvasColor},
				StretchFactor: 100,
				Layout:        d.VBox{Margins: d.Margins{Left: 24, Top: 16, Right: 24, Bottom: 20}, Spacing: 14},
				Children: []d.Widget{
					// Segmented control navigation
					d.Composite{
						Layout: d.HBox{MarginsZero: true, SpacingZero: true},
						Children: []d.Widget{
							d.Composite{
								AssignTo:   &segBar,
								Background: themeBrush{&palette.segTrackColor},
								Layout:     d.HBox{Margins: d.Margins{Left: 4, Top: 4, Right: 4, Bottom: 4}, Spacing: 4},
								Children:   links,
							},
							d.HSpacer{},
						},
					},
					// Page 0: 文件传输
					d.Composite{
						AssignTo:      &pages[0],
						StretchFactor: 1,
						Layout:        d.VBox{MarginsZero: true, Spacing: 14},
						Children: []d.Widget{
							d.Composite{
								AssignTo:   &sendCard,
								Background: themeBrush{&palette.cardColor},
								Layout:     d.VBox{Margins: d.Margins{Left: 20, Top: 18, Right: 20, Bottom: 20}, Spacing: 10},
								Children: []d.Widget{
									d.Label{
										Text:      "发送文件",
										Font:      d.Font{Family: chineseUIFont, PointSize: 14, Bold: true},
										TextColor: palette.inkPrimary,
										MaxSize:   d.Size{Height: 28},
									},
									d.Label{
										Text:      "将文件或文件夹拖入窗口任意位置，或点击下方按钮选择。",
										Font:      d.Font{Family: chineseUIFont, PointSize: 9},
										TextColor: palette.inkSecondary,
										MaxSize:   d.Size{Height: 20},
									},
									d.Composite{
										Background: themeBrush{&palette.cardColor},
										Layout:     d.HBox{Margins: d.Margins{Top: 4}, Spacing: 10},
										Children: []d.Widget{
											d.PushButton{
												AssignTo:  &sendButton,
												Text:      "选择文件并发送…",
												MinSize:   d.Size{Width: 146, Height: 36},
												MaxSize:   d.Size{Height: 36},
												OnClicked: choose,
											},
											d.PushButton{
												Text:    "选择文件夹…",
												MinSize: d.Size{Width: 114, Height: 36},
												MaxSize: d.Size{Height: 36},
												OnClicked: func() {
													dlg := walk.FileDialog{Title: "选择要发送的文件夹"}
													ok, e := dlg.ShowBrowseFolder(w)
													if e != nil {
														walk.MsgBox(w, "选择文件夹", e.Error(), walk.MsgBoxIconWarning)
													} else if ok {
														sendFiles([]string{dlg.FilePath})
													}
												},
											},
											d.HSpacer{},
										},
									},
								},
							},
							d.Composite{
								AssignTo:   &queueCard,
								Background: themeBrush{&palette.cardColor},
								Layout:     d.VBox{Margins: d.Margins{Left: 20, Top: 16, Right: 20, Bottom: 18}, Spacing: 10},
								Children: []d.Widget{
									d.Composite{
										Background: themeBrush{&palette.cardColor},
										Layout:     d.HBox{MarginsZero: true, SpacingZero: true},
										Children: []d.Widget{
											d.Label{
												Text:      "传输队列",
												Font:      d.Font{Family: chineseUIFont, PointSize: 10, Bold: true},
												TextColor: palette.inkPrimary,
												MaxSize:   d.Size{Height: 22},
											},
											d.HSpacer{},
											d.Label{
												AssignTo:  &status,
												Text:      "等待发送",
												Font:      d.Font{Family: chineseUIFont, PointSize: 9},
												TextColor: palette.inkSecondary,
												MaxSize:   d.Size{Height: 22},
											},
										},
									},
									d.Composite{
										AssignTo:   &queueFrame,
										Background: themeBrush{&palette.cardColor},
										Layout:     d.VBox{Margins: d.Margins{Left: 6, Top: 5, Right: 6, Bottom: 5}, SpacingZero: true},
										Children: []d.Widget{
											d.ListBox{
												AssignTo: &queueList,
												MinSize:  d.Size{Height: 108},
												MaxSize:  d.Size{Height: 162},
											},
										},
									},
									d.ProgressBar{
										AssignTo: &bar,
										MaxValue: 100,
										MinSize:  d.Size{Height: 8},
										MaxSize:  d.Size{Height: 8},
									},
								},
							},
							d.Composite{
								Layout: d.HBox{MarginsZero: true, Spacing: 10},
								Children: []d.Widget{
									d.PushButton{
										AssignTo:  &retryButton,
										Text:      "重试失败项",
										Enabled:   false,
										MinSize:   d.Size{Width: 106, Height: 34},
										MaxSize:   d.Size{Height: 34},
										OnClicked: retryFailed,
									},
									d.HSpacer{},
									d.PushButton{
										AssignTo:  &inboxButton,
										Text:      "打开收件箱",
										MinSize:   d.Size{Width: 110, Height: 34},
										MaxSize:   d.Size{Height: 34},
										OnClicked: inbox,
									},
								},
							},
							d.VSpacer{},
						},
					},
					// Page 1: 配对与设置
					d.Composite{
						AssignTo:      &pages[1],
						Visible:       false,
						StretchFactor: 1,
						Layout:        d.VBox{MarginsZero: true, Spacing: 12},
						Children: []d.Widget{
							d.Composite{
								AssignTo:   &deviceInfoCard,
								Background: themeBrush{&palette.cardColor},
								Layout:     d.VBox{Margins: d.Margins{Left: 20, Top: 14, Right: 20, Bottom: 16}, Spacing: 10},
								Children: []d.Widget{
									d.Label{
										Text:      "设备信息",
										Font:      d.Font{Family: chineseUIFont, PointSize: 10, Bold: true},
										TextColor: palette.inkPrimary,
										MaxSize:   d.Size{Height: 22},
									},
									d.Composite{
										Background: themeBrush{&palette.cardColor},
										Layout:     d.HBox{MarginsZero: true, Spacing: 10},
										Children: []d.Widget{
											d.Label{Text: "本机名称", MinSize: d.Size{Width: fieldLabelWidth}, MaxSize: d.Size{Width: fieldLabelWidth}, TextColor: palette.inkSecondary},
											d.Composite{AssignTo: &nameDisplay, StretchFactor: 1, Layout: d.HBox{MarginsZero: true, SpacingZero: true}, Children: []d.Widget{
												d.Label{AssignTo: &nameLabel, Text: deviceName(root), EllipsisMode: d.EllipsisEnd, NoPrefix: true}, d.HSpacer{},
											}},
											d.LineEdit{AssignTo: &nameField, Visible: false, StretchFactor: 1, MaxSize: d.Size{Height: 32}, OnKeyDown: func(key walk.Key) {
												switch key {
												case walk.KeyEscape:
													setNameEditing(false)
												case walk.KeyReturn:
													saveName()
												}
											}},
											d.PushButton{AssignTo: &cancelName, MinSize: d.Size{Width: 84, Height: 32}, MaxSize: d.Size{Width: 84, Height: 32}, Text: "取消", Visible: false, OnClicked: func() { setNameEditing(false) }},
											d.PushButton{AssignTo: &editName, MinSize: d.Size{Width: 84, Height: 32}, MaxSize: d.Size{Width: 84, Height: 32}, Text: "编辑", OnClicked: saveName},
										},
									},
									d.Composite{
										Background: themeBrush{&palette.cardColor},
										Layout:     d.HBox{MarginsZero: true, Spacing: 10},
										Children: []d.Widget{
											d.Label{Text: "已配对设备", MinSize: d.Size{Width: fieldLabelWidth}, MaxSize: d.Size{Width: fieldLabelWidth}, TextColor: palette.inkSecondary},
											d.Label{AssignTo: &pairedCount, Text: "0 台"}, d.HSpacer{},
											d.PushButton{Text: "管理…", MinSize: d.Size{Width: 84, Height: 32}, MaxSize: d.Size{Width: 84, Height: 32}, OnClicked: func() { manageDevices(w, root); refreshCount(); showPeer() }},
										},
									},
								},
							},
							d.Composite{
								AssignTo:   &lanPairCard,
								Background: themeBrush{&palette.cardColor},
								Layout:     d.VBox{Margins: d.Margins{Left: 20, Top: 14, Right: 20, Bottom: 16}, Spacing: 9},
								Children: []d.Widget{
									d.Label{
										Text:      "局域网配对",
										Font:      d.Font{Family: chineseUIFont, PointSize: 10, Bold: true},
										TextColor: palette.inkPrimary,
										MaxSize:   d.Size{Height: 22},
									},
									d.Label{
										Text:      "在一台电脑复制 LANSend 码，在另一台电脑粘贴即可双向配对。",
										Font:      d.Font{Family: chineseUIFont, PointSize: 9},
										TextColor: palette.inkSecondary,
										MaxSize:   d.Size{Height: 20},
									},
									d.Composite{
										Background: themeBrush{&palette.cardColor},
										Layout:     d.HBox{Margins: d.Margins{Top: 2}, Spacing: 10},
										Children: []d.Widget{
											d.PushButton{
												AssignTo:  &pairButton,
												Text:      "从剪贴板配对",
												MinSize:   d.Size{Width: 132, Height: 34},
												MaxSize:   d.Size{Height: 34},
												OnClicked: func() { useInvite(false) },
											},
											d.PushButton{
												Text:      "复制 LANSend 码",
												MinSize:   d.Size{Width: 136, Height: 34},
												MaxSize:   d.Size{Height: 34},
												OnClicked: func() { useInvite(true) },
											},
											d.HSpacer{},
										},
									},
								},
							},
							d.Composite{
								AssignTo:   &receiveCard,
								Background: themeBrush{&palette.cardColor},
								Layout:     d.VBox{Margins: d.Margins{Left: 20, Top: 14, Right: 20, Bottom: 16}, Spacing: 9},
								Children: []d.Widget{
									d.Label{
										Text:      "接收设置",
										Font:      d.Font{Family: chineseUIFont, PointSize: 10, Bold: true},
										TextColor: palette.inkPrimary,
										MaxSize:   d.Size{Height: 22},
									},
									d.Composite{
										Background: themeBrush{&palette.cardColor},
										Layout:     d.HBox{MarginsZero: true, SpacingZero: true},
										Children: []d.Widget{
											d.CheckBox{
												AssignTo: &silent,
												Text:     "静默接收（关闭时前台弹窗）",
												Checked:  receiptsSilent(root),
												MaxSize:  d.Size{Height: 28},
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
										Background: themeBrush{&palette.cardColor},
										Layout:     d.HBox{MarginsZero: true, Spacing: 8},
										Children: []d.Widget{
											d.Label{Text: "收件目录", MinSize: d.Size{Width: fieldLabelWidth}, MaxSize: d.Size{Height: 22}, TextColor: palette.inkSecondary},
											d.LineEdit{Font: d.Font{Family: chineseUIFont, PointSize: 9}, MaxSize: d.Size{Height: 32}, AssignTo: &saveDirEdit, Background: themeBrush{&palette.cardColor}, ReadOnly: true, StretchFactor: 1, Text: currentSaveDir()},
											d.PushButton{Text: "更改…", MinSize: d.Size{Width: 68, Height: 32}, MaxSize: d.Size{Height: 32}, OnClicked: chooseSaveDir},
											d.PushButton{AssignTo: &resetDirButton, Text: "恢复默认", MinSize: d.Size{Width: 84, Height: 32}, MaxSize: d.Size{Height: 32}, Enabled: isCustomReceiveDir(root), OnClicked: resetSaveDir},
										},
									},
								},
							},
							d.VSpacer{},
						},
					},
					// Page 2: 我的电脑
					d.Composite{
						AssignTo:      &pages[2],
						Visible:       false,
						StretchFactor: 1,
						Layout:        d.VBox{MarginsZero: true, Spacing: 14},
						Children: []d.Widget{
							d.Composite{
								AssignTo:   &remoteCard,
								Background: themeBrush{&palette.cardColor},
								Layout:     d.VBox{Margins: d.Margins{Left: 20, Top: 18, Right: 20, Bottom: 20}, Spacing: 12},
								Children: []d.Widget{
									d.Label{
										Text:      "跨网络同账号设备",
										Font:      d.Font{Family: chineseUIFont, PointSize: 14, Bold: true},
										TextColor: palette.inkPrimary,
										MaxSize:   d.Size{Height: 28},
									},
									d.Label{
										Text:      "在线设备",
										Font:      d.Font{Family: chineseUIFont, PointSize: 10, Bold: true},
										TextColor: palette.inkPrimary,
										MaxSize:   d.Size{Height: 22},
									},
									d.Label{
										AssignTo:  &hint,
										Text:      "正在查找同一账号的电脑…",
										Font:      d.Font{Family: chineseUIFont, PointSize: 9},
										TextColor: palette.inkSecondary,
										MinSize:   d.Size{Height: 22},
									},
									d.ComboBox{
										AssignTo: &picker,
										Font:     d.Font{Family: chineseUIFont, PointSize: 10},
										MaxSize:  d.Size{Height: 34},
									},
									d.Composite{
										Background: themeBrush{&palette.cardColor},
										Layout:     d.HBox{Margins: d.Margins{Top: 4}, Spacing: 10},
										Children: []d.Widget{
											d.PushButton{
												AssignTo:  &connectButton,
												Text:      "配对所选电脑",
												MinSize:   d.Size{Width: 132, Height: 34},
												MaxSize:   d.Size{Height: 34},
												OnClicked: connect,
											},
											d.PushButton{
												Text:      "远程连接…",
												MinSize:   d.Size{Width: 112, Height: 34},
												MaxSize:   d.Size{Height: 34},
												OnClicked: remote,
											},
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
	styleDesktopUI(w, desktopStyleConfig{
		pathField:  saveDirEdit,
		nameEditor: nameField,
		buttonVariant: func(button *walk.PushButton) buttonVariant {
			if button == nav[0] || button == nav[1] || button == nav[2] {
				return buttonNav
			}
			if button == sendButton || button == pairButton || button == connectButton {
				return buttonPrimary
			}
			return buttonSecondary
		},
		isNavActive: func(button *walk.PushButton) bool {
			return button == nav[currentPage]
		},
		buttonSurface: func(button *walk.PushButton) walk.Color {
			if button == nav[0] || button == nav[1] || button == nav[2] {
				return palette.segTrackColor
			}
			if button == retryButton || button == inboxButton {
				return palette.canvasColor
			}
			return palette.cardColor
		},
		cards: map[*walk.Composite]bool{
			sendCard:       true,
			queueCard:      true,
			deviceInfoCard: true,
			lanPairCard:    true,
			receiveCard:    true,
			remoteCard:     true,
		},
		insetFrames: map[*walk.Composite]bool{
			queueFrame: true,
		},
		segContainers: map[*walk.Composite]bool{
			segBar: true,
		},
		statusBadges: map[*walk.CustomWidget]func() int{
			peerBadge: func() int { return connectedCount },
		},
	})
	watchDesktopAppearance(w)
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
	// Walk applies layout asynchronously. Show the dashboard after the first
	// layout pass has positioned its children, including any initial send UI.
	var layoutReady int
	layoutReady = pages[0].SizeChanged().Attach(func() {
		if pages[0].SizePixels().Width <= 0 {
			return
		}
		pages[0].SizeChanged().Detach(layoutReady)
		w.Synchronize(func() {
			w.Show()
			redrawDesktop(w)
			if len(initial) > 0 {
				sendFiles(initial)
			}
		})
	})
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		tick := time.NewTicker(5 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				w.Synchronize(func() {
					if currentPage == 1 {
						refreshCount()
					}
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

func manageDevices(w *walk.MainWindow, root string) {
	peers, err := peersAt(filepath.Join(root, "peer.json"))
	if err != nil {
		walk.MsgBox(w, "设备管理", err.Error(), walk.MsgBoxIconWarning)
		return
	}
	var dlg *walk.Dialog
	type deviceRow struct {
		group       *walk.Composite
		check       *walk.CheckBox
		note        *walk.LineEdit
		target      *walk.PushButton
		initialNote string
		removed     bool
	}
	rows := make([]deviceRow, len(peers))
	target := -1
	if len(peers) > 0 && peers[0].Enabled {
		target = 0
	}
	initialTarget := target
	updateTarget := func() {
		if target < 0 {
			for j, row := range rows {
				if !row.removed && row.check.Checked() {
					target = j
					break
				}
			}
		}
		for i, row := range rows {
			label := "设为发送目标"
			if i == target {
				label = "当前发送目标"
			}
			row.target.SetText(label)
			row.target.SetEnabled(i != target && row.check.Checked())
		}
	}
	var items []d.Widget
	for i, peer := range peers {
		row := &rows[i]
		row.initialNote = peerNickname(root, peer.PeerHash)
		name := peer.PeerName
		if name == "" {
			name = fmt.Sprintf("设备 %d", i+1)
		}
		items = append(items, d.Composite{AssignTo: &row.group, Background: themeBrush{&palette.cardColor}, Layout: d.HBox{Spacing: 8}, Children: []d.Widget{
			d.CheckBox{AssignTo: &row.check, Text: name, ToolTipText: name, Checked: peer.Enabled, MinSize: d.Size{Width: 120}, MaxSize: d.Size{Width: 120}, OnClicked: func() {
				if target == i && !row.check.Checked() {
					target = -1
				}
				updateTarget()
			}},
			d.LineEdit{AssignTo: &row.note, Text: row.initialNote, CueBanner: "设备备注", MinSize: d.Size{Width: 170}, StretchFactor: 1},
			d.PushButton{AssignTo: &row.target, Text: "设为发送目标", OnClicked: func() { target = i; updateTarget() }},
			d.PushButton{Text: "取消配对", OnClicked: func() {
				if walk.MsgBox(dlg, "取消配对", "保存时取消与「"+name+"」的配对？", walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
					return
				}
				row.removed = true
				row.group.SetVisible(false)
				if target == i {
					target = -1
				}
				updateTarget()
			}},
		}})
	}
	if len(items) == 0 {
		items = append(items, d.Label{Text: "暂无已配对设备"})
	}
	items = append(items, d.VSpacer{})
	save := func() {
		for i, row := range rows {
			if row.removed {
				continue
			}
			peer := peers[i]
			if row.note.Text() != row.initialNote {
				if err := setPeerNickname(root, peer.PeerHash, row.note.Text()); err != nil {
					row.note.SetFocus()
					walk.MsgBox(dlg, "设备备注", err.Error(), walk.MsgBoxIconWarning)
					return
				}
				rows[i].initialNote = row.note.Text()
			}
		}
		changed := target != initialTarget
		for i, row := range rows {
			changed = changed || row.removed || row.check.Checked() != peers[i].Enabled
		}
		if changed {
			err := editPeers(filepath.Join(root, "peer.json"), func(all []config) ([]config, error) {
				for i, row := range rows {
					var action string
					switch {
					case row.removed:
						action = "unpair"
					case row.check.Checked() == peers[i].Enabled:
						continue
					case row.check.Checked():
						action = "enable-peer"
					default:
						action = "disable-peer"
					}
					var err error
					all, err = applyPeerAction(all, peers[i].PeerHash, action)
					if err != nil {
						return nil, err
					}
				}
				if target >= 0 && target != initialTarget {
					return applyPeerAction(all, peers[target].PeerHash, "select-peer")
				}
				return all, nil
			})
			if err != nil {
				walk.MsgBox(dlg, "设备管理", err.Error(), walk.MsgBoxIconWarning)
				return
			}
		}
		dlg.Accept()
	}
	spec := d.Dialog{AssignTo: &dlg, Title: "设备管理", Font: d.Font{Family: chineseUIFont, PointSize: 10}, Background: themeBrush{&palette.cardColor}, MinSize: d.Size{Width: 600, Height: 230}, Layout: d.VBox{}, Children: []d.Widget{
		d.Label{Text: "勾选设备以允许收发，并加入右键菜单。"},
		d.ScrollView{HorizontalFixed: true, MinSize: d.Size{Height: 160}, Layout: d.VBox{MarginsZero: true, Spacing: 3}, Children: items},
		d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{d.HSpacer{}, d.PushButton{Text: "关闭", OnClicked: func() { dlg.Cancel() }}, d.PushButton{Text: "保存", Enabled: len(peers) > 0, OnClicked: save}}},
	}}
	if err := spec.Create(w); err != nil {
		walk.MsgBox(w, "设备管理", err.Error(), walk.MsgBoxIconWarning)
		return
	}
	updateTarget()
	styleDesktopUI(dlg, desktopStyleConfig{})
	watchDesktopAppearance(dlg)
	dlg.Run()
}
