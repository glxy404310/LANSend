package main

import (
	"bufio"

	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"

	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"strconv"
	"strings"
	"time"
)

const api = "/lansend/v1/"
const port = "53318"

type config struct {
	root                                       string
	Address, PeerHash, Certificate, PrivateKey string
	RemoteAddress                              string `json:",omitempty"`
}

func digest(raw []byte) string { h := sha256.Sum256(raw); return hex.EncodeToString(h[:]) }
func makeIdentity(c *config) error {
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		return e
	}
	now := time.Now()
	tmpl := x509.Certificate{SerialNumber: new(big.Int).SetBytes(key.PublicKey.X.Bytes()), Subject: pkix.Name{CommonName: "LANSend"}, NotBefore: now.Add(-time.Hour), NotAfter: now.AddDate(10, 0, 0), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}}
	cert, e := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if e != nil {
		return e
	}
	priv, e := x509.MarshalECPrivateKey(key)
	if e != nil {
		return e
	}
	c.Certificate = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert}))
	c.PrivateKey = string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: priv}))
	return nil
}
func client(c config, pairing bool) (*http.Client, string, error) {
	identity, e := tls.X509KeyPair([]byte(c.Certificate), []byte(c.PrivateKey))
	if e != nil {
		return nil, "", e
	}
	t := &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{identity}, InsecureSkipVerify: true, // Each device has a self-signed certificate. Pin the complete leaf.
		VerifyConnection: func(s tls.ConnectionState) error {
			if len(s.PeerCertificates) == 0 {
				return errors.New("接收端未提供证书")
			}
			if !pairing && digest(s.PeerCertificates[0].Raw) != c.PeerHash {
				return errPeerCertificateChanged
			}
			return nil
		}}
	return &http.Client{Transport: &http.Transport{Proxy: func(r *http.Request) (*url.URL, error) { return embeddedProxy(c.root, r) }, TLSClientConfig: t, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, TLSHandshakeTimeout: 6 * time.Second, ResponseHeaderTimeout: 3 * time.Minute}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, digest(identity.Certificate[0]), nil
}
func pair(path, addr string, code func() string) error {
	if !strings.Contains(addr, ":") {
		addr = net.JoinHostPort(addr, port)
	}
	host, port, e := net.SplitHostPort(addr)
	n, pe := strconv.Atoi(port)
	if e != nil || pe != nil || host == "" || n < 1 || n > 65535 || strings.ContainsAny(host, "/@?#\\ ") {
		return errors.New("请输入 IP 或主机名，可附加 :端口")
	}
	c, e := loadIdentity(filepath.Dir(path))
	if e != nil {
		return e
	}
	c.Address = addr
	cl, _, e := client(c, true)
	if e != nil {
		return e
	}
	cl.Timeout = 10 * time.Second
	resp, e := cl.Get("https://" + addr + api + "info")
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	var info struct{ Alias, Version string }
	if resp.StatusCode != 200 || json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&info) != nil || info.Alias == "" || info.Version != "1" {
		return errors.New("未找到 LANSend 独立接收端")
	}
	c.PeerHash = digest(resp.TLS.PeerCertificates[0].Raw)
	fmt.Printf("接收端 %q (%s)\n请输入对方安装窗口显示的本机校验码：", info.Alias, addr)
	answer := code()
	if !strings.EqualFold(strings.TrimSpace(answer), c.PeerHash[:12]) {
		return errors.New("校验码不匹配，未更改配对")
	}
	return savePair(path, c, true)
}
func savePair(path string, c config, replace bool) error {
	return savePairExpected(path, c, replace, nil)
}
func savePairExpected(path string, c config, replace bool, expected *string) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	lock, e := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer lock.Close()
	if e = lockPeerFile(lock); e != nil {
		return e
	}
	old, e := readPeer(path)
	if e != nil {
		return e
	}
	if expected != nil && old.PeerHash != *expected && old.PeerHash != c.PeerHash {
		return errPeerOccupied
	}
	if expected == nil && !replace && old.PeerHash != "" && old.PeerHash != c.PeerHash {
		return errPeerOccupied
	}
	keepPeerRoutes(&c, old)
	c.Certificate = ""
	c.PrivateKey = ""
	f, e := os.CreateTemp(filepath.Dir(path), "pair-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	e = json.NewEncoder(f).Encode(c)
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return os.Rename(f.Name(), path)
}

type progress struct {
	name       string
	size, sent int64
	last       time.Time
	report     func(int)
}

