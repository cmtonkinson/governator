// Package runlock provides exclusive locking for governator runs.
package runlock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// localStateDirName is the relative path for transient governator state.
	localStateDirName = "_governator/_local_state"
	// runLockFileName is the filename used for run locking.
	runLockFileName = "run.lock"
	// runLockFileMode defines the permissions for the lock file.
	runLockFileMode = 0o644
	// localStateDirMode defines the permissions for the local state directory.
	localStateDirMode = 0o755
)

var ErrLockHeld = errors.New("run lock already held")

// Lock holds the acquired run lock file handle.
type Lock struct {
	file *os.File
	path string
}

// Acquire attempts to create and lock the run lock file for the repo.
func Acquire(repoRoot string) (*Lock, error) {
	if repoRoot == "" {
		return nil, errors.New("repo root is required")
	}

	lockPath := filepath.Join(repoRoot, localStateDirName, runLockFileName)
	if err := os.MkdirAll(filepath.Dir(lockPath), localStateDirMode); err != nil {
		return nil, fmt.Errorf("create run lock directory %s: %w", filepath.Dir(lockPath), err)
	}

	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, runLockFileMode)
	if err != nil {
		return nil, fmt.Errorf("open run lock %s: %w", lockPath, err)
	}

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if isLockBusy(err) {
			return nil, fmt.Errorf("%w: %v", ErrLockHeld, formatHeldLockError(lockPath))
		}
		return nil, fmt.Errorf("lock run lock %s: %w", lockPath, err)
	}

	if err := checkForStaleLock(lockPath); err != nil {
		_ = releaseFileLock(file)
		_ = file.Close()
		return nil, err
	}

	info := lockInfo{pid: os.Getpid(), startedAt: time.Now().UTC()}
	if err := writeLockInfo(file, info); err != nil {
		_ = releaseFileLock(file)
		_ = file.Close()
		return nil, err
	}

	return &Lock{file: file, path: lockPath}, nil
}

// Release unlocks and removes the run lock file.
func (lock *Lock) Release() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	if err := releaseFileLock(lock.file); err != nil {
		_ = lock.file.Close()
		return err
	}
	if err := lock.file.Close(); err != nil {
		return err
	}
	if err := os.Remove(lock.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove run lock %s: %w", lock.path, err)
	}
	return nil
}

// lockInfo captures metadata written to the lock file.
type lockInfo struct {
	pid       int
	startedAt time.Time
}

// checkForStaleLock inspects any existing lock info and rejects stale entries.
func checkForStaleLock(lockPath string) error {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read run lock %s: %w", lockPath, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}

	info, err := parseLockInfo(data)
	if err != nil {
		return fmt.Errorf("stale run lock at %s: %w; remove the lock file to continue", lockPath, err)
	}

	active, err := processExists(info.pid)
	if err != nil {
		return fmt.Errorf("verify run lock pid %d: %w", info.pid, err)
	}
	if !active {
		return fmt.Errorf("stale run lock at %s (pid %d since %s); remove the lock file to continue",
			lockPath, info.pid, info.startedAt.Format(time.RFC3339))
	}
	return nil
}

// formatHeldLockError builds a lock-held error message with metadata when available.
func formatHeldLockError(lockPath string) error {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return fmt.Errorf("run lock %s is already held; wait for the other process to finish", lockPath)
	}
	info, err := parseLockInfo(data)
	if err != nil {
		return fmt.Errorf("run lock %s is already held; wait for the other process to finish", lockPath)
	}
	return fmt.Errorf("run lock %s is already held by pid %d since %s; wait for the other process to finish",
		lockPath, info.pid, info.startedAt.Format(time.RFC3339))
}

// parseLockInfo reads pid and timestamp metadata from the lock file.
func parseLockInfo(data []byte) (lockInfo, error) {
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	info := lockInfo{}
	for _, line := range lines {
		if strings.HasPrefix(line, "pid=") {
			pid, err := parseInt(strings.TrimPrefix(line, "pid="))
			if err != nil {
				return lockInfo{}, fmt.Errorf("parse pid: %w", err)
			}
			info.pid = pid
			continue
		}
		if strings.HasPrefix(line, "started_at=") {
			ts := strings.TrimPrefix(line, "started_at=")
			parsed, err := time.Parse(time.RFC3339, ts)
			if err != nil {
				return lockInfo{}, fmt.Errorf("parse started_at: %w", err)
			}
			info.startedAt = parsed
		}
	}
	if info.pid == 0 {
		return lockInfo{}, errors.New("missing pid")
	}
	if info.startedAt.IsZero() {
		return lockInfo{}, errors.New("missing started_at")
	}
	return info, nil
}

// writeLockInfo truncates and writes lock metadata to the lock file.
func writeLockInfo(file *os.File, info lockInfo) error {
	if file == nil {
		return errors.New("lock file is required")
	}
	if err := file.Truncate(0); err != nil {
		return fmt.Errorf("truncate run lock: %w", err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		return fmt.Errorf("seek run lock: %w", err)
	}
	payload := fmt.Sprintf("pid=%d\nstarted_at=%s\n", info.pid, info.startedAt.Format(time.RFC3339))
	if _, err := file.WriteString(payload); err != nil {
		return fmt.Errorf("write run lock: %w", err)
	}
	return nil
}

// parseInt parses an integer from the provided string.
func parseInt(value string) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, err
	}
	if parsed <= 0 {
		return 0, errors.New("pid must be positive")
	}
	return parsed, nil
}

// processExists checks whether a PID appears to reference a running process.
func processExists(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, syscall.ESRCH) {
		return false, nil
	}
	if errors.Is(err, syscall.EPERM) {
		return true, nil
	}
	return false, err
}

// releaseFileLock unlocks an advisory lock on the file.
func releaseFileLock(file *os.File) error {
	if file == nil {
		return nil
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_UN); err != nil {
		return fmt.Errorf("unlock run lock: %w", err)
	}
	return nil
}

// isLockBusy returns true when the lock is already held by another process.
func isLockBusy(err error) bool {
	return errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK)
}
