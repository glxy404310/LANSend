package main

import (
	"context"
	"crypto/tls"
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
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tsnet"
)

type accountDevice struct{ ID, Name, IP string }

type peerAliasEntry struct {
	ip      string
	alias   string
	expires time.Time
}

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
			c := config{PeerName: p.Name, Address: net.JoinHostPort(p.IP, port), PeerHash: digest(r.TLS.PeerCertificates[0].Raw)}
			if e := savePair(filepath.Join(n.root, "peer.json"), c); e != nil {
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
		all, e := peersAt(path)
		if e != nil {
			return e
		}
		for _, old := range all {
			if old.Address == net.JoinHostPort(p.IP, port) || old.RemoteAddress == net.JoinHostPort(p.IP, port) {
				c.PeerHash = old.PeerHash
				break
			}
		}
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
		return savePair(path, config{PeerName: p.Name, Address: address, PeerHash: digest(response.TLS.PeerCertificates[0].Raw)})
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
	s, st, e := n.accountSnapshot(ctx)
	if e != nil {
		http.Error(w, e.Error(), 409)
		return
	}
	json.NewEncoder(w).Encode(nodeStatus{Devices: n.enrichAccountDevices(ctx, s, st, accountDevices(st))})
}

func sanitizeDisplayName(raw string) string {
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, raw)
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	if len(cleaned) > len(".local") && strings.EqualFold(cleaned[len(cleaned)-len(".local"):], ".local") {
		cleaned = strings.TrimSpace(cleaned[:len(cleaned)-len(".local")])
	}
	if utf8.RuneCountInString(cleaned) > 48 {
		cleaned = string([]rune(cleaned)[:47]) + "…"
	}
	return cleaned
}

func parseNodeMeta(hostName, peerOS string) (string, string) {
	osName := func(raw string) string {
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "windows":
			return "Windows"
		case "darwin", "macos":
			return "macOS"
		case "linux":
			return "Linux"
		default:
			return ""
		}
	}
	var osLabel, shortID string
	if rest, ok := strings.CutPrefix(hostName, "lansend-"); ok {
		if i := strings.LastIndexByte(rest, '-'); i > 0 && i+1 < len(rest) {
			osLabel = osName(rest[:i])
			shortID = rest[i+1:]
		}
	}
	if osLabel == "" {
		osLabel = osName(peerOS)
	}
	return osLabel, shortID
}

func customTailscaleAlias(dnsName, hostName string) string {
	label := strings.TrimSuffix(strings.TrimSpace(dnsName), ".")
	if i := strings.IndexByte(label, '.'); i >= 0 {
		label = label[:i]
	}
	label = sanitizeDisplayName(label)
	if label == "" {
		return ""
	}
	base := strings.ToLower(strings.TrimSpace(hostName))
	lower := strings.ToLower(label)
	if base != "" && (lower == base || strings.HasPrefix(lower, base+"-")) {
		return ""
	}
	return label
}

func formatDeviceName(hostName, dnsName, peerOS, remoteAlias string) string {
	osLabel, shortID := parseNodeMeta(hostName, peerOS)
	primary := customTailscaleAlias(dnsName, hostName)
	if primary == "" {
		remote := sanitizeDisplayName(remoteAlias)
		if remote != "" && !strings.EqualFold(remote, hostName) {
			primary = remote
		}
	}
	if primary != "" {
		switch {
		case osLabel != "" && shortID != "":
			return fmt.Sprintf("%s (%s · %s)", primary, osLabel, shortID)
		case shortID != "":
			return fmt.Sprintf("%s (%s)", primary, shortID)
		case osLabel != "":
			return fmt.Sprintf("%s (%s)", primary, osLabel)
		default:
			return primary
		}
	}
	if osLabel != "" && shortID != "" {
		return fmt.Sprintf("%s (%s)", osLabel, shortID)
	}
	return hostName
}

func probePeerAlias(ctx context.Context, cl *http.Client, ip string) (string, bool) {
	pctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(pctx, http.MethodGet, "https://"+net.JoinHostPort(ip, port)+api+"info", nil)
	if err != nil {
		return "", false
	}
	resp, err := cl.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false
	}
	var info struct{ Alias, Version string }
	if json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&info) != nil || info.Version != "1" {
		return "", false
	}
	alias := sanitizeDisplayName(info.Alias)
	if alias == "" {
		return "", false
	}
	return alias, true
}

func (n *embeddedNode) enrichAccountDevices(ctx context.Context, s *tsnet.Server, st *ipnstate.Status, devices []accountDevice) []accountDevice {
	if len(devices) == 0 {
		return devices
	}
	peersByID := make(map[string]*ipnstate.PeerStatus, len(st.Peer))
	for _, p := range st.Peer {
		if p != nil && p.ID != "" {
			peersByID[string(p.ID)] = p
		}
	}
	aliases := make([]string, len(devices))
	var pending []int
	now := time.Now()
	n.mu.Lock()
	for i, d := range devices {
		p := peersByID[d.ID]
		if p != nil && customTailscaleAlias(p.DNSName, d.Name) != "" {
			continue
		}
		if entry, ok := n.aliases[d.ID]; ok && entry.ip == d.IP && now.Before(entry.expires) {
			aliases[i] = entry.alias
			continue
		}
		pending = append(pending, i)
	}
	n.mu.Unlock()

	if len(pending) > 0 {
		cl := &http.Client{
			Transport: &http.Transport{
				DialContext:           s.Dial,
				TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true},
				TLSHandshakeTimeout:   1500 * time.Millisecond,
				ResponseHeaderTimeout: 1500 * time.Millisecond,
			},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
		defer cl.CloseIdleConnections()
		probed := make([]string, len(pending))
		okList := make([]bool, len(pending))
		sem := make(chan struct{}, 8)
		var wg sync.WaitGroup
		for j, idx := range pending {
			wg.Add(1)
			go func(pos, devIdx int, ip string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				probed[pos], okList[pos] = probePeerAlias(ctx, cl, ip)
			}(j, idx, devices[idx].IP)
		}
		wg.Wait()
		fetchedAt := time.Now()
		n.mu.Lock()
		if n.server == s {
			if n.aliases == nil {
				n.aliases = make(map[string]peerAliasEntry)
			}
			for j, idx := range pending {
				d := devices[idx]
				ttl := 15 * time.Second
				if okList[j] {
					ttl = 60 * time.Second
					aliases[idx] = probed[j]
				}
				n.aliases[d.ID] = peerAliasEntry{ip: d.IP, alias: probed[j], expires: fetchedAt.Add(ttl)}
			}
			if len(n.aliases) > 64 {
				for id := range n.aliases {
					if _, alive := peersByID[id]; !alive {
						delete(n.aliases, id)
					}
				}
			}
		} else {
			for j, idx := range pending {
				if okList[j] {
					aliases[idx] = probed[j]
				}
			}
		}
		n.mu.Unlock()
	}

	for i, d := range devices {
		var dnsName, peerOS string
		if p := peersByID[d.ID]; p != nil {
			dnsName = p.DNSName
			peerOS = p.OS
		}
		devices[i].Name = formatDeviceName(d.Name, dnsName, peerOS, aliases[i])
	}
	sort.Slice(devices, func(i, j int) bool {
		if devices[i].Name == devices[j].Name {
			return devices[i].ID < devices[j].ID
		}
		return devices[i].Name < devices[j].Name
	})
	return devices
}
