package main

import (
	"archive/zip"
	"bytes"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFolderDelivery(t *testing.T) {
	source := filepath.Join(t.TempDir(), "设计稿")
	if err := os.MkdirAll(filepath.Join(source, "子目录", "空"), 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(source, "子目录", "草稿.txt")
	if err := os.WriteFile(file, []byte("version 1"), 0600); err != nil {
		t.Fatal(err)
	}
	packed, err := packFolder(source)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(packed)
	data, err := os.ReadFile(packed)
	if err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	for _, name := range []string{"设计稿", "设计稿 (1)"} {
		path, err := saveFolder(dest, "设计稿", bytes.NewReader(data), int64(len(data)))
		if err != nil || filepath.Base(path) != name {
			t.Fatal(path, err)
		}
		b, e := os.ReadFile(filepath.Join(path, "子目录", "草稿.txt"))
		if e != nil || string(b) != "version 1" {
			t.Fatal(e)
		}
		st, e := os.Stat(filepath.Join(path, "子目录", "空"))
		if e != nil || !st.IsDir() {
			t.Fatal(e)
		}
	}
	if _, err = saveFolder(dest, "bad", bytes.NewReader(data[:len(data)/2]), int64(len(data))); err == nil {
		t.Fatal("published incomplete folder")
	}
	var invalid bytes.Buffer
	zw := zip.NewWriter(&invalid)
	w, _ := zw.CreateHeader(&zip.FileHeader{Name: "../escape", Method: zip.Store})
	w.Write([]byte("x"))
	zw.Close()
	if _, err = saveFolder(dest, "bad", &invalid, int64(invalid.Len())); err == nil {
		t.Fatal("accepted escaping member")
	}
	entries, err := os.ReadDir(dest)
	if err != nil || len(entries) != 2 {
		t.Fatal("unexpected partial folder or temp file", entries, err)
	}
}

func TestRediscoverAndRename(t *testing.T) {
	sender, receiver := t.TempDir(), t.TempDir()
	for _, root := range []string{sender, receiver} {
		if e := initialize(root); e != nil {
			t.Fatal(e)
		}
	}
	id, e := loadIdentity(receiver)
	if e != nil {
		t.Fatal(e)
	}
	cert, e := tls.X509KeyPair([]byte(id.Certificate), []byte(id.PrivateKey))
	if e != nil {
		t.Fatal(e)
	}
	srv := httptest.NewUnstartedServer(routes(receiver, func(string) {}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, ClientAuth: tls.RequestClientCert}
	srv.StartTLS()
	defer srv.Close()
	wrong := httptest.NewTLSServer(routes(receiver, func(string) {}))
	defer wrong.Close()
	c, e := loadIdentity(sender)
	if e != nil {
		t.Fatal(e)
	}
	c.PeerHash = digest(cert.Certificate[0])
	c.Address = strings.TrimPrefix(wrong.URL, "https://")
	peerFile := filepath.Join(sender, "peer.json")
	if e = savePair(peerFile, c, true); e != nil {
		t.Fatal(e)
	}
	addr := strings.TrimPrefix(srv.URL, "https://")
	if e = setDeviceName(receiver, "工作室 Mac"); e != nil {
		t.Fatal(e)
	}
	if _, _, e = resolvePeerUsing(c, func(string) (string, error) { return c.Address, nil }); !errors.Is(e, errPeerCertificateChanged) {
		t.Fatal("unexpected certificate was accepted", e)
	}
	found, alias, e := resolvePeerUsing(c, func(hash string) (string, error) {
		if hash != c.PeerHash {
			t.Fatal("wrong identity lookup")
		}
		return addr, nil
	})
	if e != nil || found != addr || alias != "工作室 Mac" {
		t.Fatal(found, alias, e)
	}
	old, e := readPeer(peerFile)
	if e != nil || old.Address != addr || old.PeerHash != c.PeerHash || old.PeerName != alias {
		t.Fatal("rediscovered peer not saved", e)
	}
	if e = setDeviceName(receiver, "客厅电脑"); e != nil {
		t.Fatal(e)
	}
	if status := peerStatus(sender); status != "客厅电脑 · 在线" {
		t.Fatal(status)
	}
	if e = setPeerNickname(sender, c.PeerHash, "备用电脑"); e != nil {
		t.Fatal(e)
	}
	if status := peerStatus(sender); status != "备用电脑 · 在线" {
		t.Fatal(status)
	}
	if e = setPeerNickname(sender, c.PeerHash, ""); e != nil {
		t.Fatal(e)
	}
	srv.Close()
	if status := peerStatus(sender); status != "客厅电脑 · 离线" {
		t.Fatal(status)
	}
	// The last authenticated name remains available for an offline view.
	old, e = readPeer(peerFile)
	if e != nil || old.PeerName != "客厅电脑" {
		t.Fatal(old, e)
	}
}

func TestDiscoveryDatagram(t *testing.T) {
	loop := net.IPv4(127, 0, 0, 1)
	sock, e := net.ListenUDP("udp4", &net.UDPAddr{IP: loop})
	if e != nil {
		t.Fatal(e)
	}
	defer sock.Close()
	go answerDiscovery(sock, "paired-identity")
	address, e := discoverAt("paired-identity", []net.IP{loop}, sock.LocalAddr().(*net.UDPAddr).Port)
	if e != nil || address != net.JoinHostPort(loop.String(), port) {
		t.Fatal(address, e)
	}
}

func TestFileAndFolderThroughReceiver(t *testing.T) {
	sender, receiver := t.TempDir(), t.TempDir()
	for _, root := range []string{sender, receiver} {
		if e := initialize(root); e != nil {
			t.Fatal(e)
		}
	}
	dest := t.TempDir()
	if _, e := setReceiveDir(receiver, dest); e != nil {
		t.Fatal(e)
	}
	c, e := loadIdentity(receiver)
	if e != nil {
		t.Fatal(e)
	}
	cert, e := tls.X509KeyPair([]byte(c.Certificate), []byte(c.PrivateKey))
	if e != nil {
		t.Fatal(e)
	}
	peer, e := identityHash(sender)
	if e != nil {
		t.Fatal(e)
	}
	if e = savePair(filepath.Join(receiver, "peer.json"), config{PeerHash: peer}, true); e != nil {
		t.Fatal(e)
	}
	server := httptest.NewUnstartedServer(routes(receiver, func(string) {}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, ClientAuth: tls.RequestClientCert}
	server.StartTLS()
	defer server.Close()
	hash, e := identityHash(receiver)
	if e != nil {
		t.Fatal(e)
	}
	if e = savePair(filepath.Join(sender, "peer.json"), config{Address: strings.TrimPrefix(server.URL, "https://"), PeerHash: hash}, true); e != nil {
		t.Fatal(e)
	}
	base := t.TempDir()
	folder := filepath.Join(base, "empty")
	os.Mkdir(folder, 0700)
	file := filepath.Join(base, "file.txt")
	os.WriteFile(file, []byte("abc"), 0600)
	if e = sendSaved(sender, []string{file, folder}, nil); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(dest, "file.txt"))
	if e != nil || string(b) != "abc" {
		t.Fatal(e)
	}
	st, e := os.Stat(filepath.Join(dest, "empty"))
	if e != nil || !st.IsDir() {
		t.Fatal(e)
	}
	req, _ := http.NewRequest("PUT", server.URL+api+"folder?name=forbidden", bytes.NewReader(nil))
	c.PeerHash = hash
	unauthorised, _, e := client(c, false)
	if e != nil {
		t.Fatal(e)
	}
	defer unauthorised.CloseIdleConnections()
	resp, e := unauthorised.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatal("accepted unpaired folder", resp.Status)
	}
}
