package statedb

import (
	"os"
	"syscall"
)

// flock holds an exclusive cross-process lock on f, following the harness's
// withFileLock: agents run in separate processes, so an in-process mutex
// cannot serialize what they do to a shared file.
func flock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

func funlock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
