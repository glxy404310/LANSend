//go:build !windows && !darwin

package main

func collectLocalIPv4Candidates() []localIPCandidate {
	return collectGenericLocalIPv4s()
}
