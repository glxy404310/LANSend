package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"
)

//go:embed "mac-desktop/用 LANSend 发送.workflow/Contents/Info.plist"
var serviceInfo string

//go:embed "mac-desktop/用 LANSend 发送.workflow/Contents/document.wflow"
var serviceFlow string

//go:embed windows-ui/DeviceMenus.ps1
var menuScript string

type menuDevice struct{ Name, Hash string }

func menuDevices(root string) ([]menuDevice, error) {
	peers, e := peersAt(filepath.Join(root, "peer.json"))
	if e != nil {
		return nil, e
	}
	out := []menuDevice{}
	for _, p := range peers {
		if !p.Enabled {
			continue
		}
		if _, e := nicknameFile(root, p.PeerHash); e != nil {
			return nil, e
		}
		label := strings.Map(func(r rune) rune {
			if unicode.IsControl(r) || strings.ContainsRune(`<>:"/\|?*`, r) {
				return '_'
			}
			return r
		}, peerLabel(root, p))
		r := []rune(label)
		if len(r) > 48 {
			label = string(r[:48])
		}
		out = append(out, menuDevice{"LANSend · " + label, p.PeerHash})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Hash < out[j].Hash })
	used := make(map[string]bool)
	for i, d := range out {
		label := d.Name
		for n := 2; used[strings.ToLower(label)]; n++ {
			label = fmt.Sprintf("%s (%d)", d.Name, n)
		}
		out[i].Name = label
		used[strings.ToLower(label)] = true
	}
	return out, nil
}
func watchPeerMenus(root string, stop <-chan struct{}) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	last := ""
	for {
		devices, e := menuDevices(root)
		if e == nil {
			b, _ := json.Marshal(devices)
			if string(b) != last {
				if e = syncDeviceMenus(devices, b); e == nil {
					last = string(b)
				} else {
					fmt.Fprintln(os.Stderr, e)
				}
			}
		}
		select {
		case <-stop:
			return
		case <-ticker.C:
		}
	}
}
func syncDeviceMenus(devices []menuDevice, data []byte) error {
	switch runtime.GOOS {
	case "windows":
		exe, e := os.Executable()
		if e != nil {
			return e
		}
		cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", menuScript)
		cmd.Env = append(os.Environ(), "LANSEND_DEVICE_MENUS="+string(data), "LANSEND_MENU_EXE="+filepath.Join(filepath.Dir(exe), "LANSend-Desktop.exe"))
		hideMenuProcess(cmd)
		if out, e := cmd.CombinedOutput(); e != nil {
			return fmt.Errorf("发送菜单：%w %s", e, out)
		}
	case "darwin":
		home, e := os.UserHomeDir()
		if e != nil {
			return e
		}
		if e = syncFinderMenus(filepath.Join(home, "Library", "Services"), devices); e != nil {
			return e
		}
		_ = exec.Command("/System/Library/CoreServices/pbs", "-flush").Run()
	}
	return nil
}
func syncFinderMenus(dir string, devices []menuDevice) error {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	keep := map[string]bool{}
	for _, d := range devices {
		var label bytes.Buffer
		xml.EscapeText(&label, []byte(d.Name))
		info := strings.ReplaceAll(strings.ReplaceAll(serviceInfo, "用 LANSend 发送", label.String()), "local.lansend.finder.send", "local.lansend.finder.device."+d.Hash)
		flow := strings.Replace(serviceFlow, `exec /usr/bin/open -b local.lansend.desktop "$@"`, `exec "$HOME/Library/Application Support/LANSend/lansend" send-to `+d.Hash+` "$@"`, 1)
		name := d.Name + ".workflow"
		keep[name] = true
		dest := filepath.Join(dir, name, "Contents")
		if b, e := os.ReadFile(filepath.Join(dest, "Info.plist")); e == nil && !strings.Contains(string(b), "local.lansend.finder.device.") {
			return fmt.Errorf("快速操作名称已被占用：%s", name)
		}
		if e := os.MkdirAll(dest, 0700); e != nil {
			return e
		}
		for f, v := range map[string]string{"Info.plist": info, "document.wflow": flow} {
			if e := os.WriteFile(filepath.Join(dest, f), []byte(v), 0600); e != nil {
				return e
			}
		}
	}
	entries, e := os.ReadDir(dir)
	if e != nil {
		return e
	}
	for _, f := range entries {
		if keep[f.Name()] || !strings.HasPrefix(f.Name(), "LANSend · ") || !strings.HasSuffix(f.Name(), ".workflow") {
			continue
		}
		p := filepath.Join(dir, f.Name())
		info, _ := os.ReadFile(filepath.Join(p, "Contents", "Info.plist"))
		if strings.Contains(string(info), "local.lansend.finder.device.") {
			if e := os.RemoveAll(p); e != nil {
				return e
			}
		}
	}
	return nil
}
