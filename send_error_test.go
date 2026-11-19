package main

import (
	"context"
	"net"
	"net/url"
	"strings"
	"testing"
)

func TestSendFailureDisplay(t *testing.T) {
	endpoint := "https://192.168.1.7:53318/file?name=%E5%A4%A7"
	dial := &url.Error{Op: "Put", URL: endpoint, Err: &net.OpError{Op: "dial", Net: "tcp", Err: context.DeadlineExceeded}}
	for _, tc := range []struct {
		err  error
		both bool
		want string
	}{{dial, false, "可能已关机"}, {dial, true, "局域网和远程连接"}, {&url.Error{Op: "Put", URL: endpoint, Err: context.DeadlineExceeded}, false, "等待接收端响应超时"}, {&url.Error{Op: "Put", URL: endpoint, Err: errPeerCertificateChanged}, false, "重新配对"}} {
		msg := sendFailure(tc.err, "/private/test/大作业.pdf", 1, 3, tc.both).Error()
		for _, s := range []string{tc.want, "大作业.pdf", "1/3"} {
			if !strings.Contains(msg, s) {
				t.Fatal(msg)
			}
		}
		for _, s := range []string{"https://", "192.168", "/private/", "%E5", "dial tcp"} {
			if strings.Contains(msg, s) {
				t.Fatal(msg)
			}
		}
	}
}
