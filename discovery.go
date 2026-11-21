package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/sync/errgroup"
)

const discoveryPrefix = "LANSend/1 discover "

func deviceName(root string) string {
	b, e := os.ReadFile(filepath.Join(root, "device-name"))
	if e == nil && len(b) > 0 {
		return string(b)
	}
	name, _ := os.Hostname()
	if name == "" {
		return "LANSend"
	}
	return name
}
func setDeviceName(root, name string) error { return writeName(root, "device-name", name) }

// Local nicknames follow certificate identities across address and pairing changes.
func nicknameFile(root, hash string) (string, error) {
	key, err := hex.DecodeString(hash)
	if err != nil || len(key) != 32 {
		return "", errors.New("请先配对设备")
	}
	return filepath.Join(root, "nickname-"+hex.EncodeToString(key)), nil
}
func peerNickname(root, hash string) string {
	path, err := nicknameFile(root, hash)
	if err != nil {
		return ""
	}
	b, _ := os.ReadFile(path)
	return string(b)
}
func setPeerNickname(root, hash, name string) error {
	path, err := nicknameFile(root, hash)
	if err != nil {
		return err
	}
	paired, err := findPeer(filepath.Join(root, "peer.json"), hash)
	if err != nil {
		return err
	}
	if paired.PeerHash != hash {
		return errors.New("配对设备已更换，请重新打开设置")
	}
	if strings.TrimSpace(name) == "" {
		err = os.Remove(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return writeName(root, filepath.Base(path), name)
}
func writeName(root, file, name string) error {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 48 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return errors.New("请输入 1 到 48 个可显示字符")
	}
	f, e := os.CreateTemp(root, "device-name-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	_, e = f.WriteString(name)
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), filepath.Join(root, file))
}
func listenDiscovery(self string) func() {
	socket, err := net.ListenUDP("udp4", &net.UDPAddr{Port: 53318})
	if err != nil {
		return func() {}
	}
	go answerDiscovery(socket, self)
	return func() { socket.Close() }
}
func answerDiscovery(socket *net.UDPConn, self string) {
	buf := make([]byte, 256)
	prefix := discoveryPrefix + self + " "
	for {
		n, src, e := socket.ReadFromUDP(buf)
		if e != nil {
			return
		}
		msg := string(buf[:n])
		if len(msg) != len(prefix)+32 || !strings.HasPrefix(msg, prefix) {
			continue
		}
		nonce := msg[len(prefix):]
		if _, e = hex.DecodeString(nonce); e != nil {
			continue
		}
		socket.WriteToUDP([]byte("LANSend/1 here "+nonce), src)
	}
}
func broadcastTargets() []net.IP {
	out := []net.IP{net.IPv4bcast}
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		network, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip := network.IP.To4()
		ones, bits := network.Mask.Size()
		if ip == nil || ip.IsLoopback() || bits != 32 || ones >= 31 || ones == 0 {
			continue
		}
		b := append(net.IP(nil), ip...)
		for i := range b {
			b[i] |= ^network.Mask[i]
		}
		out = append(out, b)
	}
	return out
}

// Replies provide only a candidate address. resolvePeer verifies the saved TLS pin before use.
func discoverPeer(hash string) (string, error) {
	return discoverAt(hash, broadcastTargets(), 53318)
}
func discoverAt(hash string, targets []net.IP, servicePort int) (string, error) {
	socket, e := net.ListenUDP("udp4", &net.UDPAddr{})
	if e != nil {
		return "", e
	}
	defer socket.Close()
	var random [16]byte
	if _, e = rand.Read(random[:]); e != nil {
		return "", e
	}
	nonce := hex.EncodeToString(random[:])
	socket.SetDeadline(time.Now().Add(1200 * time.Millisecond))
	for _, b := range targets {
		socket.WriteToUDP([]byte(discoveryPrefix+hash+" "+nonce), &net.UDPAddr{IP: b, Port: servicePort})
	}
	buf := make([]byte, 128)
	for {
		n, src, e := socket.ReadFromUDP(buf)
		if e != nil {
			return "", e
		}
		if src.Port == servicePort && string(buf[:n]) == "LANSend/1 here "+nonce {
			return net.JoinHostPort(src.IP.String(), port), nil
		}
	}
}
func peerStatus(root string) string {
	stored, e := readPeer(filepath.Join(root, "peer.json"))
	if e != nil || stored.PeerHash == "" {
		return "尚未配对"
	}
	nickname := peerNickname(root, stored.PeerHash)
	name := stored.PeerName
	if name == "" {
		name = "已配对设备"
	}
	if nickname != "" {
		name = nickname
	}
	if !stored.Enabled {
		return name + " · 未启用"
	}
	id, e := loadIdentity(root)
	if e == nil {
		_, alias, e := resolveStoredPeer(id, stored)
		if e == nil {
			if alias != "" && nickname == "" {
				name = alias
			}
			return fmt.Sprintf("%s · 在线", name)
		}
	}
	return fmt.Sprintf("%s · 离线", name)
}

func resolveStoredPeer(id, stored config) (string, string, error) {
	id.Address, id.RemoteAddress, id.PeerHash, id.PeerName = stored.Address, stored.RemoteAddress, stored.PeerHash, stored.PeerName
	return resolvePeer(id)
}

func connectedPeerCount(root string) (int, error) {
	peers, err := peersAt(filepath.Join(root, "peer.json"))
	if err != nil || len(peers) == 0 {
		return 0, err
	}
	id, err := loadIdentity(root)
	if err != nil {
		return 0, err
	}
	var online atomic.Int32
	var probes errgroup.Group
	probes.SetLimit(4)
	for _, peer := range peers {
		if !peer.Enabled {
			continue
		}
		probes.Go(func() error {
			if _, _, err := resolveStoredPeer(id, peer); err == nil {
				online.Add(1)
			}
			return nil
		})
	}
	probes.Wait()
	return int(online.Load()), nil
}
