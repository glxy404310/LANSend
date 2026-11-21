//go:build !windows

package main

import "os/exec"

func hideMenuProcess(c *exec.Cmd) {}
