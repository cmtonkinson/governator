// Package plan provides the explicit planning command implementation.
package plan

import (
	"errors"
	"fmt"
	"github.com/cmtonkinson/governator/internal/bootstrap"
	"github.com/cmtonkinson/governator/internal/config"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	localStateDirName   = "_governator/_local_state"
	plannerStateDirName = "plan"
	plannerStateMode    = 0o755
)

// Options configures the plan command behavior.
type Options struct {
	Stdout          io.Writer
	Stderr          io.Writer
	Warn            func(string)
	ConfigOverrides map[string]any
	ReasoningEffort string
}

// Result captures the plan command outputs.
type Result struct {
	BootstrapRan    bool
	BootstrapResult bootstrap.Result
	PromptDir       string
	Prompts         []PromptInfo
}

// PromptInfo describes a serialized agent prompt file emitted during planning.
type PromptInfo struct {
	Agent string
	Path  string
}

// Run executes the planning preparation, ensuring the repo is bootstrapped and
// laying out the prompts that each agent should consume.
func Run(repoRoot string, options Options) (Result, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return Result{}, fmt.Errorf("repo root is required")
	}

	stdout := resolveWriter(options.Stdout)
	stderr := resolveWriter(options.Stderr)
	warn := options.Warn
	if warn == nil {
		warn = resolveWarn(stderr)
	}

	ranBootstrap, bootstrapResult, err := ensureBootstrap(repoRoot)
	if err != nil {
		return Result{}, err
	}
	if ranBootstrap {
		fmt.Fprintln(stdout, "bootstrap ok")
	}

	cfg, err := config.Load(repoRoot, options.ConfigOverrides, warn)
	if err != nil {
		return Result{}, err
	}
	reasoningEffort := resolveReasoningEffort(options.ReasoningEffort)
	prompts, promptDir, err := prepareAgentPrompts(repoRoot, warn, cfg, reasoningEffort)
	if err != nil {
		return Result{}, err
	}

	fmt.Fprintf(stdout, "plan ok prompts=%d\n", len(prompts))

	return Result{
		BootstrapRan:    ranBootstrap,
		BootstrapResult: bootstrapResult,
		PromptDir:       repoRelativePath(repoRoot, promptDir),
		Prompts:         prompts,
	}, nil
}

// ensureBootstrap creates required Power Six artifacts when they are missing.
func ensureBootstrap(repoRoot string) (bool, bootstrap.Result, error) {
	missing, err := requiredBootstrapMissing(repoRoot)
	if err != nil {
		return false, bootstrap.Result{}, err
	}
	if !missing {
		return false, bootstrap.Result{}, nil
	}
	result, err := bootstrap.Run(repoRoot, bootstrap.Options{Force: false})
	if err != nil {
		return true, result, fmt.Errorf("bootstrap failed: %w", err)
	}
	return true, result, nil
}

// requiredBootstrapMissing reports whether required Power Six artifacts are absent.
func requiredBootstrapMissing(repoRoot string) (bool, error) {
	docsDir := filepath.Join(repoRoot, "_governator", "docs")
	for _, artifact := range bootstrap.Artifacts() {
		if !artifact.Required {
			continue
		}
		path := filepath.Join(docsDir, artifact.Name)
		_, err := os.Stat(path)
		if err == nil {
			continue
		}
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, fmt.Errorf("stat bootstrap artifact %s: %w", path, err)
	}
	return false, nil
}

// resolveWarn returns a warning sink that writes to stderr.
func resolveWarn(stderr io.Writer) func(string) {
	return func(message string) {
		if message == "" {
			return
		}
		fmt.Fprintf(stderr, "warning: %s\n", message)
	}
}

// resolveWriter returns io.Discard when the writer is nil.
func resolveWriter(writer io.Writer) io.Writer {
	if writer == nil {
		return io.Discard
	}
	return writer
}

// repoRelativePath returns a repository-relative path using forward slashes.
func repoRelativePath(root string, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func resolveReasoningEffort(level string) string {
	return strings.TrimSpace(level)
}
