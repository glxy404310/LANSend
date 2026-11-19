package main

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

var errPeerCertificateChanged = errors.New("接收端证书已变化，请核对设备并重新配对")

func sendFailure(err error, path string, completed, total int, bothRoutes bool) error {
	peer := "对方电脑"
	switch runtime.GOOS {
	case "darwin":
		peer = "Windows 电脑"
	case "windows":
		peer = "Mac"
	}
	var op *net.OpError
	var request *url.Error
	var fs *os.PathError
	var dns *net.DNSError
	var timed net.Error
	var message string
	switch {
	case errors.Is(err, errPeerCertificateChanged):
		message = errPeerCertificateChanged.Error() + "。"
	case bothRoutes:
		message = fmt.Sprintf("通过局域网和远程连接都未能连上%s。请确认它已开机、唤醒并联网，再重试。", peer)
	case errors.As(err, &fs):
		message = "无法读取文件，请确认文件仍在原位置且可以打开。"
	case errors.As(err, &dns):
		message = "找不到对方的网络地址，请检查网络连接和设备配对中的地址。"
	case errors.Is(err, syscall.ECONNREFUSED) || (runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(10061))):
		message = fmt.Sprintf("%s拒绝了连接。请在对方电脑上打开 LANSend，等接收服务就绪后重试。", peer)
	case errors.As(err, &op) && op.Op == "dial":
		message = fmt.Sprintf("无法连接到%s。它可能已关机、正在睡眠或未联网。请将它开机或唤醒，确认联网后重试。", peer)
	case errors.As(err, &timed) && timed.Timeout():
		message = "等待接收端响应超时。请先检查对方收件箱，再重试尚未收到的文件。"
	case errors.As(err, &request):
		message = "与接收端的连接中断。请检查两端网络和对方收件箱，再重试。"
	default:
		message = strings.TrimSpace(err.Error())
	}
	if path != "" {
		message += fmt.Sprintf(" 文件：%s。", filepath.Base(path))
	}
	if total > 1 {
		message += fmt.Sprintf(" 已完成 %d/%d 个文件。", completed, total)
	}
	return errors.New(message)
}
