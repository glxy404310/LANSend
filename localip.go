package main

import (
	"net"
	"net/netip"
	"sort"
	"strings"
)

var fakeIPv4Prefix = netip.MustParsePrefix("198.18.0.0/15")

type localIPCandidate struct {
	IP             netip.Addr
	Name           string
	Description    string
	Hardware       bool
	MediaConnected bool
	HasGateway     bool
	PhysicalMedium bool
	Wired          bool
	Preferred      bool
	Metric         uint32
	LinkSpeed      uint64
	InterfaceIndex uint32
}

var virtualAdapterSubstrings = []string{
	"vethernet",
	"hyper-v",
	"wsl",
	"vmware",
	"vmnet",
	"virtualbox",
	"vbox",
	"virtual",
	"wintun",
	"wireguard",
	"tailscale",
	"zerotier",
	"tap-windows",
	"tap-win",
	"openvpn",
	"clash",
	"mihomo",
	"sing-box",
	"singbox",
	"nekobox",
	"v2ray",
	"shadowsocks",
	"radmin",
	"hamachi",
	"npcap",
	"loopback",
	"teredo",
	"isatap",
	"tunnel",
	"bluetooth",
	"蓝牙",
	"vpn",
	"docker",
	"podman",
	"km-test",
	"pseudo",
}

var virtualAdapterExactNames = []string{
	"meta",
	"tun",
	"tap",
	"wg",
	"lo",
}

var virtualAdapterPrefixes = []string{
	"utun",
	"awdl",
	"llw",
	"bridge",
	"vmenet",
	"anpi",
	"feth",
	"gif",
	"stf",
	"ipsec",
	"ppp",
	"br-",
	"veth",
	"virbr",
	"vboxnet",
}

var virtualAdapterDigitPrefixes = []string{
	"tun",
	"tap",
	"wg",
	"ap",
	"lo",
}

func primaryLocalIPv4() string {
	return pickPrimaryIPv4(collectLocalIPv4Candidates())
}

func pickPrimaryIPv4(candidates []localIPCandidate) string {
	var tier1, tier2, tier3 []localIPCandidate
	for _, c := range candidates {
		if !c.IP.IsValid() || !c.IP.Is4() {
			continue
		}
		if c.IP.IsLoopback() || c.IP.IsLinkLocalUnicast() || c.IP.IsMulticast() || c.IP.IsUnspecified() {
			continue
		}
		if fakeIPv4Prefix.Contains(c.IP) || !c.MediaConnected {
			continue
		}
		virtual := looksVirtualAdapter(c.Name, c.Description)
		physical := c.Hardware && !virtual
		if !physical && tailnetIPv4.Contains(c.IP) {
			continue
		}
		switch {
		case physical:
			tier1 = append(tier1, c)
		case !virtual:
			tier2 = append(tier2, c)
		default:
			tier3 = append(tier3, c)
		}
	}

	var pool []localIPCandidate
	switch {
	case len(tier1) > 0:
		pool = tier1
	case len(tier2) > 0:
		pool = tier2
	case len(tier3) > 0:
		pool = tier3
	default:
		return ""
	}

	sort.Slice(pool, func(i, j int) bool {
		return candidateBetter(pool[i], pool[j])
	})
	return pool[0].IP.String()
}

func candidateBetter(a, b localIPCandidate) bool {
	if a.HasGateway != b.HasGateway {
		return a.HasGateway
	}
	if a.PhysicalMedium != b.PhysicalMedium {
		return a.PhysicalMedium
	}
	if a.Preferred != b.Preferred {
		return a.Preferred
	}
	aNonTailnet := !tailnetIPv4.Contains(a.IP)
	bNonTailnet := !tailnetIPv4.Contains(b.IP)
	if aNonTailnet != bNonTailnet {
		return aNonTailnet
	}
	am, bm := effectiveMetric(a.Metric), effectiveMetric(b.Metric)
	if am != bm {
		return am < bm
	}
	if a.Wired != b.Wired {
		return a.Wired
	}
	if a.LinkSpeed != b.LinkSpeed {
		return a.LinkSpeed > b.LinkSpeed
	}
	if a.InterfaceIndex != b.InterfaceIndex {
		return a.InterfaceIndex < b.InterfaceIndex
	}
	return a.IP.Compare(b.IP) < 0
}

func effectiveMetric(metric uint32) uint32 {
	if metric == 0 {
		return ^uint32(0)
	}
	return metric
}

func looksVirtualAdapter(name, desc string) bool {
	lowName := strings.TrimSpace(strings.ToLower(name))
	lowDesc := strings.TrimSpace(strings.ToLower(desc))
	for _, sub := range virtualAdapterSubstrings {
		if strings.Contains(lowName, sub) || strings.Contains(lowDesc, sub) {
			return true
		}
	}
	for _, exact := range virtualAdapterExactNames {
		if lowName == exact {
			return true
		}
	}
	for _, prefix := range virtualAdapterPrefixes {
		if strings.HasPrefix(lowName, prefix) {
			return true
		}
	}
	for _, prefix := range virtualAdapterDigitPrefixes {
		if hasPrefixDigit(lowName, prefix) {
			return true
		}
	}
	return false
}

func hasPrefixDigit(s, prefix string) bool {
	if len(s) <= len(prefix) || !strings.HasPrefix(s, prefix) {
		return false
	}
	next := s[len(prefix)]
	return next >= '0' && next <= '9'
}

func collectGenericLocalIPv4s() []localIPCandidate {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
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
		hardware := iface.Flags&net.FlagBroadcast != 0 && len(iface.HardwareAddr) > 0 && !looksVirtualAdapter(iface.Name, "")
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
				Preferred:      true,
				InterfaceIndex: uint32(iface.Index),
			})
		}
	}
	return out
}

func extractIPv4(ad net.Addr) netip.Addr {
	var ip net.IP
	switch v := ad.(type) {
	case *net.IPNet:
		ip = v.IP
	case *net.IPAddr:
		ip = v.IP
	default:
		parsed, _, err := net.ParseCIDR(ad.String())
		if err == nil {
			ip = parsed
		}
	}
	v4 := ip.To4()
	if v4 == nil {
		return netip.Addr{}
	}
	addr, ok := netip.AddrFromSlice(v4)
	if !ok {
		return netip.Addr{}
	}
	return addr
}
