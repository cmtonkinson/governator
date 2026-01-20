// Tests for task index file locking behavior.
package index

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestIndexSaveLockContention ensures Save fails fast when the lock is held.
func TestIndexSaveLockContention(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "index.json")

	cmd := exec.Command(os.Args[0], "-test.run=TestIndexLockHelperProcess", "--", indexPath)
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

	idx := Index{
		SchemaVersion: 1,
		Digests: Digests{
			GovernatorMD: "",
			PlanningDocs: map[string]string{},
		},
	}
	err = Save(indexPath, idx)
	if err == nil {
		t.Fatalf("expected lock contention error, got nil")
	}
	if !strings.Contains(err.Error(), "already held") {
		t.Fatalf("expected lock contention error, got %v", err)
	}

	_ = stdin.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wait helper: %v", err)
	}
}

// TestIndexLockHelperProcess holds the lock to simulate contention.
func TestIndexLockHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	lockPath, err := helperLockPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}
	lock, err := lockIndexForWrite(lockPath)
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

// helperLockPath extracts the lock path argument from the helper process args.
func helperLockPath() (string, error) {
	for i, arg := range os.Args {
		if arg == "--" && i+1 < len(os.Args) {
			return os.Args[i+1], nil
		}
	}
	return "", fmt.Errorf("missing lock path")
}
