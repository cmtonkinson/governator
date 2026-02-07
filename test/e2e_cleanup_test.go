// Package test provides helpers for managing E2E test repositories.
package test

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"sync"
	"testing"
)

var (
	e2ePreserveAll = flag.Bool("e2e-preserve-all", false, "Preserve all E2E test repositories")
	e2eClearAll    = flag.Bool("e2e-clear-all", false, "Clear all E2E test repositories, even on failure")
)

// preservedRepo captures a repository path and preservation reason.
type preservedRepo struct {
	Path   string
	Reason string
}

var (
	preservedMu    sync.Mutex
	preservedRepos = map[string]preservedRepo{}
)

// TrackE2ERepo registers a repo root for cleanup or preservation based on test outcome.
func TrackE2ERepo(t *testing.T, repoRoot string) {
	t.Helper()
	t.Cleanup(func() {
		preserve, reason := shouldPreserveRepo(t.Failed())
		if preserve {
			recordPreservedRepo(repoRoot, reason)
			return
		}
		if err := os.RemoveAll(repoRoot); err != nil {
			recordPreservedRepo(repoRoot, fmt.Sprintf("cleanup_failed: %v", err))
		}
	})
}

// TestMain parses flags, runs the suite, and prints preserved repositories.
func TestMain(m *testing.M) {
	flag.Parse()
	exitCode := m.Run()
	printPreservedRepos()
	os.Exit(exitCode)
}

// shouldPreserveRepo decides whether to retain a repo based on flags and failure.
func shouldPreserveRepo(failed bool) (bool, string) {
	if *e2eClearAll {
		return false, ""
	}
	if *e2ePreserveAll {
		return true, "forced"
	}
	if failed {
		return true, "failure"
	}
	return false, ""
}

// recordPreservedRepo tracks a preserved repository for summary output.
func recordPreservedRepo(path string, reason string) {
	preservedMu.Lock()
	defer preservedMu.Unlock()
	preservedRepos[path] = preservedRepo{Path: path, Reason: reason}
}

// printPreservedRepos emits a summary of preserved repositories to stdout.
func printPreservedRepos() {
	preservedMu.Lock()
	defer preservedMu.Unlock()

	fmt.Fprintln(os.Stdout, "=== Preserved E2E Repositories ===")
	if len(preservedRepos) == 0 {
		fmt.Fprintln(os.Stdout, "None")
		return
	}

	paths := make([]string, 0, len(preservedRepos))
	for path := range preservedRepos {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	for _, path := range paths {
		entry := preservedRepos[path]
		fmt.Fprintf(os.Stdout, "%s [%s]\n", entry.Path, entry.Reason)
	}
}
