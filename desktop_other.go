//go:build !windows

package main

func desktopEntry(args []string) bool { return false }

func receive(root string) error { return serveFiles(root, notifyReceived) }
