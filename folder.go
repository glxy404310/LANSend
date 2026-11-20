package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func packFolder(folder string) (string, error) {
	f, err := os.CreateTemp("", "lansend-folder-*.zip")
	if err != nil {
		return "", err
	}
	output := f.Name()
	zw := zip.NewWriter(f)
	err = filepath.WalkDir(folder, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == folder || path == output {
			return nil
		}
		rel, e := filepath.Rel(folder, path)
		if e != nil {
			return e
		}
		name := filepath.ToSlash(rel)
		if !folderMember(name) {
			return fmt.Errorf("不兼容的文件名：%s", name)
		}
		if entry.IsDir() {
			_, e = zw.CreateHeader(&zip.FileHeader{Name: name + "/", Method: zip.Store})
			return e
		}
		st, e := entry.Info()
		if e != nil {
			return e
		}
		if !st.Mode().IsRegular() {
			return fmt.Errorf("无法发送链接或特殊文件：%s", name)
		}
		src, e := os.Open(path)
		if e != nil {
			return e
		}
		defer src.Close()
		current, e := src.Stat()
		if e != nil {
			return e
		}
		if !os.SameFile(st, current) {
			return fmt.Errorf("文件已变更：%s", name)
		}
		dst, e := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if e != nil {
			return e
		}
		_, e = io.Copy(dst, src)
		return e
	})
	closeZip := zw.Close()
	closeFile := f.Close()
	if err = errors.Join(err, closeZip, closeFile); err != nil {
		os.Remove(output)
		return "", err
	}
	return output, nil
}
func folderMember(name string) bool {
	if !fs.ValidPath(name) || name == "." {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if !safeName(part) {
			return false
		}
	}
	return true
}

func saveFolder(dest, name string, body io.Reader, size int64) (string, error) {
	if !safeName(name) {
		return "", errors.New("文件夹名称无效")
	}
	archive, err := os.CreateTemp(dest, ".lansend-*.part")
	if err != nil {
		return "", err
	}
	defer os.Remove(archive.Name())
	defer archive.Close()
	n, err := io.Copy(archive, body)
	if err != nil {
		return "", err
	}
	if n != size {
		return "", io.ErrUnexpectedEOF
	}
	zr, err := zip.NewReader(archive, n)
	if err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(dest, ".lansend-folder-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	remaining := uint64(n)
	for _, entry := range zr.File {
		if entry.UncompressedSize64 > remaining {
			return "", errors.New("文件夹数据长度无效")
		}
		remaining -= entry.UncompressedSize64
		rel := strings.TrimSuffix(entry.Name, "/")
		if !folderMember(rel) || entry.Method != zip.Store || (!entry.Mode().IsRegular() && !entry.FileInfo().IsDir()) {
			return "", errors.New("文件夹条目无效")
		}
		target := filepath.Join(stage, filepath.FromSlash(rel))
		if entry.FileInfo().IsDir() {
			if err = os.MkdirAll(target, 0700); err != nil {
				return "", err
			}
			continue
		}
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return "", err
		}
		if err = extractMember(target, entry); err != nil {
			return "", err
		}
	}
	// Choose a fresh name atomically, including on simultaneous receives.
	for i := 0; ; i++ {
		label := name
		if i > 0 {
			label = name + " (" + strconv.Itoa(i) + ")"
		}
		target := filepath.Join(dest, label)
		err = publishDirectory(stage, target)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		return target, nil
	}
}
func extractMember(path string, entry *zip.File) error {
	src, err := entry.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, src)
	return errors.Join(err, dst.Close())
}