func (p *progress) Write(b []byte) (int, error) {
	p.sent += int64(len(b))
	if time.Since(p.last) > 250*time.Millisecond && p.size > 0 {
		fmt.Printf("\r%s %.0f%%", p.name, 100*float64(p.sent)/float64(p.size))
		if p.report != nil {
			p.report(int(100 * float64(p.sent) / float64(p.size)))
		}
		p.last = time.Now()
	}
	return len(b), nil
}

type transferProgress func(int, int, string, int)

func send(c config, paths []string) error { return sendProgress(c, paths, nil) }
func sendProgress(c config, paths []string, update transferProgress) (err error) {
	cl, _, e := client(c, false)
	if e != nil {
		return e
	}
	defer cl.CloseIdleConnections()
	for _, p := range paths {
		st, e := os.Stat(p)
		if e != nil {
			return sendFailure(e, p, 0, len(paths), false)
		}
		if !st.Mode().IsRegular() {
			return fmt.Errorf("请先压缩文件夹：%s", p)
		}
	}
	for i, p := range paths {
		fmt.Printf("发送 %d/%d：%s\n", i+1, len(paths), filepath.Base(p))
		report := func(percent int) {
			if update != nil {
				update(i+1, len(paths), filepath.Base(p), percent)
			}
		}
		report(0)
		if e := upload(cl, c.Address, p, report); e != nil {
			return sendFailure(e, p, i, len(paths), false)
		}
	}
	fmt.Printf("已发送 %d 个文件。\n", len(paths))
	return nil
}
func upload(cl *http.Client, address, path string, report func(int)) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return e
	}
	name := filepath.Base(path)
	var body io.Reader = http.NoBody
	if st.Size() > 0 {
		body = io.TeeReader(f, &progress{name: name, size: st.Size(), report: report})
	}
	req, e := http.NewRequest("PUT", "https://"+address+api+"file?"+url.Values{"name": {name}}.Encode(), body)
	if e != nil {
		return e
	}
	req.ContentLength = st.Size()
	resp, e := cl.Do(req)
	fmt.Println()
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}
func run(args []string) error {
	if len(args) == 2 && args[0] == "reveal" {
		return revealReceived(args[1])
	}
	root, e := os.UserConfigDir()
	if e != nil {
		return e
	}
	root = filepath.Join(root, "LANSend")
	path := filepath.Join(root, "peer.json")
	if len(args) == 1 {
		switch args[0] {
		case "devices":
			st, e := nodeRequest(root, "devices")
			if e != nil {
				return e
			}
			return json.NewEncoder(os.Stdout).Encode(st.Devices)
		case "remote-login":
			msg, err := remoteLogin(root)
			if err == nil {
				fmt.Println(msg)
			}
			return err
		case "remote-off":
			_, err := nodeRequest(root, "disable")
			return err
		case "init":
			return initialize(root)
		case "receive":
			return receive(root)
		case "ready":
			return ready(root)
		}
	}
	if len(args) == 2 && args[0] == "invite-new" {
		code, err := makeInvite(root, args[1])
		if err == nil {
			fmt.Println(code)
		}
		return err
	}
	if len(args) == 2 && args[0] == "invite-join" {
		data, err := io.ReadAll(io.LimitReader(os.Stdin, 1024))
		if err != nil {
			return err
		}
		return joinInvite(root, string(data), args[1])
	}
	if len(args) == 2 && args[0] == "connect-device" {
		_, e := nodeRequest(root, "connect?id="+url.QueryEscape(args[1]))
		return e
	}
	if len(args) == 2 && args[0] == "pair" {
		return pair(path, args[1], func() string { s, _ := bufio.NewReader(os.Stdin).ReadString('\n'); return s })
	}
	if len(args) < 2 || (args[0] != "send" && args[0] != "menu") {
		return errors.New("用法：lansend pair 对方IP 或 lansend send 文件…")
	}
	return sendSaved(root, args[1:], nil)
}
func sendSaved(root string, paths []string, progress transferProgress) error {
	b, e := os.ReadFile(filepath.Join(root, "peer.json"))
	if e != nil {
		return errors.New("请先在配对与设置中连接对方电脑")
	}
	var c config
	if e = json.Unmarshal(b, &c); e != nil {
		return e
	}
	identity, e := loadIdentity(root)
	if e != nil {
		return e
	}
	identity.PeerHash = c.PeerHash
	identity.Address = c.Address
	identity.RemoteAddress = c.RemoteAddress
	identity.Address, e = chooseRoute(identity)
	if e != nil {
		return sendFailure(e, "", 0, len(paths), true)
	}
	return sendProgress(identity, paths, progress)
}
func main() {
	args := os.Args[1:]
	if desktopEntry(args) {
		return
	}
	err := run(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	if err != nil {
		os.Exit(1)
	}
}
