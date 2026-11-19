package main

import (
	"crypto/tls"
	"crypto/x509"
	"net/http/httptest"
	"testing"
)

func TestLocalNodeControl(t *testing.T) {
	var c config
	if err := makeIdentity(&c); err != nil {
		t.Fatal(err)
	}
	key, err := tls.X509KeyPair([]byte(c.Certificate), []byte(c.PrivateKey))
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(key.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	n := embeddedNode{self: digest(leaf.Raw)}
	for _, tc := range []struct {
		source  string
		present bool
		want    int
	}{{"100.64.0.1:50", true, 403}, {"127.0.0.1:50", false, 403}, {"127.0.0.1:50", true, 200}} {
		r := httptest.NewRequest("POST", "https://localhost"+nodeAPI+"status", nil)
		r.RemoteAddr = tc.source
		if tc.present {
			r.TLS.PeerCertificates = []*x509.Certificate{leaf}
		}
		w := httptest.NewRecorder()
		n.control(w, r)
		if w.Code != tc.want {
			t.Fatalf("%s: %d", tc.source, w.Code)
		}
	}
}
