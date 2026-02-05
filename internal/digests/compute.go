// Package digests computes content digests for governance and planning artifacts.
package digests

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/cmtonkinson/governator/internal/index"
)

const (
	governatorFileName = "GOVERNATOR.md"
	docsDirName        = "_governator/docs"
)

// Compute builds digests for GOVERNATOR.md and planning artifacts under _governator/docs.
func Compute(repoRoot string) (index.Digests, error) {
	if repoRoot == "" {
		return index.Digests{}, fmt.Errorf("repo root is required")
	}

	governatorPath := filepath.Join(repoRoot, governatorFileName)
	governatorDigest, err := digestFile(governatorPath)
	if err != nil {
		return index.Digests{}, err
	}

	planningDocs, err := docsDigests(repoRoot)
	if err != nil {
		return index.Digests{}, err
	}

	return index.Digests{
		GovernatorMD: governatorDigest,
		PlanningDocs: planningDocs,
	}, nil
}

// digestFile returns a sha256 digest for the file or an empty string if it is missing.
func digestFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", sum), nil
}

// docsDigests collects digests for all regular files under _governator/docs.
func docsDigests(repoRoot string) (map[string]string, error) {
	docsRoot := filepath.Join(repoRoot, docsDirName)
	entries := map[string]string{}

	err := filepath.WalkDir(docsRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		if entry.Name() == ".keep" || entry.Name() == "planning-notes.md" {
			return nil
		}

		digest, err := digestFile(path)
		if err != nil {
			return err
		}
		relative, err := repoRelativePath(repoRoot, path)
		if err != nil {
			return err
		}
		entries[relative] = digest
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return entries, nil
		}
		return nil, fmt.Errorf("walk planning docs %s: %w", docsRoot, err)
	}

	return entries, nil
}

// repoRelativePath returns a repo-relative path using forward slashes.
func repoRelativePath(repoRoot string, path string) (string, error) {
	relative, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return "", fmt.Errorf("relativize %s: %w", path, err)
	}
	return filepath.ToSlash(relative), nil
}
