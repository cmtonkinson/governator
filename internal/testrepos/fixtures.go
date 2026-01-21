package testrepos

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/cmtonkinson/governator/internal/repo"
)

// ApplyFixture copies a known fixture tree into the temporary repository.
//
// The fixture directories live under tests/fixtures/<name>. This helper walks
// the fixture, recreates directories inside the repo root, and copies files
// with their permissions so tests can start with deterministic state.
func (r *TempRepo) ApplyFixture(tb testing.TB, fixtureName string) {
	tb.Helper()

	projectRoot, err := repo.DiscoverRootFromCWD()
	if err != nil {
		tb.Fatalf("discover repo root: %v", err)
	}

	fixtureDir := filepath.Join(projectRoot, "tests", "fixtures", fixtureName)
	if _, err := os.Stat(fixtureDir); err != nil {
		tb.Fatalf("fixture %s not found: %v", fixtureName, err)
	}

	err = filepath.WalkDir(fixtureDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(fixtureDir, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(r.Root, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		mode := info.Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}
		if writeErr := os.WriteFile(target, data, mode); writeErr != nil {
			return writeErr
		}
		return nil
	})
	if err != nil {
		tb.Fatalf("copying fixture %s: %v", fixtureName, err)
	}
}
