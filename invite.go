package main

import (
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const inviteLifetime = 5 * time.Minute

type lanInvite struct {
	sync.Mutex
	token, claimed string
	expires        time.Time
}

func parseInvite(code string) (address, fingerprint, token string, err error) {
	invalid := errors.New("LANSend 码格式无效，请完整复制对方生成的配对码")
	code = strings.TrimSpace(code)
	raw, ok := strings.CutPrefix(code, "lansend:")
	endpoint, fragment, hashOK := strings.Cut(raw, "#")
	fingerprint, token, tokenOK := strings.Cut(fragment, ".")
	addr, e := netip.ParseAddrPort(endpoint)
	pin, pinErr := hex.DecodeString(fingerprint)
	secret, secretErr := base64.RawURLEncoding.DecodeString(token)
	if !ok || !hashOK || !tokenOK || e != nil || !addr.Addr().Is4() || !addr.Addr().IsGlobalUnicast() || addr.Port() == 0 || pinErr != nil || len(pin) != 32 || secretErr != nil || len(secret) != 24 {
		return "", "", "", invalid
	}
	return addr.String(), strings.ToLower(fingerprint), token, nil
}

func invitePost(root, address, pin, action, body string) (string, error) {
	c, e := loadIdentity(root)
	if e != nil {
		return "", e
	}
	c.PeerHash = pin
	cl, _, e := client(c, false)
	if e != nil {
		return "", e
	}
	cl.Timeout = 8 * time.Second
	defer cl.CloseIdleConnections()
	req, e := http.NewRequest(http.MethodPost, "https://"+address+api+action, strings.NewReader(body))
	if e != nil {
		return "", e
	}
	req.Header.Set("X-LANSend-Name", url.QueryEscape(deviceName(root)))
	resp, e := cl.Do(req)
	if e != nil {
		if errors.Is(e, errPeerCertificateChanged) {
			return "", errors.New("设备身份与配对码不符，请重新复制")
		}
		return "", errors.New("无法连接接收服务，请确认两台电脑已启动 LANSend 并连接同一局域网")
	}
	defer resp.Body.Close()
	data, e := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if e != nil {
		return "", e
	}
	if resp.StatusCode == 404 {
		return "", errors.New("接收服务不支持 LANSend 码，请更新双方的 LANSend")
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("配对未完成：%s", strings.TrimSpace(string(data)))
	}
	return string(data), nil
}

func identityHash(root string) (string, error) {
	c, e := loadIdentity(root)
	if e != nil {
		return "", e
	}
	cert, e := tls.X509KeyPair([]byte(c.Certificate), []byte(c.PrivateKey))
	if e != nil {
		return "", e
	}
	return digest(cert.Certificate[0]), nil
}

func makeInvite(root, expected string) (string, error) {
	self, e := identityHash(root)
	if e != nil {
		return "", e
	}
	return invitePost(root, "127.0.0.1:"+port, self, "invite/new", expected)
}

func joinInvite(root, code, expected string) error {
	addr, pin, token, e := parseInvite(code)
	if e != nil {
		return e
	}
	self, e := identityHash(root)
	if e != nil {
		return e
	}
	if self == pin {
		return errors.New("这是本机的配对码，请复制另一台电脑上的配对码")
	}
	path := filepath.Join(root, "peer.json")
	name, e := invitePost(root, addr, pin, "invite/join", token)
	if e != nil {
		return e
	}
	return savePair(path, config{Address: addr, PeerHash: pin, PeerName: sanitizeDisplayName(name)})
}

func (inv *lanInvite) register(mux *http.ServeMux, root, self string) {
	path := filepath.Join(root, "peer.json")
	mux.HandleFunc("POST "+api+"invite/new", func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		if !net.ParseIP(host).IsLoopback() || r.TLS == nil || len(r.TLS.PeerCertificates) == 0 || digest(r.TLS.PeerCertificates[0].Raw) != self {
			http.Error(w, "仅本机可生成配对码", 403)
			return
		}
		_, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 64))
		if e != nil {
			http.Error(w, "请求无效", 400)
			return
		}
		ip := primaryLocalIPv4()
		if ip == "" {
			http.Error(w, "未检测到局域网地址", 409)
			return
		}
		inv.Lock()
		defer inv.Unlock()
		secret := make([]byte, 24)
		if _, e = rand.Read(secret); e != nil {
			http.Error(w, "无法生成配对码", 500)
			return
		}
		inv.token = base64.RawURLEncoding.EncodeToString(secret)

		inv.claimed = ""
		inv.expires = time.Now().Add(inviteLifetime)
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprintf(w, "lansend:%s#%s.%s", net.JoinHostPort(ip, port), self, inv.token)
	})
	mux.HandleFunc("POST "+api+"invite/join", func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			http.Error(w, "缺少设备证书", 403)
			return
		}
		peer := digest(r.TLS.PeerCertificates[0].Raw)
		host, _, e := net.SplitHostPort(r.RemoteAddr)
		if e != nil || peer == self {
			http.Error(w, "设备地址无效", 400)
			return
		}
		data, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 64))
		if e != nil {
			http.Error(w, "请求无效", 400)
			return
		}
		inv.Lock()
		defer inv.Unlock()
		if !time.Now().Before(inv.expires) || subtle.ConstantTimeCompare(data, []byte(inv.token)) != 1 || (inv.claimed != "" && inv.claimed != peer) {
			http.Error(w, "配对码已失效，请重新复制", 403)
			return
		}
		// A retry from the same certificate may acknowledge a lost response, but cannot restore a replaced pairing.
		name, _ := url.QueryUnescape(r.Header.Get("X-LANSend-Name"))
		if e = storePair(path, config{Address: net.JoinHostPort(host, port), PeerHash: peer, PeerName: sanitizeDisplayName(name)}, inv.claimed != ""); e != nil {
			http.Error(w, "无法保存配对，请检查现有配对后重试", 409)
			return
		}
		inv.claimed = peer
		fmt.Fprint(w, deviceName(root))
	})
}
