package main

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
)

func notifyReceived(path string) {
	root, e := os.UserConfigDir()
	if e != nil {
		return
	}
	url := "lansend-receipt://file/" + base64.RawURLEncoding.EncodeToString([]byte(filepath.Base(path)))
	cmd := exec.Command("/usr/bin/open", "-g", "-a", filepath.Join(root, "LANSend", "LANSend.app"), url)
	if cmd.Start() == nil {
		go cmd.Wait()
	}
}
