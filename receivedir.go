package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func defaultReceiveDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Downloads", "LANSend"), nil
}

func receiveDirSettingPath(root string) string {
	return filepath.Join(root, "receive-dir")
}

func sameDirPath(a, b string) bool {
	ca, cb := filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(ca, cb)
	}
	return ca == cb
}

func receiveDir(root string) (string, error) {
	if root == "" {
		return defaultReceiveDir()
	}
	b, err := os.ReadFile(receiveDirSettingPath(root))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaultReceiveDir()
		}
		return "", err
	}
	dir := strings.TrimSpace(strings.TrimPrefix(string(b), "\ufeff"))
	if dir == "" || !filepath.IsAbs(dir) {
		return defaultReceiveDir()
	}
	return filepath.Clean(dir), nil
}

func isCustomReceiveDir(root string) bool {
	cur, err1 := receiveDir(root)
	def, err2 := defaultReceiveDir()
	return err1 == nil && err2 == nil && !sameDirPath(cur, def)
}

func validateWritableDir(dir string) error {
	if !filepath.IsAbs(dir) {
		return errors.New("收件目录必须是绝对路径")
	}
	if st, err := os.Stat(dir); err == nil && !st.IsDir() {
		return errors.New("所选路径不是文件夹")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return errors.New("所选目录无法写入，请检查权限或更换文件夹")
	}
	f, err := os.CreateTemp(dir, ".lansend-check-*")
	if err != nil {
		return errors.New("所选目录无法写入，请检查权限或更换文件夹")
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return nil
}

func setReceiveDir(root, dir string) (string, error) {
	def, err := defaultReceiveDir()
	if err != nil {
		return "", err
	}
	dir = strings.TrimSpace(dir)
	if dir == "" || sameDirPath(dir, def) {
		err := os.Remove(receiveDirSettingPath(root))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if err := os.MkdirAll(def, 0700); err != nil {
			return "", err
		}
		return def, nil
	}
	dir = filepath.Clean(dir)
	if err := validateWritableDir(dir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(root, "receive-dir-*")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := f.WriteString(dir + "\n"); err != nil {
		_ = f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, receiveDirSettingPath(root)); err != nil {
		return "", err
	}
	cleanup = false
	return dir, nil
}
