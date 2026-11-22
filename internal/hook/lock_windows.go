package hook

import (
	"errors"
	"syscall"
)

// errSharingViolation is ERROR_SHARING_VIOLATION: another handle has the
// file open without sharing.
const errSharingViolation syscall.Errno = 32

// tryLock opens the file at path with no sharing, so a second open fails
// until this handle closes. Windows closes it when the process dies, so a
// killed sync never leaves a stale lock. ok is false when another process
// holds it.
func tryLock(path string) (release func(), ok bool, err error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, false, err
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil,
		syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if errors.Is(err, errSharingViolation) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return func() { _ = syscall.CloseHandle(h) }, true, nil
}
