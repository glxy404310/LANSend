//go:build darwin

package main

import (
	"context"
	"net"
	"os/exec"
	"strings"
	"time"
)

func collectLocalIPv4Candidates() []localIPCandidate {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	defaultOrder := darwinDefaultRouteOrder()
	var out []localIPCandidate
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagPointToPoint != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		mediaConnected := iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagRunning != 0
		hardware := isDarwinPhysicalInterface(iface.Name) &&
			iface.Flags&net.FlagLoopback == 0 &&
			iface.Flags&net.FlagPointToPoint == 0 &&
			iface.Flags&net.FlagBroadcast != 0 &&
			len(iface.HardwareAddr) > 0

		order, hasGW := defaultOrder[iface.Name]
		metric := uint32(iface.Index) + 1000
		if hasGW {
			metric = order
		}

		for _, ad := range addrs {
			ip := extractIPv4(ad)
			if !ip.IsValid() {
				continue
			}
			out = append(out, localIPCandidate{
				IP:             ip,
				Name:           iface.Name,
				Hardware:       hardware,
				MediaConnected: mediaConnected,
				HasGateway:     hasGW,
				PhysicalMedium: hardware,
				Preferred:      true,
				Metric:         metric,
				InterfaceIndex: uint32(iface.Index),
			})
		}
	}
	return out
}

func darwinDefaultRouteOrder() map[string]uint32 {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/sbin/netstat", "-rn", "-f", "inet").Output()
	if err != nil {
		return map[string]uint32{}
	}
	return parseDarwinDefaultRouteOrder(string(out))
}

func parseDarwinDefaultRouteOrder(output string) map[string]uint32 {
	order := make(map[string]uint32)
	var rank uint32
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[0] != "default" || strings.HasPrefix(fields[1], "link#") {
			continue
		}
		netif := fields[3]
		if _, exists := order[netif]; !exists {
			rank++
			order[netif] = rank
		}
	}
	return order
}

func isDarwinPhysicalInterface(name string) bool {
	if len(name) < 3 || !strings.HasPrefix(name, "en") {
		return false
	}
	for i := 2; i < len(name); i++ {
		if name[i] < '0' || name[i] > '9' {
			return false
		}
	}
	return true
}
