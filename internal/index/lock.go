// Package index provides task index persistence helpers.
package index

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// indexLock holds the advisory lock for a task index write.
type indexLock struct {
	file *os.File
	path string
}

// lockIndexForWrite acquires an exclusive lock for task index writes.
func lockIndexForWrite(path string) (*indexLock, error) {
	lockPath := path + ".lock"
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, indexFileMode)
	if err != nil {
		return nil, fmt.Errorf("open task index lock %s: %w", lockPath, err)
	}

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if isLockBusy(err) {
			return nil, fmt.Errorf("task index lock %s is already held; wait for the other process to finish", lockPath)
		}
		return nil, fmt.Errorf("lock task index %s: %w", lockPath, err)
	}

	return &indexLock{file: file, path: lockPath}, nil
}

// Release unlocks the index lock and closes the lock file.
func (lock *indexLock) Release() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	if err := syscall.Flock(int(lock.file.Fd()), syscall.LOCK_UN); err != nil {
		_ = lock.file.Close()
		return err
	}
	return lock.file.Close()
}

// isLockBusy returns true when the lock is already held by another process.
func isLockBusy(err error) bool {
	return errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK)
}
