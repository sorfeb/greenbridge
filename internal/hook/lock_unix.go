//go:build !windows

package hook

import (
	"errors"
	"os"
	"syscall"
)

// tryLock takes an exclusive lock on the file at path without waiting. The
// kernel drops it when the process dies, so a killed sync never leaves a
// stale lock. ok is false when another process holds it.
func tryLock(path string) (release func(), ok bool, err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return func() { _ = f.Close() }, true, nil
}
