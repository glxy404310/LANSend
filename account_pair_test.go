package main

import (
	"net/netip"
	"path/filepath"
	"testing"

	"tailscale.com/ipn/ipnstate"
	"tailscale.com/types/key"
	"tailscale.com/types/views"
)

func TestAccountDeviceEligibility(t *testing.T) {
	self := &ipnstate.PeerStatus{ID: "self", UserID: 42}
	peer := ipnstate.PeerStatus{ID: "remote", UserID: 42, Online: true, HostName: "lansend-windows-abc123", TailscaleIPs: []netip.Addr{netip.MustParseAddr("100.80.0.1")}}
	tags := views.SliceOf([]string{"tag:team"})
	for name, change := range map[string]func(*ipnstate.PeerStatus){"same": func(p *ipnstate.PeerStatus) {}, "other": func(p *ipnstate.PeerStatus) { p.UserID++ }, "tagged": func(p *ipnstate.PeerStatus) { p.Tags = &tags }, "shared": func(p *ipnstate.PeerStatus) { p.ShareeNode = true }, "offline": func(p *ipnstate.PeerStatus) { p.Online = false }, "self": func(p *ipnstate.PeerStatus) { p.ID = self.ID }, "other app": func(p *ipnstate.PeerStatus) { p.HostName = "laptop" }} {
		p := peer
		change(&p)
		st := ipnstate.Status{BackendState: "Running", Self: self, Peer: map[key.NodePublic]*ipnstate.PeerStatus{{}: &p}}
		if got := len(accountDevices(&st)); (got == 1) != (name == "same") {
			t.Fatalf("%s: %d", name, got)
		}
	}
}
func TestAccountPairPreservesExistingPeer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "peer.json")
	a := config{Address: "192.168.0.8:53318", PeerHash: "a"}
	if e := savePair(path, a, true); e != nil {
		t.Fatal(e)
	}
	b := config{Address: "100.80.0.8:53318", PeerHash: "b"}
	if e := savePair(path, b, false); e != errPeerOccupied {
		t.Fatal(e)
	}
	b.PeerHash = a.PeerHash
	if e := savePair(path, b, false); e != nil {
		t.Fatal(e)
	}
	c, e := readPeer(path)
	if e != nil || c.Address != a.Address || c.RemoteAddress != b.Address {
		t.Fatal(c, e)
	}
}
