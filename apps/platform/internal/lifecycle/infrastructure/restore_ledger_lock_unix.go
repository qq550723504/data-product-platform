//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package infrastructure

import (
	"os"

	"golang.org/x/sys/unix"
)

func lockRestoreLedger(f *os.File, exclusive bool) (func(), error) {
	mode := unix.LOCK_SH
	if exclusive {
		mode = unix.LOCK_EX
	}
	if err := unix.Flock(int(f.Fd()), mode); err != nil {
		return nil, err
	}
	return func() { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN) }, nil
}
