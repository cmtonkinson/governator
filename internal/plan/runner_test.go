// Tests for the plan command runner.
package plan

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestRunWritesAgentPrompts ensures plan writes prompt files for each agent.
func TestRunWritesAgentPrompts(t *testing.T) {
	repoRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	if err := writeGovernatorDoc(repoRoot); err != nil {
		t.Fatalf("write governator: %v", err)
	}
	if err := writePlanPrereqs(repoRoot, "high"); err != nil {
		t.Fatalf("prepare plan prerequisites: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	result, err := Run(repoRoot, Options{Stdout: &stdout, Stderr: &stderr, ReasoningEffort: "high"})
	if err != nil {
		t.Fatalf("run plan: %v", err)
	}

	if !result.BootstrapRan {
		t.Fatalf("expected bootstrap to run")
	}
	if result.PromptDir != "_governator/_local_state/plan" {
		t.Fatalf("unexpected prompt dir %q", result.PromptDir)
	}
	if len(result.Prompts) != len(agentSpecs) {
		t.Fatalf("expected %d prompts, got %d", len(agentSpecs), len(result.Prompts))
	}
	if !strings.Contains(stdout.String(), "bootstrap ok") {
		t.Fatalf("expected bootstrap ok output, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "plan ok prompts=") {
		t.Fatalf("expected plan summary, got %q", stdout.String())
	}

	for _, prompt := range result.Prompts {
		path := filepath.Join(repoRoot, prompt.Path)
		assertFileExists(t, path)
		if prompt.Agent == "Architecture Baseline" {
			assertFileContains(t, path, "Emit the Power Six architecture artifacts")
		}
		if prompt.Agent == "Gap Analysis" {
			assertFileContains(t, path, "gap analysis agent")
		}
		assertFileContains(t, path, "## Worker contract")
	}
}

// TestRunMissingGovernator ensures plan fails when GOVERNATOR.md is absent.
func TestRunMissingGovernator(t *testing.T) {
	repoRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	_, err := Run(repoRoot, Options{})
	if err == nil {
		t.Fatal("expected error for missing GOVERNATOR.md")
	}
	if !strings.Contains(err.Error(), "GOVERNATOR.md") {
		t.Fatalf("expected missing GOVERNATOR.md error, got %v", err)
	}
}

// TestRunBootstrapFailure ensures bootstrap errors surface when docs dir is unwritable.
func TestRunBootstrapFailure(t *testing.T) {
	repoRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	if err := writeGovernatorDoc(repoRoot); err != nil {
		t.Fatalf("write governator: %v", err)
	}
	docsDir := filepath.Join(repoRoot, "_governator", "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.Chmod(docsDir, 0o555); err != nil {
		t.Fatalf("chmod docs: %v", err)
	}

	_, err := Run(repoRoot, Options{})
	if err == nil {
		t.Fatal("expected bootstrap failure")
	}
	if !strings.Contains(err.Error(), "bootstrap failed") {
		t.Fatalf("expected bootstrap failure message, got %v", err)
	}
}

func assertFileContains(t *testing.T, path string, substring string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(data), substring) {
		t.Fatalf("%s: missing %q", path, substring)
	}
}

// writeGovernatorDoc creates a minimal GOVERNATOR.md file.
func writeGovernatorDoc(repoRoot string) error {
	path := filepath.Join(repoRoot, "GOVERNATOR.md")
	content := "# Governator\n\nConstraints: alignment required.\n"
	return os.WriteFile(path, []byte(content), 0o644)
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file %s: %v", path, err)
	}
}

func writePlanPrereqs(repoRoot, reasoningEffort string) error {
	if err := writeWorkerContract(repoRoot); err != nil {
		return err
	}
	if err := writeReasoningEffort(repoRoot, reasoningEffort); err != nil {
		return err
	}
	for _, role := range uniqueAgentRoles() {
		if err := writeRolePrompt(repoRoot, role); err != nil {
			return err
		}
	}
	return nil
}

func writeWorkerContract(repoRoot string) error {
	dir := filepath.Join(repoRoot, "_governator")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "worker-contract.md")
	content := "# Worker Contract\n\nPlease obey the worker contract rules.\n"
	return os.WriteFile(path, []byte(content), 0o644)
}

func writeReasoningEffort(repoRoot, effort string) error {
	if effort == "" {
		return errors.New("reasoning effort is required")
	}
	dir := filepath.Join(repoRoot, "_governator", "reasoning")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, effort+".md")
	content := fmt.Sprintf("# Reasoning effort\nGuidance for %s reasoning.\n", effort)
	return os.WriteFile(path, []byte(content), 0o644)
}

func writeRolePrompt(repoRoot, role string) error {
	dir := filepath.Join(repoRoot, "_governator", "roles")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, role+".md")
	content := fmt.Sprintf("# Role: %s\nRole prompt content for %s.\n", role, role)
	return os.WriteFile(path, []byte(content), 0o644)
}

func uniqueAgentRoles() []string {
	set := map[string]struct{}{}
	for _, spec := range agentSpecs {
		role := string(spec.Role)
		if role == "" {
			continue
		}
		set[role] = struct{}{}
	}
	roles := make([]string, 0, len(set))
	for role := range set {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	return roles
}
