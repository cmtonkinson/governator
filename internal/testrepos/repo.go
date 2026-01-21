package testrepos

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TempRepo represents a temporary git repository that can be reused in tests.
type TempRepo struct {
	Root string
}

// New creates a temporary git repository with an initial commit that tests can run against.
func New(tb testing.TB) *TempRepo {
	tb.Helper()
	root, err := os.MkdirTemp("", "governator-test-repo-*")
	if err != nil {
		tb.Fatalf("create temp repo directory: %v", err)
	}

	repo := &TempRepo{Root: root}
	tb.Cleanup(func() {
		if cleanupErr := repo.Cleanup(); cleanupErr != nil {
			tb.Fatalf("cleanup temp repo: %v", cleanupErr)
		}
	})

	repo.initialize(tb)
	return repo
}

// RunGit executes git in the repository directory and fails the test if git returns an error.
func (r *TempRepo) RunGit(tb testing.TB, args ...string) string {
	tb.Helper()
	output, err := runGit(r.Root, args...)
	if err != nil {
		tb.Fatalf("git %s failed: %v: %s", strings.Join(args, " "), err, output)
	}
	return output
}

// Cleanup removes the temporary repository root. Missing directories are treated as success.
func (r *TempRepo) Cleanup() error {
	if r == nil || r.Root == "" {
		return nil
	}
	if err := os.RemoveAll(r.Root); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove temp repo %s: %w", r.Root, err)
	}
	return nil
}

func (r *TempRepo) initialize(tb testing.TB) {
	tb.Helper()
	r.RunGit(tb, "init", "--initial-branch=main")
	r.RunGit(tb, "config", "user.name", "Governator Test")
	r.RunGit(tb, "config", "user.email", "test@example.com")

	readme := filepath.Join(r.Root, "README.md")
	if err := os.WriteFile(readme, []byte("# Temp Governator Repository\n"), 0o644); err != nil {
		tb.Fatalf("write README: %v", err)
	}

	r.RunGit(tb, "add", "README.md")
	r.RunGit(tb, "commit", "-m", "Initial commit")
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(output), nil
}
