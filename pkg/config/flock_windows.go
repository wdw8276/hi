//go:build windows

package config

import "os"

// Windows does not support flock; these are no-ops.
// Multi-agent refcount races are unlikely on Windows and acceptable without locking.
func flockLock(_ *os.File) error   { return nil }
func flockUnlock(_ *os.File) error { return nil }
