// Package repo provides git repository root discovery helpers.
package repo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// gitDirName is the filesystem entry that marks a git repository root.
const gitDirName = ".git"

// ErrRepoNotFound is returned when no git repository root can be discovered.
var ErrRepoNotFound = errors.New("no git repository found")

// DiscoverRootFromCWD resolves the git repository root from the current working directory.
func DiscoverRootFromCWD() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	return DiscoverRoot(cwd)
}

// DiscoverRoot resolves the git repository root by walking upward from start.
func DiscoverRoot(start string) (string, error) {
	if start == "" {
		return "", fmt.Errorf("%w: provide a start directory or run inside a repo", ErrRepoNotFound)
	}

	absStart, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path for %s: %w", start, err)
	}

	absStart, err = filepath.EvalSymlinks(absStart)
	if err != nil {
		return "", fmt.Errorf("resolve symlinks for %s: %w", absStart, err)
	}

	info, err := os.Stat(absStart)
	if err != nil {
		return "", fmt.Errorf("stat start path %s: %w", absStart, err)
	}

	current := absStart
	if !info.IsDir() {
		current = filepath.Dir(absStart)
	}

	for {
		found, err := hasGitDir(current)
		if err != nil {
			return "", err
		}
		if found {
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	return "", fmt.Errorf("%w from %s; run inside a git repo or initialize one with `git init`", ErrRepoNotFound, absStart)
}

// hasGitDir reports whether the directory contains a .git entry.
func hasGitDir(dir string) (bool, error) {
	path := filepath.Join(dir, gitDirName)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat %s: %w", path, err)
	}
	return info.IsDir() || info.Mode().IsRegular(), nil
}
