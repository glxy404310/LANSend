package main

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

func loadIdentity(root string) (config, error) {
	var c config
	b, e := os.ReadFile(filepath.Join(root, "identity.json"))
	if e != nil {
		return c, e
	}
	e = json.Unmarshal(b, &c)
	c.root = root
	return c, e
}
func initialize(root string) error {
	if e := os.MkdirAll(root, 0700); e != nil {
		return e
	}
	c, e := loadIdentity(root)
	if errors.Is(e, os.ErrNotExist) {
		if e = makeIdentity(&c); e != nil {
			return e
		}
		f, e := os.CreateTemp(root, "identity-")
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
		if e = os.Link(f.Name(), filepath.Join(root, "identity.json")); e != nil && !errors.Is(e, os.ErrExist) {
			return e
		}
		c, e = loadIdentity(root)
		if e != nil {
			return e
		}
	} else if e != nil {
		return e
	}
	cert, e := tls.X509KeyPair([]byte(c.Certificate), []byte(c.PrivateKey))
	if e != nil {
		return e
	}
	fmt.Printf("本机校验码：%s\n", digest(cert.Certificate[0])[:12])
	addrs, e := net.InterfaceAddrs()
	if e != nil {
		return e
	}
	for _, a := range addrs {
		ip, _, e := net.ParseCIDR(a.String())
		if e == nil && ip.To4() != nil && !ip.IsLoopback() {
			fmt.Println("本机 IPv4：", ip)
		}
	}
	return nil
}
func ready(root string) error {
	c, e := loadIdentity(root)
	if e != nil {
		return e
	}
	identity, e := tls.X509KeyPair([]byte(c.Certificate), []byte(c.PrivateKey))
	if e != nil {
		return e
	}
	c.PeerHash = digest(identity.Certificate[0])
	cl, _, e := client(c, false)
	if e != nil {
		return e
	}
	cl.Timeout = time.Second
	defer cl.CloseIdleConnections()
	for i := 0; i < 15; i++ {
		r, err := cl.Get("https://127.0.0.1:" + port + api + "info")
		if err == nil {
			r.Body.Close()
			if r.StatusCode == 200 {
				return nil
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return errors.New("本机接收服务未启动，请检查 TCP 53318 端口是否被占用")
}
func receive(root string) error {
	c, e := loadIdentity(root)
	if e != nil {
		return e
	}
	cert, e := tls.X509KeyPair([]byte(c.Certificate), []byte(c.PrivateKey))
	if e != nil {
		return e
	}
	home, e := os.UserHomeDir()
	if e != nil {
		return e
	}
	dest := filepath.Join(home, "Downloads", "LANSend")
	if e = os.MkdirAll(dest, 0700); e != nil {
		return e
	}
	srv := http.Server{Addr: ":" + port, Handler: routes(root, dest, notifyReceived), TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}, ClientAuth: tls.RequestClientCert}, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	node := &embeddedNode{root: root, self: digest(cert.Certificate[0]), tlsConfig: srv.TLSConfig.Clone(), handler: srv.Handler}
	mux := http.NewServeMux()
	mux.Handle("/", srv.Handler)
	mux.HandleFunc("POST "+nodeAPI, node.control)
	srv.Handler = mux
	if _, err := os.Stat(filepath.Join(root, "remote-enabled")); err == nil {
		go node.start()
	}
	defer node.stop()
	return srv.ListenAndServeTLS("", "")
}
func routes(root, dest string, received func(string)) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+api+"info", func(w http.ResponseWriter, r *http.Request) {
		host, _ := os.Hostname()
		json.NewEncoder(w).Encode(map[string]string{"alias": host, "version": "1"})
	})
	mux.HandleFunc("PUT "+api+"file", func(w http.ResponseWriter, r *http.Request) {
		var peer config
		b, e := os.ReadFile(filepath.Join(root, "peer.json"))
		if e != nil || json.Unmarshal(b, &peer) != nil || r.TLS == nil || len(r.TLS.PeerCertificates) == 0 || digest(r.TLS.PeerCertificates[0].Raw) != peer.PeerHash {
			http.Error(w, "接收端尚未与这台电脑配对", 403)
			return
		}
		name := r.URL.Query().Get("name")
		if !safeName(name) || r.ContentLength < 0 {
			http.Error(w, "文件名不兼容或缺少长度", 400)
			return
		}
		path, e := saveFile(dest, name, r.Body, r.ContentLength)
		if e != nil {
			http.Error(w, "保存失败，请检查磁盘空间、文件名和目录权限", 500)
			return
		}
		received(path)
		w.WriteHeader(201)
	})
	return mux
}
func safeName(name string) bool {
	if !filepath.IsLocal(name) || name == "." || name != strings.TrimRight(name, ". ") || strings.ContainsAny(name, "/\\:*?\"<>|") || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return false
	}
	return true
}
func saveFile(dest, name string, body io.Reader, size int64) (string, error) {
	f, e := os.CreateTemp(dest, ".lansend-*.part")
	if e != nil {
		return "", e
	}
	defer os.Remove(f.Name())
	n, e := io.Copy(f, body)
	ce := f.Close()
	if e != nil {
		return "", e
	}
	if ce != nil {
		return "", ce
	}
	if n != size {
		return "", io.ErrUnexpectedEOF
	}
	ext := filepath.Ext(name)
	for i := 0; ; i++ {
		candidate := name
		if i > 0 {
			candidate = fmt.Sprintf("%s (%d)%s", strings.TrimSuffix(name, ext), i, ext)
		}
		path := filepath.Join(dest, candidate)
		e = os.Link(f.Name(), path)
		if e == nil {
			return path, nil
		}
		if !errors.Is(e, os.ErrExist) {
			return "", e
		}
	}
}
