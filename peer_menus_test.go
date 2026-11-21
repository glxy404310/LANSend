package main

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedDeviceMenus(t *testing.T) {
	root, services := t.TempDir(), t.TempDir()
	path := filepath.Join(root, "peer.json")
	a, b := strings.Repeat("a", 64), strings.Repeat("b", 64)
	// The legacy record keeps its identity and requires an explicit enable.
	if e := os.WriteFile(path, []byte(`{"PeerHash":"`+a+`","Address":"legacy:53318"}`), 0600); e != nil {
		t.Fatal(e)
	}
	if e := savePair(path, config{PeerHash: b, PeerName: "Mac"}); e != nil {
		t.Fatal(e)
	}
	sync := func(n int) []menuDevice {
		t.Helper()
		m, e := menuDevices(root)
		if e != nil || len(m) != n {
			t.Fatalf("menus %v %v", m, e)
		}
		if e = syncFinderMenus(services, m); e != nil {
			t.Fatal(e)
		}
		entries, e := os.ReadDir(services)
		if e != nil || len(entries) != n {
			t.Fatal(entries, e)
		}
		return m
	}
	change := func(h, op string) {
		t.Helper()
		if e := changePeer(root, h, op); e != nil {
			t.Fatal(e)
		}
	}
	sync(0)
	change(a, "enable-peer")
	change(b, "enable-peer")
	sync(2)
	if e := setPeerNickname(root, b, "工作室 & Mac"); e != nil {
		t.Fatal(e)
	}
	m := sync(2)
	if !strings.Contains(m[1].Name, "工作室 & Mac") {
		t.Fatal(m)
	}
	dir := filepath.Join(services, m[1].Name+".workflow", "Contents")
	info, _ := os.ReadFile(filepath.Join(dir, "Info.plist"))
	var v struct{}
	if e := xml.Unmarshal(info, &v); e != nil {
		t.Fatal(e)
	}
	flow, _ := os.ReadFile(filepath.Join(dir, "document.wflow"))
	if !strings.Contains(string(flow), "send-to "+b) {
		t.Fatal("wrong Finder target")
	}
	change(b, "select-peer")
	change(b, "disable-peer")
	sync(1)
	current, _ := readPeer(path)
	if current.PeerHash != a {
		t.Fatal("no enabled default")
	}
	if p, e := findPeer(path, b); e != nil || p.Enabled || peerNickname(root, b) != "工作室 & Mac" {
		t.Fatal("disabled peer lost", e)
	}
	change(a, "unpair")
	sync(0)
	if e := sendTo(root, a, []string{"file"}, nil); e == nil {
		t.Fatal("stale shortcut accepted")
	}
}
