//go:build !windows

package app

import "syscall"

// processUmask reads the process umask. Reading requires a set-and-restore
// round trip; the export command creates no other files concurrently, so the
// window is harmless.
func processUmask() int {
	mask := syscall.Umask(0)
	syscall.Umask(mask)
	return mask
}
