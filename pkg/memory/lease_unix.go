//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package memory

import (
	"os"

	"golang.org/x/sys/unix"
)

func lockFile(f *os.File, exclusive bool) error {
	flag := unix.LOCK_SH
	if exclusive {
		flag = unix.LOCK_EX
	}
	return unix.Flock(int(f.Fd()), flag|unix.LOCK_NB)
}
