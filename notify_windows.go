package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unicode/utf8"
	"unsafe"
)

func notifyReceived(path string) {
	exe, e := os.Executable()
	if e != nil {
		return
	}
	token := base64.RawURLEncoding.EncodeToString([]byte(filepath.Base(path)))
	shell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	cmd := exec.Command(shell, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(filepath.Dir(exe), "Notify.ps1"), token)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if cmd.Start() == nil {
		go cmd.Wait()
	}
}

func revealReceived(raw string) error {
	const prefix = "lansend-reveal://file/"
	if !strings.HasPrefix(raw, prefix) {
		return errors.New("invalid receipt")
	}
	name, e := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(raw, prefix))
	if e != nil || !utf8.Valid(name) || !safeName(string(name)) {
		return errors.New("invalid file name")
	}
	home, e := os.UserHomeDir()
	if e != nil {
		return e
	}
	path := filepath.Join(home, "Downloads", "LANSend", string(name))
	wide, e := syscall.UTF16PtrFromString(path)
	if e != nil {
		return e
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ole := syscall.NewLazyDLL("ole32.dll")
	hr, _, _ := ole.NewProc("CoInitializeEx").Call(0, 2)
	if int32(hr) < 0 {
		return fmt.Errorf("COM initialization: %x", hr)
	}
	defer ole.NewProc("CoUninitialize").Call()
	shell := syscall.NewLazyDLL("shell32.dll")
	var pidl uintptr
	hr, _, _ = shell.NewProc("SHParseDisplayName").Call(uintptr(unsafe.Pointer(wide)), 0, uintptr(unsafe.Pointer(&pidl)), 0, 0)
	if int32(hr) < 0 {
		return fmt.Errorf("file lookup: %x", hr)
	}
	defer ole.NewProc("CoTaskMemFree").Call(pidl)
	hr, _, _ = shell.NewProc("SHOpenFolderAndSelectItems").Call(pidl, 0, 0, 0)
	if int32(hr) < 0 {
		return fmt.Errorf("Explorer: %x", hr)
	}
	return nil
}
