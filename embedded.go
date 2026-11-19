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
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"tailscale.com/tsnet"
)

const nodeAPI = "/lansend-local/v1/remote/"

type nodeStatus struct {
	State, IP, LoginURL, ProxyAddr, ProxyPassword string
	Devices                                       []accountDevice `json:",omitempty"`
}
type embeddedNode struct {
	mu                       sync.Mutex
	root, self               string
	tlsConfig                *tls.Config
	handler                  http.Handler
	server                   *tsnet.Server
	proxyAddr, proxyPassword string
}

func (n *embeddedNode) start() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.server != nil {
		return nil
	}
	s := &tsnet.Server{Dir: filepath.Join(n.root, "tsnet"), Hostname: "lansend-" + runtime.GOOS + "-" + n.self[:6], UserLogf: func(string, ...any) {}}
	ln, err := s.Listen("tcp", ":"+port)
	if err != nil {
		s.Close()
		return err
	}
	addr, password, _, err := s.Loopback()
	if err != nil {
		ln.Close()
		s.Close()
		return err
	}
	n.server = s
	n.proxyAddr = addr
	n.proxyPassword = password
	overlay := http.NewServeMux()
	overlay.Handle("/", n.handler)
	overlay.HandleFunc("POST "+api+"account-pair", n.acceptAccountPair)
	srv := &http.Server{Handler: overlay, TLSConfig: n.tlsConfig.Clone(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	go srv.ServeTLS(ln, "", "")
	return nil
}
func (n *embeddedNode) stop() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.server != nil {
		n.server.Close()
		n.server = nil
		n.proxyAddr = ""
		n.proxyPassword = ""
	}
}
func (n *embeddedNode) status(ctx context.Context, proxy bool) (nodeStatus, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.server == nil {
		return nodeStatus{State: "Off"}, nil
	}
	if proxy {
		return nodeStatus{ProxyAddr: n.proxyAddr, ProxyPassword: n.proxyPassword}, nil
	}
	lc, err := n.server.LocalClient()
	if err != nil {
		return nodeStatus{}, err
	}
	st, err := lc.StatusWithoutPeers(ctx)
	if err != nil {
		return nodeStatus{}, err
	}
	result := nodeStatus{State: st.BackendState, LoginURL: st.AuthURL}
	for _, ip := range st.TailscaleIPs {
		if ip.Is4() {
			result.IP = ip.String()
			break
		}
	}
	return result, nil
}
func (n *embeddedNode) control(w http.ResponseWriter, r *http.Request) {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if !net.ParseIP(host).IsLoopback() || r.TLS == nil || len(r.TLS.PeerCertificates) == 0 || digest(r.TLS.PeerCertificates[0].Raw) != n.self {
		http.Error(w, "forbidden", 403)
		return
	}
	action := strings.TrimPrefix(r.URL.Path, nodeAPI)
	marker := filepath.Join(n.root, "remote-enabled")
	switch action {
	case "enable":
		if err := os.WriteFile(marker, []byte("1\n"), 0600); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if err := n.start(); err != nil {
			http.Error(w, err.Error(), 503)
			return
		}
	case "disable":
		if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
			http.Error(w, err.Error(), 500)
			return
		}
		n.stop()
	case "devices", "connect":
		id := r.URL.Query().Get("id")
		if action == "connect" && id == "" {
			http.Error(w, "请选择设备", 400)
			return
		}
		n.deviceControl(w, r, id)
		return
	case "status", "proxy":
	default:
		http.NotFound(w, r)
		return
	}
	status, err := n.status(r.Context(), action == "proxy")
	if err != nil {
		http.Error(w, err.Error(), 503)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(status)
}
func nodeRequest(root, action string) (nodeStatus, error) {
	var st nodeStatus
	c, err := loadIdentity(root)
	if err != nil {
		return st, err
	}
	cert, err := tls.X509KeyPair([]byte(c.Certificate), []byte(c.PrivateKey))
	if err != nil {
		return st, err
	}
	c.PeerHash = digest(cert.Certificate[0])
	cl, _, err := client(c, false)
	if err != nil {
		return st, err
	}
	defer cl.CloseIdleConnections()
	cl.Timeout = 12 * time.Second
	res, err := cl.Post("https://127.0.0.1:"+port+nodeAPI+action, "application/json", nil)
	if err != nil {
		return st, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		return st, fmt.Errorf("%s (%s)", strings.TrimSpace(string(msg)), res.Status)
	}
	err = json.NewDecoder(res.Body).Decode(&st)
	return st, err
}
func embeddedProxy(root string, r *http.Request) (*url.URL, error) {
	if root == "" || !isRemoteAddress(r.URL.Host) {
		return nil, nil
	}
	if _, err := os.Stat(filepath.Join(root, "remote-enabled")); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	status, err := nodeRequest(root, "proxy")
	if err != nil {
		return nil, err
	}
	host, _, err := net.SplitHostPort(status.ProxyAddr)
	if err != nil || !net.ParseIP(host).IsLoopback() || status.ProxyPassword == "" {
		return nil, errors.New("内置远程连接尚未就绪")
	}
	return &url.URL{Scheme: "socks5", Host: status.ProxyAddr, User: url.UserPassword("tsnet", status.ProxyPassword)}, nil
}
func remoteLogin(root string) (string, error) {
	status, err := nodeRequest(root, "enable")
	for i := 0; err == nil && i < 18; i++ {
		if status.State == "Running" && status.IP != "" {
			return "已登录。打开“我的电脑”，选择对方并点配对。本机远程地址：" + status.IP, nil
		}
		if status.LoginURL != "" {
			u, e := url.Parse(status.LoginURL)
			if e != nil || u.Scheme != "https" || u.Host != "login.tailscale.com" || u.User != nil {
				return "", errors.New("未获得有效的 Tailscale 登录链接")
			}
			var cmd *exec.Cmd
			if runtime.GOOS == "windows" {
				cmd = exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "rundll32.exe"), "url.dll,FileProtocolHandler", u.String())
			} else {
				cmd = exec.Command("/usr/bin/open", u.String())
			}
			if e := cmd.Start(); e != nil {
				return "", e
			}
			go cmd.Wait()
			return "请在浏览器完成登录，在“我的电脑”中选择设备配对。", nil
		}
		time.Sleep(500 * time.Millisecond)
		status, err = nodeRequest(root, "status")
	}
	if err != nil {
		return "", err
	}
	return "远程网络状态：" + status.State + "。稍后可再点一次查看。", nil
}
