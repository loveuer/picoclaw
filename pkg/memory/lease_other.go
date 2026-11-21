//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly && !windows

package memory

import (
	"errors"
	"os"
)

func lockFile(_ *os.File, _ bool) error {
	return errors.New("memory database locking is unavailable on this platform")
}
