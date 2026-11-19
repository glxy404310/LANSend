package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestKeepRoutes(t *testing.T) {
	local, remote := "192.168.1.2:53318", "100.80.1.3:53318"
	for _, tc := range []struct{ old, extra, next, pin, want, alt string }{
		{local, "", remote, "p", local, remote}, {remote, "", local, "p", local, remote}, {local, remote, "192.168.1.3:53318", "p", "192.168.1.3:53318", remote},
		{local, remote, "desk.example.ts.net:53318", "p", local, "desk.example.ts.net:53318"}, {local, remote, remote, "new", remote, ""},
	} {
		next := config{Address: tc.next, PeerHash: tc.pin}
		keepPeerRoutes(&next, config{Address: tc.old, RemoteAddress: tc.extra, PeerHash: "p"})
		if next.Address != tc.want || next.RemoteAddress != tc.alt {
			t.Fatal(next)
		}
	}
}
func TestRouteFallback(t *testing.T) {
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != api+"info" {
			t.Error(r.URL.Path)
		}
		w.Write([]byte(`{"version":"1"}`))
	}))
	defer s.Close()
	addr := s.Listener.Addr().String()
	c := config{Address: "127.0.0.1:0", RemoteAddress: addr, PeerHash: digest(s.Certificate().Raw)}
	if e := makeIdentity(&c); e != nil {
		t.Fatal(e)
	}
	if a, e := chooseRoute(c); e != nil || a != addr {
		t.Fatal(a, e)
	}
	c.Address, c.RemoteAddress = c.RemoteAddress, c.Address
	if a, e := chooseRoute(c); e != nil || a != addr {
		t.Fatal(a, e)
	}
	c.PeerHash = "wrong"
	if _, e := chooseRoute(c); e == nil {
		t.Fatal("wrong certificate accepted")
	}
}
