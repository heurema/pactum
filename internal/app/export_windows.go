//go:build windows

package app

// processUmask is zero on Windows: there is no umask, and chmod permission
// bits beyond read-only are ignored there anyway.
func processUmask() int { return 0 }
