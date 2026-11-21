package memory

import (
	"os"

	"golang.org/x/sys/windows"
)

func lockFile(f *os.File, exclusive bool) error {
	flag := uint32(windows.LOCKFILE_FAIL_IMMEDIATELY)
	if exclusive {
		flag |= windows.LOCKFILE_EXCLUSIVE_LOCK
	}
	var offset windows.Overlapped
	return windows.LockFileEx(windows.Handle(f.Fd()), flag, 0, 1, 0, &offset)
}
