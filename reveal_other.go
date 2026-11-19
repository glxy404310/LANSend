//go:build !windows

package main

import "errors"

func revealReceived(string) error { return errors.New("此命令仅供 Windows 收件通知使用") }
