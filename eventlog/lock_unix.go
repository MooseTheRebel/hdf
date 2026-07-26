//go:build !windows

package eventlog

import (
	"os"
	"syscall"
)

// lockFile takes an exclusive advisory lock on path (creating it if needed)
// and returns a function that releases the lock. flock locks are tied to the
// open file description, so they also serialize goroutines within one
// process and are released by the kernel if the process dies. Mirrors
// config.lockFile — duplicated here rather than shared so eventlog stays
// free of a dependency on the config package.
func lockFile(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
