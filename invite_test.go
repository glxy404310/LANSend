package main

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLANInvitePairing(t *testing.T) {
	a, b, c := t.TempDir(), t.TempDir(), t.TempDir()
	for _, root := range []string{a, b, c} {
		if err := initialize(root); err != nil {
			t.Fatal(err)
		}
	}
	pin, err := identityHash(a)
	if err != nil {
		t.Fatal(err)
	}
	id, err := loadIdentity(a)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := tls.X509KeyPair([]byte(id.Certificate), []byte(id.PrivateKey))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	inv := new(lanInvite)
	inv.register(mux, a, pin)
	srv := httptest.NewUnstartedServer(mux)
	srv.Listener.Close()
	srv.Listener, err = net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, ClientAuth: tls.RequestClientCert}
	srv.StartTLS()
	defer srv.Close()
	_, portNumber, _ := net.SplitHostPort(srv.Listener.Addr().String())
	local := net.JoinHostPort("127.0.0.1", portNumber)
	if _, err = invitePost(c, local, pin, "invite/new", ""); err == nil {
		t.Fatal("foreign device issued invitation")
	}
	if primaryLocalIPv4() == "" {
		t.Skip("no active LAN interface")
	}
	issue := func(expected string) string {
		t.Helper()
		code, e := invitePost(a, local, pin, "invite/new", expected)
		if e != nil {
			t.Fatal(e)
		}
		addr, _, _, e := parseInvite(code)
		if e != nil {
			t.Fatal(e)
		}
		host, _, _ := net.SplitHostPort(addr)
		return strings.Replace(code, addr, net.JoinHostPort(host, portNumber), 1)
	}
	old := issue("")
	code := issue("")
	if joinInvite(b, old, "") == nil {
		t.Fatal("superseded invitation worked")
	}
	if joinInvite(a, code, "") == nil {
		t.Fatal("self pairing worked")
	}
	if err = joinInvite(b, code, ""); err != nil {
		t.Fatal(err)
	}
	pb, _ := identityHash(b)
	assertPeer := func(root, hash string) {
		t.Helper()
		peer, e := findPeer(filepath.Join(root, "peer.json"), hash)
		if e != nil || peer.PeerHash != hash || peer.Address == "" {
			t.Fatal("incorrect pairing", e)
		}
	}
	assertPeer(a, pb)
	assertPeer(b, pin)
	if joinInvite(c, code, "") == nil {
		t.Fatal("another certificate reused invitation")
	}
	if err = joinInvite(b, code, ""); err != nil {
		t.Fatal("lost-response retry", err)
	}
	code = issue(pb)
	inv.Lock()
	inv.expires = time.Now().Add(-time.Second)
	inv.Unlock()
	if joinInvite(c, code, "") == nil {
		t.Fatal("expired invitation worked")
	}
	assertPeer(a, pb)
	code = issue(pb)
	if err = joinInvite(c, code, ""); err != nil {
		t.Fatal(err)
	}
	pc, _ := identityHash(c)
	assertPeer(a, pb)
	assertPeer(a, pc)
	if e := changePeer(a, pc, "unpair"); e != nil {
		t.Fatal(e)
	}
	if joinInvite(c, code, "") == nil {
		t.Fatal("removed device restored using claimed invitation")
	}
	assertPeer(a, pb)
	assertPeer(c, pin)
}
