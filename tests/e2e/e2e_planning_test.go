package test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/run"
	"github.com/cmtonkinson/governator/internal/testrepos"
)

// TestE2EPlanning validates the full planning flow using test worker
func TestE2EPlanning(t *testing.T) {
	// Setup: Create temporary repository
	repo := testrepos.New(t)
	repoRoot := repo.Root
	t.Logf("Test repo at: %s", repoRoot)
	TrackE2ERepo(t, repoRoot)

	// Copy GOVERNATOR.md fixture to repo
	fixtureDir, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatalf("get fixture dir: %v", err)
	}
	governatorMdContent, err := os.ReadFile(filepath.Join(fixtureDir, "GOVERNATOR.md"))
	if err != nil {
		t.Fatalf("read GOVERNATOR.md fixture: %v", err)
	}

	err = os.WriteFile(filepath.Join(repoRoot, "GOVERNATOR.md"), governatorMdContent, 0644)
	if err != nil {
		t.Fatalf("write GOVERNATOR.md: %v", err)
	}

	// Configure test worker
	testWorkerPath, err := filepath.Abs("test-worker.sh")
	if err != nil {
		t.Fatalf("get test worker path: %v", err)
	}

	fixturesPath, err := filepath.Abs("testdata/fixtures/worker-actions-realistic.yaml")
	if err != nil {
		t.Fatalf("get fixtures path: %v", err)
	}

	t.Setenv("GOVERNATOR_TEST_FIXTURES", fixturesPath)

	cfg := &config.Config{
		Workers: config.WorkersConfig{
			Commands: config.WorkerCommands{
				Default: []string{testWorkerPath, "{task_path}"}, // Must use {task_path} not {prompt_path}!
				Roles:   map[string][]string{},
			},
		},
		Concurrency: config.ConcurrencyConfig{
			Global:      1,
			DefaultRole: 1,
			Roles:       map[string]int{},
		},
		Timeouts: config.TimeoutsConfig{
			WorkerSeconds: 30, // Short timeout for tests
		},
		Retries: config.RetriesConfig{
			MaxAttempts: 1, // No retries in tests
		},
		Branches: config.BranchConfig{
			Base: "main",
		},
		ReasoningEffort: config.ReasoningEffortConfig{
			Default: "medium",
			Roles:   map[string]string{},
		},
	}

	// Initialize governator state directory
	err = config.InitFullLayout(repoRoot, config.InitOptions{})
	if err != nil {
		t.Fatalf("init full layout: %v", err)
	}
	if err := config.ApplyRepoMigrations(repoRoot, config.InitOptions{}); err != nil {
		t.Fatalf("apply repo migrations: %v", err)
	}

	// Write config to the expected location
	configPath := filepath.Join(repoRoot, "_governator/_durable-state/config.json")
	configJSON, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	err = os.WriteFile(configPath, configJSON, 0644)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Logf("Wrote config to %s:\n%s", configPath, string(configJSON))

	// Create planning.json configuration
	planningSpec := run.PlanningSpec{
		Version: 2,
		Steps: []run.PlanningStepSpec{
			{
				ID:     "architecture-baseline",
				Name:   "Architecture Baseline",
				Prompt: "_governator/prompts/architecture-baseline.md",
				Role:   "architect",
				Validations: []run.PlanningValidationSpec{
					{Type: "file", Path: "_governator/docs/arch-asr.md"},
					{Type: "file", Path: "_governator/docs/arch-arc42.md"},
					{Type: "file", Path: "_governator/docs/adr/[Aa][Dd][Rr]-*.md"},
				},
			},
			{
				ID:     "gap-analysis",
				Name:   "Gap Analysis",
				Prompt: "_governator/prompts/gap-analysis.md",
				Role:   "default",
				Validations: []run.PlanningValidationSpec{
					{Type: "file", Path: "_governator/docs/gap-decision-ledger.md"},
					{Type: "file", Path: "_governator/docs/gap-register.md"},
					{Type: "file", Path: "_governator/docs/gap-planning-constraints.md"},
				},
			},
			{
				ID:     "project-planning",
				Name:   "Project Planning",
				Prompt: "_governator/prompts/roadmap.md",
				Role:   "planner",
				Validations: []run.PlanningValidationSpec{
					{Type: "file", Path: "_governator/docs/milestones.md"},
					{Type: "file", Path: "_governator/docs/epics.md"},
				},
			},
			{
				ID:     "task-planning",
				Name:   "Task Planning",
				Prompt: "_governator/prompts/task-planning.md",
				Role:   "planner",
				Validations: []run.PlanningValidationSpec{
					{Type: "directory", Path: "_governator/tasks"},
				},
			},
		},
	}

	planningSpecPath := filepath.Join(repoRoot, "_governator/planning.json")
	planningJSON, err := json.MarshalIndent(planningSpec, "", "  ")
	if err != nil {
		t.Fatalf("marshal planning spec: %v", err)
	}
	err = os.WriteFile(planningSpecPath, planningJSON, 0644)
	if err != nil {
		t.Fatalf("write planning spec: %v", err)
	}

	// Create minimal prompt files (test worker matches on prompt content patterns)
	err = os.MkdirAll(filepath.Join(repoRoot, "_governator/prompts"), 0755)
	if err != nil {
		t.Fatalf("create prompts dir: %v", err)
	}

	prompts := map[string]string{
		"architecture-baseline.md": "# Architecture Baseline\n\nCreate the Power Six architectural artifacts including Architecturally Significant Requirements.",
		"gap-analysis.md":          "# Gap Analysis\n\nIdentify missing requirements and unresolved decisions in the architecture.",
		"roadmap.md":               "# Project Planning\n\nCreate roadmap with milestones and epics based on the architecture and gap analysis.",
		"task-planning.md":         "# Task Planning\n\nDecompose epics into executable tasks.",
	}

	for filename, content := range prompts {
		promptPath := filepath.Join(repoRoot, "_governator/prompts", filename)
		err = os.WriteFile(promptPath, []byte(content), 0644)
		if err != nil {
			t.Fatalf("write prompt %s: %v", filename, err)
		}
	}

	// Change to repo directory (workers expect to run from repo root)
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get current dir: %v", err)
	}
	defer func() { _ = os.Chdir(originalDir) }()

	err = os.Chdir(repoRoot)
	if err != nil {
		t.Fatalf("chdir to repo: %v", err)
	}

	// Seed planning index (required for supervisor to run)
	if err := run.SeedPlanningIndex(repoRoot); err != nil {
		t.Fatalf("seed planning index: %v", err)
	}

	// Commit all governator setup files (matching what `governator init` does)
	repo.RunGit(t, "add", "GOVERNATOR.md", "_governator")
	repo.RunGit(t, "commit", "-m", "Add governator configuration")

	// Run unified supervisor
	err = run.RunUnifiedSupervisor(repoRoot, run.UnifiedSupervisorOptions{
		Stdout:       os.Stdout,
		Stderr:       os.Stderr,
		PollInterval: 1 * time.Second, // Slower polling to give worker time to finish
	})

	// Debug: Check what was created even if supervisor failed
	if err != nil {
		t.Logf("Unified supervisor error: %v", err)

		// Give a moment for background processes to finish
		time.Sleep(500 * time.Millisecond)

		// Show directory structure
		t.Logf("Directory structure:")
		filepath.Walk(filepath.Join(repoRoot, "_governator"), func(path string, info os.FileInfo, err error) error {
			if err == nil {
				rel, _ := filepath.Rel(repoRoot, path)
				if info.IsDir() {
					t.Logf("  DIR: %s", rel)
				} else {
					t.Logf("  FILE: %s (%d bytes)", rel, info.Size())
				}
			}
			return nil
		})

		// Look for worker logs
		logFiles, _ := filepath.Glob(filepath.Join(repoRoot, "**/**/stderr.log"))
		t.Logf("Found %d stderr.log files", len(logFiles))
		for _, logFile := range logFiles {
			content, _ := os.ReadFile(logFile)
			t.Logf("Log file %s:\n%s", logFile, string(content))
		}

		// Look for exit.json
		exitFiles, _ := filepath.Glob(filepath.Join(repoRoot, "**/**/exit.json"))
		t.Logf("Found %d exit.json files", len(exitFiles))
		for _, exitFile := range exitFiles {
			content, _ := os.ReadFile(exitFile)
			t.Logf("Exit file %s:\n%s", exitFile, string(content))
		}

		// Manually find files in nested structure
		var dispatchPath, envPath string
		filepath.Walk(repoRoot, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				if info.Name() == "dispatch.sh" {
					dispatchPath = path
				} else if info.Name() == "env" && strings.Contains(path, "planning-architecture-baseline") {
					envPath = path
				}
			}
			return nil
		})

		if dispatchPath != "" {
			content, _ := os.ReadFile(dispatchPath)
			t.Logf("Dispatch wrapper:\n%s", string(content))
		} else {
			t.Logf("No dispatch.sh found")
		}

		if envPath != "" {
			content, _ := os.ReadFile(envPath)
			t.Logf("Environment file:\n%s", string(content))
		} else {
			t.Logf("No env file found")
		}

		// Find and read exit.json and stderr.log
		var exitJsonPath, stderrPath string
		filepath.Walk(repoRoot, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				if info.Name() == "exit.json" {
					exitJsonPath = path
				} else if info.Name() == "stderr.log" && strings.Contains(path, "planning-architecture-baseline") {
					stderrPath = path
				}
			}
			return nil
		})
		if exitJsonPath != "" {
			content, _ := os.ReadFile(exitJsonPath)
			t.Logf("exit.json found at %s:\n%s", exitJsonPath, string(content))
		} else {
			t.Logf("No exit.json found anywhere in repo")
		}
		if stderrPath != "" {
			content, _ := os.ReadFile(stderrPath)
			t.Logf("stderr.log content:\n%s", string(content))
		} else {
			t.Logf("No stderr.log found")
		}

		t.Fatalf("run unified supervisor: %v", err)
	}

	// Verify Step 1: Architecture Baseline outputs
	t.Run("architecture_baseline_outputs", func(t *testing.T) {
		assertFileExists(t, repoRoot, "_governator/docs/arch-asr.md")
		assertFileExists(t, repoRoot, "_governator/docs/arch-arc42.md")
		assertFileExists(t, repoRoot, "_governator/docs/arch-wardley.md")
		assertFileExists(t, repoRoot, "_governator/docs/arch-c4.md")
		assertFileExists(t, repoRoot, "_governator/docs/adr/adr-0001-posix-c-stdlib.md")
		assertFileExists(t, repoRoot, "_governator/docs/adr/adr-0002-timestamp-format.md")

		// Verify content
		asrContent, err := os.ReadFile(filepath.Join(repoRoot, "_governator/docs/arch-asr.md"))
		if err != nil {
			t.Fatalf("read ASR file: %v", err)
		}
		assertContains(t, string(asrContent), "ASR-1")
		assertContains(t, string(asrContent), "Deterministic Ordering")
	})

	// Verify Step 2: Gap Analysis outputs
	t.Run("gap_analysis_outputs", func(t *testing.T) {
		assertFileExists(t, repoRoot, "_governator/docs/gap-register.md")
		assertFileExists(t, repoRoot, "_governator/docs/gap-decision-ledger.md")
		assertFileExists(t, repoRoot, "_governator/docs/gap-planning-constraints.md")

		// Verify content
		gapContent, err := os.ReadFile(filepath.Join(repoRoot, "_governator/docs/gap-register.md"))
		if err != nil {
			t.Fatalf("read gap register: %v", err)
		}
		assertContains(t, string(gapContent), "GR-001")
	})

	// Verify Step 3: Project Planning outputs
	t.Run("project_planning_outputs", func(t *testing.T) {
		assertFileExists(t, repoRoot, "_governator/docs/milestones.md")
		assertFileExists(t, repoRoot, "_governator/docs/epics.md")

		// Verify content
		milestonesContent, err := os.ReadFile(filepath.Join(repoRoot, "_governator/docs/milestones.md"))
		if err != nil {
			t.Fatalf("read milestones: %v", err)
		}
		assertContains(t, string(milestonesContent), "Milestone m1")

		epicsContent, err := os.ReadFile(filepath.Join(repoRoot, "_governator/docs/epics.md"))
		if err != nil {
			t.Fatalf("read epics: %v", err)
		}
		assertContains(t, string(epicsContent), "Epic E1.1")
	})

	// Verify Step 4: Task Planning outputs
	t.Run("task_planning_outputs", func(t *testing.T) {
		assertFileExists(t, repoRoot, "_governator/tasks/001-output-format-contract-architect.md")
		assertFileExists(t, repoRoot, "_governator/tasks/002-metadata-error-policies-architect.md")
		assertFileExists(t, repoRoot, "_governator/tasks/003-repository-layout-build-planner.md")

		// Verify content
		task001, err := os.ReadFile(filepath.Join(repoRoot, "_governator/tasks/001-output-format-contract-architect.md"))
		if err != nil {
			t.Fatalf("read task 001: %v", err)
		}
		assertContains(t, string(task001), "milestone: m1")
		assertContains(t, string(task001), "Define Output Format Contract")
	})

	// Verify task inventory updated the execution index after planning completes.
	t.Run("task_inventory_indexed", func(t *testing.T) {
		indexPath := filepath.Join(repoRoot, "_governator", "_local-state", "index.json")
		idx, err := index.Load(indexPath)
		if err != nil {
			t.Fatalf("load task index: %v", err)
		}

		expectedIDs := []string{
			"001-output-format-contract-architect",
			"002-metadata-error-policies-architect",
			"003-repository-layout-build-planner",
		}
		taskByID := make(map[string]index.Task, len(idx.Tasks))
		for _, task := range idx.Tasks {
			taskByID[task.ID] = task
		}
		for _, id := range expectedIDs {
			if _, ok := taskByID[id]; !ok {
				t.Fatalf("task %s missing from index.json", id)
			}
		}
	})

	t.Logf("E2E planning test completed successfully in %s", repoRoot)
}

// assertFileExists is a test helper to verify a file exists
func assertFileExists(t *testing.T, repoDir, relativePath string) {
	t.Helper()
	fullPath := filepath.Join(repoDir, relativePath)
	_, err := os.Stat(fullPath)
	if err != nil {
		t.Fatalf("expected file to exist: %s (error: %v)", relativePath, err)
	}
}

// assertContains is a test helper to verify string contains substring
func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if len(haystack) == 0 {
		t.Fatalf("haystack is empty")
	}
	if len(needle) == 0 {
		t.Fatalf("needle is empty")
	}
	if !containsSubstring(haystack, needle) {
		t.Fatalf("expected string to contain %q, but it doesn't. String: %q", needle, truncate(haystack, 200))
	}
}

// containsSubstring checks if haystack contains needle
func containsSubstring(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// truncate limits string length for error messages
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
