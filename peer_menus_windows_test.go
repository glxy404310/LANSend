package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExplorerShortcuts(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(os.Getenv("SystemRoot"), "System32", "notepad.exe")
	script := strings.Replace(menuScript, `[Environment]::GetFolderPath('SendTo')`, "$env:LANSEND_TEST_DIR", 1)
	run := func(data, code string) {
		t.Helper()
		cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", code)
		cmd.Env = append(os.Environ(), "LANSEND_TEST_DIR="+dir, "LANSEND_MENU_EXE="+exe, "LANSEND_DEVICE_MENUS="+data)
		if out, e := cmd.CombinedOutput(); e != nil {
			t.Fatalf("%v: %s", e, out)
		}
	}
	hash := strings.Repeat("a", 64)
	run(`[{"Name":"LANSend · 客厅","Hash":"`+hash+`"}]`, script)
	run("[]", `$l=(New-Object -ComObject WScript.Shell).CreateShortcut((Join-Path $env:LANSEND_TEST_DIR 'LANSend · 客厅.lnk')); if ($l.TargetPath -ne $env:LANSEND_MENU_EXE -or $l.Arguments -ne 'send-to `+hash+`'){exit 1}`)
	run("[]", script)
	if _, e := os.Stat(filepath.Join(dir, "LANSend · 客厅.lnk")); !os.IsNotExist(e) {
		t.Fatal("stale Explorer shortcut", e)
	}
}
