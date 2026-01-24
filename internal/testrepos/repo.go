package testrepos

import (
	"errors"
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
	r.setupGovernatorBaseline(tb)
	r.RunGit(tb, "add", filepath.Join("_governator", "worker-contract.md"), filepath.Join("_governator", "reasoning"))
	r.RunGit(tb, "commit", "-m", "Add governator scaffolding")
}

func (r *TempRepo) setupGovernatorBaseline(tb testing.TB) {
	tb.Helper()
	baseDir := filepath.Join(r.Root, "_governator")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		tb.Fatalf("create _governator dir: %v", err)
	}

	contractPath := filepath.Join(baseDir, "worker-contract.md")
	if _, err := os.Stat(contractPath); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(contractPath, []byte("# Worker Contract\n"), 0o644); err != nil {
			tb.Fatalf("write worker contract: %v", err)
		}
	}

	reasoningDir := filepath.Join(baseDir, "reasoning")
	if err := os.MkdirAll(reasoningDir, 0o755); err != nil {
		tb.Fatalf("create reasoning dir: %v", err)
	}

	prompts := map[string]string{
		"high.md":   "# System Note\nHigh reasoning test prompt.\n",
		"medium.md": "# System Note\nMedium reasoning test prompt.\n",
		"low.md":    "# System Note\nLow reasoning test prompt.\n",
	}
	for name, content := range prompts {
		path := filepath.Join(reasoningDir, name)
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			tb.Fatalf("stat reasoning prompt %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			tb.Fatalf("write reasoning prompt %s: %v", name, err)
		}
	}
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
