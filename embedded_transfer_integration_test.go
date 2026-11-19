//go:build integration

package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"tailscale.com/derp/derpserver"
	"tailscale.com/net/netns"
	"tailscale.com/tailcfg"
	"tailscale.com/tstest/integration/testcontrol"
	"tailscale.com/types/key"
	"tailscale.com/types/logger"
	"testing"
	"time"
)

func TestEmbeddedTransfer(t *testing.T) {
	netns.SetEnabled(false)
	defer netns.SetEnabled(true)
	relay := derpserver.New(key.NewNode(), logger.Discard)
	server := httptest.NewTLSServer(derpserver.Handler(relay))
	defer server.Close()
	defer relay.Close()
	dm := &tailcfg.DERPMap{Regions: map[tailcfg.DERPRegionID]*tailcfg.DERPRegion{1: {RegionID: 1, RegionCode: "test", Nodes: []*tailcfg.DERPNode{{Name: "test", RegionID: 1, HostName: "127.0.0.1", IPv4: "127.0.0.1", IPv6: "none", STUNPort: -1, DERPPort: server.Listener.Addr().(*net.TCPAddr).Port, InsecureForTests: true}}}}}
	control := &testcontrol.Server{AllOnline: true, AllNodesSameUser: true, DERPMap: dm, Logf: logger.Discard}
	control.HTTPTestServer = httptest.NewServer(control)
	defer control.HTTPTestServer.Close()
	t.Setenv("TS_CONTROL_URL", control.HTTPTestServer.URL)
	t.Setenv("TS_NO_LOGS_NO_SUPPORT", "true")
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	type machine struct {
		c       config
		n       *embeddedNode
		ip, box string
	}
	newMachine := func() machine {
		root := t.TempDir()
		box := t.TempDir()
		var c config
		if e := makeIdentity(&c); e != nil {
			t.Fatal(e)
		}
		b, _ := json.Marshal(c)
		if e := os.WriteFile(filepath.Join(root, "identity.json"), b, 0600); e != nil {
			t.Fatal(e)
		}
		cert, e := tls.X509KeyPair([]byte(c.Certificate), []byte(c.PrivateKey))
		if e != nil {
			t.Fatal(e)
		}
		node := &embeddedNode{root: root, self: digest(cert.Certificate[0]), tlsConfig: &tls.Config{Certificates: []tls.Certificate{cert}, ClientAuth: tls.RequestClientCert}, handler: routes(root, box, func(string) {})}
		if e := node.start(); e != nil {
			t.Fatal(e)
		}
		t.Cleanup(node.stop)
		st, e := node.server.Up(ctx)
		if e != nil {
			t.Fatal(e)
		}
		return machine{c, node, st.TailscaleIPs[0].String(), box}
	}
	a, b := newMachine(), newMachine()
	var found []accountDevice
	for i := 0; i < 120; i++ {
		_, st, e := a.n.accountSnapshot(ctx)
		if e == nil {
			found = accountDevices(st)
			if len(found) == 1 {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if len(found) != 1 {
		_, st, e := a.n.accountSnapshot(ctx)
		if e == nil {
			t.Logf("self: %s %v %v %s", st.Self.ID, st.Self.UserID, st.Self.IsTagged(), st.BackendState)
			for _, p := range st.Peer {
				t.Logf("peer: %s %s %v %v %v %v %v %v", p.ID, p.HostName, p.UserID, p.Online, p.ShareeNode, p.AltSharerUserID, p.IsTagged(), p.TailscaleIPs)
			}
		}
		t.Fatalf("discovery: %v err %v", found, e)
	}
	if e := a.n.connectDevice(ctx, found[0].ID); e != nil {
		t.Fatalf("one click: %v", e)
	}
	if e := a.n.connectDevice(ctx, found[0].ID); e != nil {
		t.Fatalf("repeat: %v", e)
	}
	for _, pair := range [][2]machine{{a, b}, {b, a}} {
		tx, rx := pair[0], pair[1]
		saved, e := readPeer(filepath.Join(rx.n.root, "peer.json"))
		if e != nil || saved.PeerHash != tx.n.self {
			t.Fatal("reciprocal pairing missing", e)
		}
		c := tx.c
		c.PeerHash = rx.n.self
		cl, _, e := client(c, false)
		if e != nil {
			t.Fatal(e)
		}
		st, e := tx.n.status(ctx, true)
		if e != nil {
			t.Fatal(e)
		}
		cl.Transport.(*http.Transport).Proxy = http.ProxyURL(&url.URL{Scheme: "socks5", Host: st.ProxyAddr, User: url.UserPassword("tsnet", st.ProxyPassword)})
		cl.Timeout = 20 * time.Second
		src := filepath.Join(t.TempDir(), "跨网 test.txt")
		payload := []byte("two embedded nodes")
		if e = os.WriteFile(src, payload, 0600); e != nil {
			t.Fatal(e)
		}
		if e = upload(cl, rx.ip+":"+port, src, func(int) {}); e != nil {
			t.Fatal(e)
		}
		got, e := os.ReadFile(filepath.Join(rx.box, filepath.Base(src)))
		if e != nil || string(got) != string(payload) {
			t.Fatalf("file mismatch: %v", e)
		}
		cl.CloseIdleConnections()
	}
}
