// Package index provides task index persistence helpers.
package index

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

var (
	// ErrLockHeld indicates another process currently holds the index write lock.
	ErrLockHeld = errors.New("task index lock already held")
)

// indexLock holds the advisory lock for a task index write.
type indexLock struct {
	file *os.File
	path string
}

// WriteLock is an exported handle for a held task index write lock.
type WriteLock struct {
	inner *indexLock
}

// AcquireWriteLock acquires an exclusive advisory write lock for the task index at path.
func AcquireWriteLock(path string) (*WriteLock, error) {
	lock, err := lockIndexForWrite(path)
	if err != nil {
		return nil, err
	}
	return &WriteLock{inner: lock}, nil
}

// Release unlocks and closes the underlying lock file.
func (lock *WriteLock) Release() error {
	if lock == nil || lock.inner == nil {
		return nil
	}
	return lock.inner.Release()
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
			return nil, fmt.Errorf("%w: %s", ErrLockHeld, lockPath)
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
