//go:build unix

package daemon

import (
	"errors"
	"os"
	"syscall"
)

func flock(f *os.File) error {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return ErrAlreadyRunning
	}
	return err
}
