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
	if c.RemoteAddress == "" {
		return c.Address, nil
	}
	cl, _, err := client(c, false)
	if err != nil {
		return "", err
	}
	defer cl.CloseIdleConnections()
	var failures []error
	for i, address := range []string{c.Address, c.RemoteAddress} {
		timeout := 2 * time.Second
		if i == 1 {
			timeout = 12 * time.Second
		}
		if err = probeRoute(cl, address, timeout); err == nil {
			return address, nil
		}
		failures = append(failures, fmt.Errorf("%s：%w", address, err))
	}
	return "", fmt.Errorf("两条连接都未成功，请确认接收端在线且两端 Tailscale 已连接：%w", errors.Join(failures...))
}
func probeRoute(cl *http.Client, address string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", "https://"+address+api+"info", nil)
	if err != nil {
		return err
	}
	res, err := cl.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var info struct{ Version string }
	if res.StatusCode != 200 || json.NewDecoder(io.LimitReader(res.Body, 65536)).Decode(&info) != nil || info.Version != "1" {
		return errors.New("接收器未就绪")
	}
	return nil
}
