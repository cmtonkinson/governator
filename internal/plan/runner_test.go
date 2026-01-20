// Tests for the plan command runner.
package plan

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cmtonkinson/governator/internal/planner"
)

const plannerOutputFixture = `{
  "schema_version": 1,
  "kind": "planner_output",
  "architecture_baseline": {
    "schema_version": 1,
    "kind": "architecture_baseline",
    "mode": "synthesis",
    "summary": "Single CLI binary with a minimal planning pipeline.",
    "sources": [
      "_governator/architecture/context.md",
      "GOVERNATOR.md"
    ]
  },
  "roadmap": {
    "schema_version": 1,
    "kind": "roadmap_decomposition",
    "depth_policy": "epic->task",
    "width_policy": "1-3 days",
    "items": [
      {
        "id": "epic-01",
        "title": "Planner contract and parsing",
        "type": "epic",
        "order": 10
      }
    ]
  },
  "tasks": {
    "schema_version": 1,
    "kind": "task_generation",
    "tasks": [
      {
        "id": "task-11",
        "title": "Implement planner output parser",
        "summary": "Parse planner JSON output into internal task models.",
        "role": "engineer",
        "dependencies": [],
        "order": 20,
        "overlap": [
          "planner",
          "index"
        ],
        "acceptance_criteria": [
          "Parser handles all planner output fields."
        ],
        "tests": [
          "Unit tests cover valid and invalid output."
        ]
      }
    ]
  }
}`

// TestRunHappyPath verifies plan executes bootstrap, planner, and emission.
func TestRunHappyPath(t *testing.T) {
	repoRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GO_WANT_PLANNER_HELPER", "1")

	if err := writeGovernatorDoc(repoRoot); err != nil {
		t.Fatalf("write governator: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	options := Options{
		RepoState:      planner.RepoState{IsGreenfield: false},
		PlannerCommand: helperPlannerCommand(),
		Stdout:         &stdout,
		Stderr:         &stderr,
	}

	result, err := Run(repoRoot, options)
	if err != nil {
		t.Fatalf("run plan: %v", err)
	}
	if !result.BootstrapRan {
		t.Fatal("expected bootstrap to run")
	}
	if result.TaskCount != 1 {
		t.Fatalf("expected 1 task, got %d", result.TaskCount)
	}
	if !strings.Contains(stdout.String(), "bootstrap ok") {
		t.Fatalf("expected bootstrap ok output, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "plan ok tasks=1") {
		t.Fatalf("expected plan summary output, got %q", stdout.String())
	}

	assertFileExists(t, filepath.Join(repoRoot, "_governator", "task-index.json"))
	assertFileExists(t, filepath.Join(repoRoot, "_governator", "plan", "architecture-baseline.json"))
	assertFileExists(t, filepath.Join(repoRoot, "_governator", "plan", "roadmap.json"))
	assertFileExists(t, filepath.Join(repoRoot, "_governator", "plan", "tasks.json"))
	assertFileExists(t, filepath.Join(repoRoot, "_governator", "tasks", "task-11-implement-planner-output-parser.md"))
	assertFileExists(t, filepath.Join(repoRoot, filepath.FromSlash(result.PromptPath)))
}

// TestRunMissingGovernator ensures plan fails when GOVERNATOR.md is absent.
func TestRunMissingGovernator(t *testing.T) {
	repoRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GO_WANT_PLANNER_HELPER", "1")

	options := Options{
		RepoState:      planner.RepoState{IsGreenfield: false},
		PlannerCommand: helperPlannerCommand(),
	}

	_, err := Run(repoRoot, options)
	if err == nil {
		t.Fatal("expected error for missing GOVERNATOR.md")
	}
	if !strings.Contains(err.Error(), "GOVERNATOR.md") {
		t.Fatalf("expected missing GOVERNATOR.md error, got %v", err)
	}
}

// TestRunBootstrapFailure ensures bootstrap errors are surfaced clearly.
func TestRunBootstrapFailure(t *testing.T) {
	repoRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GO_WANT_PLANNER_HELPER", "1")

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

	options := Options{
		RepoState:      planner.RepoState{IsGreenfield: false},
		PlannerCommand: helperPlannerCommand(),
	}

	_, err := Run(repoRoot, options)
	if err == nil {
		t.Fatal("expected bootstrap failure")
	}
	if !strings.Contains(err.Error(), "bootstrap failed") {
		t.Fatalf("expected bootstrap failure message, got %v", err)
	}
}

// TestPlannerHelperProcess emits planner output for plan command tests.
func TestPlannerHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_PLANNER_HELPER") != "1" {
		return
	}
	promptPath, err := helperPromptPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}
	if err := validatePromptOrder(string(prompt)); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}
	fmt.Fprintln(os.Stdout, plannerOutputFixture)
	os.Exit(0)
}

// helperPromptPath extracts the prompt path from helper args.
func helperPromptPath() (string, error) {
	for i, arg := range os.Args {
		if arg == "--" && i+1 < len(os.Args) {
			return os.Args[i+1], nil
		}
	}
	return "", errors.New("missing prompt path")
}

// helperPlannerCommand builds the helper planner command invocation.
func helperPlannerCommand() []string {
	return []string{os.Args[0], "-test.run=TestPlannerHelperProcess", "--", "{task_path}"}
}

// validatePromptOrder ensures planning sub-jobs are ordered serially.
func validatePromptOrder(prompt string) error {
	sequence := []string{
		"## Planning sub-job: architecture-baseline",
		"## Planning sub-job: gap-analysis",
		"## Planning sub-job: roadmap",
		"## Planning sub-job: tasks",
		"Input JSON:",
	}
	last := -1
	for _, token := range sequence {
		next := strings.Index(prompt, token)
		if next == -1 {
			return fmt.Errorf("missing prompt section %q", token)
		}
		if next <= last {
			return fmt.Errorf("prompt section %q out of order", token)
		}
		last = next
	}
	return nil
}

// writeGovernatorDoc creates a minimal GOVERNATOR.md file.
func writeGovernatorDoc(repoRoot string) error {
	path := filepath.Join(repoRoot, "GOVERNATOR.md")
	content := "# Governator\n\nConstraints: none.\n"
	return os.WriteFile(path, []byte(content), 0o644)
}

// assertFileExists fails the test when the file is missing.
func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file %s: %v", path, err)
	}
}
