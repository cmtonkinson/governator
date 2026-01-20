// Package inflight provides tests for in-flight tracking persistence.
package inflight

import (
	"os"
	"path/filepath"
	"testing"
)

// TestStoreAddAndLoadRoundTrip ensures in-flight entries persist correctly.
func TestStoreAddAndLoadRoundTrip(t *testing.T) {
	store := newTempStore(t)

	if _, err := store.Add("task-a", "task-b"); err != nil {
		t.Fatalf("add tasks: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load tasks: %v", err)
	}

	if !loaded.Contains("task-a") || !loaded.Contains("task-b") {
		t.Fatalf("expected loaded set to include task-a and task-b")
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 in-flight tasks, got %d", len(loaded))
	}
}

// TestStoreRemoveStaleEntry ensures removing missing entries is safe.
func TestStoreRemoveStaleEntry(t *testing.T) {
	store := newTempStore(t)

	if _, err := store.Add("task-a"); err != nil {
		t.Fatalf("add task: %v", err)
	}

	if _, err := store.Remove("task-missing"); err != nil {
		t.Fatalf("remove stale task: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load tasks: %v", err)
	}
	if !loaded.Contains("task-a") {
		t.Fatalf("expected task-a to remain in-flight")
	}
}

// TestStoreRemoveClearsEntry ensures tracked tasks can be cleared.
func TestStoreRemoveClearsEntry(t *testing.T) {
	store := newTempStore(t)

	if _, err := store.Add("task-a"); err != nil {
		t.Fatalf("add task: %v", err)
	}

	if _, err := store.Remove("task-a"); err != nil {
		t.Fatalf("remove task: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load tasks: %v", err)
	}
	if loaded.Contains("task-a") {
		t.Fatalf("expected task-a to be cleared")
	}
}

// newTempStore builds a Store rooted at a temporary directory.
func newTempStore(t *testing.T) Store {
	t.Helper()
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store
}

// TestStoreLoadMissingFileReturnsEmpty ensures missing files return an empty set.
func TestStoreLoadMissingFileReturnsEmpty(t *testing.T) {
	store := newTempStore(t)
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load tasks: %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("expected empty in-flight set, got %d", len(loaded))
	}
}

// TestStoreSaveCreatesDirectory ensures Save creates local state directories.
func TestStoreSaveCreatesDirectory(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := store.Save(Set{}); err != nil {
		t.Fatalf("save: %v", err)
	}
	path := filepath.Join(root, localStateDirName, inFlightFileName)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected in-flight file to exist: %v", err)
	}
}
