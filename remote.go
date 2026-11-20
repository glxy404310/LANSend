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
	"path/filepath"
	"strings"
	"time"
)

var tailnetIPv4 = netip.MustParsePrefix("100.64.0.0/10")

func isRemoteAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	ip, err := netip.ParseAddr(host)
	return (err == nil && tailnetIPv4.Contains(ip)) || strings.HasSuffix(strings.TrimSuffix(strings.ToLower(host), "."), ".ts.net")
}

// Address is always usable by older clients. Retain a second route only for the same certificate.
func keepPeerRoutes(next *config, old config) {
	if old.PeerHash != next.PeerHash || old.Address == "" {
		return
	}
	if next.PeerName == "" {
		next.PeerName = old.PeerName
	}
	if isRemoteAddress(next.Address) {
		if !isRemoteAddress(old.Address) {
			next.RemoteAddress = next.Address
			next.Address = old.Address
		}
	} else if isRemoteAddress(old.Address) {
		next.RemoteAddress = old.Address
	} else {
		next.RemoteAddress = old.RemoteAddress
	}
}

func chooseRoute(c config) (string, error) {
	address, _, err := resolvePeer(c)
	return address, err
}
func resolvePeer(c config) (string, string, error) {
	return resolvePeerUsing(c, discoverPeer)
}
func resolvePeerUsing(c config, lookup func(string) (string, error)) (string, string, error) {
	cl, _, err := client(c, false)
	if err != nil {
		return "", "", err
	}
	defer cl.CloseIdleConnections()
	var failures []error
	try := func(address string, timeout time.Duration, discovered bool) (string, bool) {
		alias, e := probeRoute(cl, address, timeout)
		if e != nil {
			failures = append(failures, e)
			return "", false
		}
		if discovered || (alias != "" && alias != c.PeerName) {
			next := config{Address: address, PeerHash: c.PeerHash, PeerName: alias}
			e = savePairExpected(filepath.Join(c.root, "peer.json"), next, false, &c.PeerHash)
			if e != nil {
				failures = append(failures, e)
				return "", false
			}
		}
		return alias, true
	}
	if alias, ok := try(c.Address, 2*time.Second, false); ok {
		return c.Address, alias, nil
	}
	if candidate, e := lookup(c.PeerHash); e == nil && candidate != c.Address {
		if alias, ok := try(candidate, 2*time.Second, true); ok {
			return candidate, alias, nil
		}
	}
	if c.RemoteAddress != "" {
		if alias, ok := try(c.RemoteAddress, 10*time.Second, false); ok {
			return c.RemoteAddress, alias, nil
		}
	} else if isRemoteAddress(c.Address) {
		if alias, ok := try(c.Address, 10*time.Second, false); ok {
			return c.Address, alias, nil
		}
	}
	return "", "", fmt.Errorf("设备未能连接：%w", errors.Join(failures...))
}
func probeRoute(cl *http.Client, address string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", "https://"+address+api+"info", nil)
	if err != nil {
		return "", err
	}
	res, err := cl.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var info struct{ Version, Alias string }
	if res.StatusCode != 200 || json.NewDecoder(io.LimitReader(res.Body, 65536)).Decode(&info) != nil || info.Version != "1" {
		return "", errors.New("接收器未就绪")
	}
	return sanitizeDisplayName(info.Alias), nil
}
