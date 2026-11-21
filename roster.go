package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type peerFile struct {
	config
	Others []config `json:",omitempty"`
}

var errMissingPeer = errors.New("设备尚未配对，请在设备管理中检查")

func peersAt(path string) ([]config, error) {
	b, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	var f peerFile
	if e = json.Unmarshal(b, &f); e != nil {
		return nil, e
	}
	if f.PeerHash == "" {
		return nil, nil
	}
	return append([]config{f.config}, f.Others...), nil
}
func peerIndex(all []config, h string) int {
	return slices.IndexFunc(all, func(c config) bool { return c.PeerHash == h })
}
func findPeer(path, h string) (config, error) {
	all, e := peersAt(path)
	if e != nil {
		return config{}, e
	}
	if i := peerIndex(all, h); i >= 0 {
		return all[i], nil
	}
	return config{}, errMissingPeer
}
func editPeers(path string, edit func([]config) ([]config, error)) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	lock, e := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer lock.Close()
	if e = lockPeerFile(lock); e != nil {
		return e
	}
	all, e := peersAt(path)
	if e != nil {
		return e
	}
	all, e = edit(all)
	if e != nil {
		return e
	}
	var data peerFile
	if len(all) > 0 {
		data.config = all[0]
		data.Others = all[1:]
	}
	f, e := os.CreateTemp(filepath.Dir(path), "pair-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	e = json.NewEncoder(f).Encode(data)
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return os.Rename(f.Name(), path)
}
func savePair(path string, c config) error { return storePair(path, c, false) }
func storePair(path string, c config, existingOnly bool) error {
	return editPeers(path, func(all []config) ([]config, error) {
		i := peerIndex(all, c.PeerHash)
		if existingOnly && i < 0 {
			return nil, errMissingPeer
		}
		c.Certificate = ""
		c.PrivateKey = ""
		if i >= 0 {
			keepPeerRoutes(&c, all[i])
			c.Enabled = all[i].Enabled
			all[i] = c
			return all, nil
		}
		return append(all, c), nil
	})
}
func changePeer(root, h, action string) error {
	return editPeers(filepath.Join(root, "peer.json"), func(all []config) ([]config, error) {
		return applyPeerAction(all, h, action)
	})
}

func applyPeerAction(all []config, h, action string) ([]config, error) {
	i := peerIndex(all, h)
	if i < 0 {
		return nil, errMissingPeer
	}
	switch action {
	case "enable-peer":
		all[i].Enabled = true
		if !all[0].Enabled {
			all[0], all[i] = all[i], all[0]
		}
	case "disable-peer":
		all[i].Enabled = false
	case "select-peer":
		if !all[i].Enabled {
			return nil, errors.New("请先启用设备")
		}
		all[0], all[i] = all[i], all[0]
	case "unpair":
		all = slices.Delete(all, i, i+1)
	default:
		return nil, errors.New("未知设备操作")
	}
	if len(all) > 0 && !all[0].Enabled {
		for j, c := range all {
			if c.Enabled {
				all[0], all[j] = all[j], all[0]
				break
			}
		}
	}
	return all, nil
}

func peerLabel(root string, c config) string {
	for _, s := range []string{peerNickname(root, c.PeerHash), c.PeerName} {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return "已配对设备"
}
