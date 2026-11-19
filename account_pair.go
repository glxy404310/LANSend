package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"time"

	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tsnet"
)

type accountDevice struct{ ID, Name, IP string }

var errPeerOccupied = errors.New("已有另一台已配对电脑。如需更换，请使用手动配对")

func readPeer(path string) (config, error) {
	var c config
	b, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return c, nil
	}
	if e != nil {
		return c, e
	}
	e = json.Unmarshal(b, &c)
	return c, e
}
func accountDevices(st *ipnstate.Status) []accountDevice {
	devices := []accountDevice{}
	if st.Self == nil || st.Self.UserID == 0 || st.Self.IsTagged() || st.BackendState != "Running" {
		return devices
	}
	for _, p := range st.Peer {
		if p == nil || p.ID == "" || p.ID == st.Self.ID || !p.Online || p.UserID != st.Self.UserID || p.IsTagged() || p.ShareeNode || p.AltSharerUserID != 0 || !strings.HasPrefix(p.HostName, "lansend-") {
			continue
		}
		for _, ip := range p.TailscaleIPs {
			if ip.Is4() && tailnetIPv4.Contains(ip) {
				devices = append(devices, accountDevice{string(p.ID), p.HostName, ip.String()})
				break
			}
		}
	}
	sort.Slice(devices, func(i, j int) bool {
		if devices[i].Name == devices[j].Name {
			return devices[i].ID < devices[j].ID
		}
		return devices[i].Name < devices[j].Name
	})
	return devices
}
func (n *embeddedNode) accountSnapshot(ctx context.Context) (*tsnet.Server, *ipnstate.Status, error) {
	n.mu.Lock()
	s := n.server
	n.mu.Unlock()
	if s == nil {
		return nil, nil, errors.New("请先点“远程连接”，登录你的账号")
	}
	lc, e := s.LocalClient()
	if e != nil {
		return nil, nil, e
	}
	st, e := lc.Status(ctx)
	if e != nil {
		return nil, nil, e
	}
	if st.BackendState != "Running" || st.Self == nil {
		return nil, nil, errors.New("等待远程连接登录完成")
	}
	return s, st, nil
}

// Mounted only on the embedded tailnet listener. Derive the caller's address and identity from its connection.
func (n *embeddedNode) acceptAccountPair(w http.ResponseWriter, r *http.Request) {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		http.Error(w, "缺少设备证书", 403)
		return
	}
	s, st, e := n.accountSnapshot(r.Context())
	if e != nil {
		http.Error(w, "远程连接尚未就绪", 503)
		return
	}
	if string(st.Self.ID) != r.URL.Query().Get("target") {
		http.Error(w, "设备已经变更，请重新选择", 409)
		return
	}
	lc, e := s.LocalClient()
	if e != nil {
		http.Error(w, "远程连接未就绪", 503)
		return
	}
	who, e := lc.WhoIs(r.Context(), r.RemoteAddr)
	if e != nil || who.Node == nil {
		http.Error(w, "设备身份未确认", 403)
		return
	}
	source, e := netip.ParseAddrPort(r.RemoteAddr)
	if e != nil {
		http.Error(w, "地址无效", 403)
		return
	}
	for _, p := range accountDevices(st) {
		if p.ID == string(who.Node.StableID) && p.IP == source.Addr().Unmap().String() {
			c := config{Address: net.JoinHostPort(p.IP, port), PeerHash: digest(r.TLS.PeerCertificates[0].Raw)}
			if e := savePair(filepath.Join(n.root, "peer.json"), c, false); e != nil {
				http.Error(w, e.Error(), 409)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	http.Error(w, "仅允许同一账号的 LANSend 设备配对", 403)
}
func (n *embeddedNode) connectDevice(ctx context.Context, id string) error {
	s, st, e := n.accountSnapshot(ctx)
	if e != nil {
		return e
	}
	for _, p := range accountDevices(st) {
		if p.ID != id {
			continue
		}
		c, e := loadIdentity(n.root)
		if e != nil {
			return e
		}
		path := filepath.Join(n.root, "peer.json")
		old, e := readPeer(path)
		if e != nil {
			return e
		}
		c.PeerHash = old.PeerHash
		cl, _, e := client(c, c.PeerHash == "")
		if e != nil {
			return e
		}
		defer cl.CloseIdleConnections()
		tr := cl.Transport.(*http.Transport)
		tr.Proxy = nil
		tr.DialContext = s.Dial
		address := net.JoinHostPort(p.IP, port)
		req, e := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+address+api+"account-pair?target="+url.QueryEscape(id), nil)
		if e != nil {
			return e
		}
		response, e := cl.Do(req)
		if e != nil {
			return e
		}
		defer response.Body.Close()
		if response.StatusCode == 404 {
			return errors.New("请先将对方 LANSend 升级到 1.5 或更高版本")
		}
		if response.StatusCode != 204 {
			msg, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
			return fmt.Errorf("对方未完成配对：%s %s", response.Status, strings.TrimSpace(string(msg)))
		}
		return savePair(path, config{Address: address, PeerHash: digest(response.TLS.PeerCertificates[0].Raw)}, false)
	}
	return errors.New("对方已离线或已更换账号，请刷新设备列表")
}
func (n *embeddedNode) deviceControl(w http.ResponseWriter, r *http.Request, id string) {
	ctx, cancel := context.WithTimeout(r.Context(), 9*time.Second)
	defer cancel()
	if id != "" {
		if e := n.connectDevice(ctx, id); e != nil {
			http.Error(w, e.Error(), 409)
			return
		}
		json.NewEncoder(w).Encode(nodeStatus{})
		return
	}
	_, st, e := n.accountSnapshot(ctx)
	if e != nil {
		http.Error(w, e.Error(), 409)
		return
	}
	json.NewEncoder(w).Encode(nodeStatus{Devices: accountDevices(st)})
}
