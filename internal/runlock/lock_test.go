// Tests for run lock acquisition and stale handling.
package runlock

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestAcquireReleaseLock verifies a single run acquires and releases the lock.
func TestAcquireReleaseLock(t *testing.T) {
	dir := t.TempDir()

	lock, err := Acquire(dir)
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}

	lockPath := filepath.Join(dir, localStateDirName, runLockFileName)
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("read lock file: %v", err)
	}
	if !strings.Contains(string(data), "pid=") {
		t.Fatalf("expected pid metadata in lock file")
	}
	if !strings.Contains(string(data), "started_at=") {
		t.Fatalf("expected started_at metadata in lock file")
	}

	if err := lock.Release(); err != nil {
		t.Fatalf("release lock: %v", err)
	}
	if _, err := os.Stat(lockPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected lock file to be removed")
	}
}

// TestAcquireLockContention ensures a second run reports the active lock.
func TestAcquireLockContention(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, localStateDirName, runLockFileName)

	cmd := exec.Command(os.Args[0], "-test.run=TestRunLockHelperProcess", "--", dir)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}

	reader := bufio.NewReader(stdout)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read helper output: %v", err)
	}
	if strings.TrimSpace(line) != "locked" {
		t.Fatalf("expected helper to report lock acquired, got %q", line)
	}

	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("expected lock file to exist: %v", err)
	}

	_, err = Acquire(dir)
	if err == nil {
		t.Fatalf("expected lock contention error, got nil")
	}
	if !strings.Contains(err.Error(), "already held") {
		t.Fatalf("expected lock contention message, got %v", err)
	}

	_ = stdin.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wait helper: %v", err)
	}
}

// TestAcquireStaleLock ensures stale locks provide operator guidance.
func TestAcquireStaleLock(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, localStateDirName, runLockFileName)
	if err := os.MkdirAll(filepath.Dir(lockPath), localStateDirMode); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	info := lockInfo{pid: 999999, startedAt: time.Now().UTC().Add(-time.Hour)}
	if err := os.WriteFile(lockPath, []byte(fmt.Sprintf("pid=%d\nstarted_at=%s\n", info.pid, info.startedAt.Format(time.RFC3339))), runLockFileMode); err != nil {
		t.Fatalf("write stale lock: %v", err)
	}

	_, err := Acquire(dir)
	if err == nil {
		t.Fatalf("expected stale lock error, got nil")
	}
	if !strings.Contains(err.Error(), "stale run lock") {
		t.Fatalf("expected stale lock guidance, got %v", err)
	}
}

// TestRunLockHelperProcess holds the lock to simulate contention.
func TestRunLockHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	root, err := helperRepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}
	lock, err := Acquire(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lock helper failed: %v\n", err)
		os.Exit(2)
	}
	defer func() {
		_ = lock.Release()
	}()

	fmt.Fprintln(os.Stdout, "locked")
	_, _ = io.Copy(io.Discard, os.Stdin)
}

// helperRepoRoot extracts the repo root argument from the helper process args.
func helperRepoRoot() (string, error) {
	for i, arg := range os.Args {
		if arg == "--" && i+1 < len(os.Args) {
			return os.Args[i+1], nil
		}
	}
	return "", fmt.Errorf("missing repo root")
}
