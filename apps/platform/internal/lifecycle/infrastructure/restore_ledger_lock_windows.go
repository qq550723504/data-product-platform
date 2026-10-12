package infrastructure

import (
	"os"

	"golang.org/x/sys/windows"
)

func lockRestoreLedger(f *os.File, exclusive bool) (func(), error) {
	var flags uint32
	if exclusive {
		flags = windows.LOCKFILE_EXCLUSIVE_LOCK
	}
	var overlap windows.Overlapped
	handle := windows.Handle(f.Fd())
	if err := windows.LockFileEx(handle, flags, 0, 1, 0, &overlap); err != nil {
		return nil, err
	}
	return func() { _ = windows.UnlockFileEx(handle, 0, 1, 0, &overlap) }, nil
}
