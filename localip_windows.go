//go:build windows

package main

import (
	"errors"
	"net/netip"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	iaosfHardwareInterface = 1 << 0
	iaosfConnectorPresent  = 1 << 2
	iaosfNotMediaConnected = 1 << 4

	ndisPhysicalMediumWirelessLan  = 1
	ndisPhysicalMediumNative802_11 = 9
	ndisPhysicalMediumBluetooth    = 10
	ndisPhysicalMedium802_3        = 14
)

func collectLocalIPv4Candidates() []localIPCandidate {
	const flags = windows.GAA_FLAG_INCLUDE_GATEWAYS |
		windows.GAA_FLAG_SKIP_ANYCAST |
		windows.GAA_FLAG_SKIP_MULTICAST |
		windows.GAA_FLAG_SKIP_DNS_SERVER

	size := uint32(15 * 1024)
	var buf []byte
	var err error
	for i := 0; i < 3; i++ {
		buf = make([]byte, size)
		err = windows.GetAdaptersAddresses(windows.AF_INET, flags, 0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])), &size)
		if err == nil {
			break
		}
		if !errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) || size < uint32(unsafe.Sizeof(windows.IpAdapterAddresses{})) {
			return collectGenericLocalIPv4s()
		}
	}
	if err != nil || len(buf) < int(unsafe.Sizeof(windows.IpAdapterAddresses{})) {
		return collectGenericLocalIPv4s()
	}

	var out []localIPCandidate
	for aa := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])); aa != nil; aa = aa.Next {
		if aa.OperStatus != windows.IfOperStatusUp ||
			aa.IfType == windows.IF_TYPE_SOFTWARE_LOOPBACK ||
			aa.IfType == windows.IF_TYPE_TUNNEL {
			continue
		}

		name := windows.UTF16PtrToString(aa.FriendlyName)
		desc := windows.UTF16PtrToString(aa.Description)
		isEthOrWiFi := aa.IfType == windows.IF_TYPE_ETHERNET_CSMACD || aa.IfType == windows.IF_TYPE_IEEE80211

		mediaConnected := true
		hardware := false
		physicalMedium := false

		var row windows.MibIfRow2
		row.InterfaceLuid = aa.Luid
		if rowErr := windows.GetIfEntry2Ex(windows.MibIfEntryNormalWithoutStatistics, &row); rowErr == nil {
			mediaConnected = (row.InterfaceAndOperStatusFlags & iaosfNotMediaConnected) == 0
			hardware = isEthOrWiFi &&
				(row.InterfaceAndOperStatusFlags&iaosfHardwareInterface) != 0 &&
				(row.InterfaceAndOperStatusFlags&iaosfConnectorPresent) != 0 &&
				row.PhysicalMediumType != ndisPhysicalMediumBluetooth
			physicalMedium = row.PhysicalMediumType == ndisPhysicalMedium802_3 ||
				row.PhysicalMediumType == ndisPhysicalMediumNative802_11 ||
				row.PhysicalMediumType == ndisPhysicalMediumWirelessLan
		}

		hasGateway := false
		for gw := aa.FirstGatewayAddress; gw != nil; gw = gw.Next {
			ip := gw.Address.IP()
			if ip == nil {
				continue
			}
			v4 := ip.To4()
			if v4 != nil && !v4.IsUnspecified() {
				hasGateway = true
				break
			}
		}

		speed := aa.TransmitLinkSpeed
		if speed == ^uint64(0) {
			speed = 0
		}

		for ua := aa.FirstUnicastAddress; ua != nil; ua = ua.Next {
			if ua.DadState == windows.IpDadStateInvalid || ua.DadState == windows.IpDadStateDuplicate {
				continue
			}
			ip := ua.Address.IP()
			if ip == nil {
				continue
			}
			v4 := ip.To4()
			if v4 == nil {
				continue
			}
			addr, ok := netip.AddrFromSlice(v4)
			if !ok {
				continue
			}
			out = append(out, localIPCandidate{
				IP:             addr,
				Name:           name,
				Description:    desc,
				Hardware:       hardware,
				MediaConnected: mediaConnected,
				HasGateway:     hasGateway,
				PhysicalMedium: physicalMedium,
				Wired:          aa.IfType == windows.IF_TYPE_ETHERNET_CSMACD,
				Preferred:      ua.DadState == windows.IpDadStatePreferred,
				Metric:         aa.Ipv4Metric,
				LinkSpeed:      speed,
				InterfaceIndex: aa.IfIndex,
			})
		}
	}
	return out
}
