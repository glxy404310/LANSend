package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStandalone(t *testing.T) {
	root, dest, src := t.TempDir(), t.TempDir(), t.TempDir()
	var tx config
	if e := makeIdentity(&tx); e != nil {
		t.Fatal(e)
	}
	_, pin, e := client(tx, false)
	if e != nil {
		t.Fatal(e)
	}
	events := make(chan string, 8)
	server := httptest.NewUnstartedServer(routes(root, dest, func(path string) { events <- path }))
	server.TLS = &tls.Config{ClientAuth: tls.RequestClientCert}
	server.StartTLS()
	defer server.Close()
	tx.Address = server.Listener.Addr().String()
	tx.PeerHash = digest(server.Certificate().Raw)
	p := filepath.Join(src, "甲 & b.txt")
	payload := bytes.Repeat([]byte("中文 hello"), 90000)
	if e = os.WriteFile(p, payload, 0600); e != nil {
		t.Fatal(e)
	}
	empty := filepath.Join(src, "空 文件")
	if e = os.WriteFile(empty, nil, 0600); e != nil {
		t.Fatal(e)
	}
	if e = send(tx, []string{p}); e == nil {
		t.Fatal("unpaired accepted")
	}
	b, _ := json.Marshal(config{PeerHash: pin})
	if e = os.WriteFile(filepath.Join(root, "peer.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
	if e = send(tx, []string{p, empty, p}); e != nil {
		t.Fatal(e)
	}
	for _, n := range []string{"甲 & b.txt", "甲 & b (1).txt", "空 文件"} {
		got, e := os.ReadFile(filepath.Join(dest, n))
		if e != nil {
			t.Fatal(e)
		}
		if n == "空 文件" && len(got) != 0 || n != "空 文件" && !bytes.Equal(got, payload) {
			t.Fatal(n)
		}
	}
	for _, name := range []string{"甲 & b.txt", "空 文件", "甲 & b (1).txt"} {
		select {
		case path := <-events:
			if path != filepath.Join(dest, name) {
				t.Fatal(path)
			}
		default:
			t.Fatal("missing notification")
		}
	}
	if len(events) != 0 {
		t.Fatal("extra notification")
	}
	tx.PeerHash = "other"
	if e = send(tx, []string{p}); e == nil {
		t.Fatal("wrong receiver accepted")
	}
	if _, e = saveFile(dest, "partial", strings.NewReader("x"), 5); e == nil {
		t.Fatal("short upload accepted")
	}
	list, e := os.ReadDir(dest)
	if e != nil || len(list) != 3 {
		t.Fatal("partial file leaked", e)
	}
	for _, n := range []string{"../x", "..", "a/b", "a\\b", "a:b", "x.", ""} {
		if safeName(n) {
			t.Fatal(n)
		}
	}
}
