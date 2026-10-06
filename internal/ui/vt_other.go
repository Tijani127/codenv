//go:build !windows

package ui

import "sync"

var (
	vtOnce      sync.Once
	vtSucceeded bool
)

func enableVirtualTerminal() bool {
	vtOnce.Do(func() { vtSucceeded = true })
	return vtSucceeded
}
