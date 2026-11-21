package main

import (
	"os/exec"
	"syscall"
)

func hideMenuProcess(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }
