// Package planner tests planner prompt assembly and request encoding.
package planner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cmtonkinson/governator/internal/bootstrap"
	"github.com/cmtonkinson/governator/internal/config"
)

func TestAssemblePromptHappyPath(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "GOVERNATOR.md"), "governator content")
	writePowerSixDocs(t, root, true)
	writeFile(t, filepath.Join(root, "_governator", "prompts", "_global.md"), "global prompt")

	cfg := config.Config{
		Workers: config.WorkersConfig{
			Commands: config.WorkerCommands{
				Default: []string{"echo", "planner"},
				Roles: map[string][]string{
					"planner": {"echo", "planner-role"},
					"worker":  {"echo", "worker-role"},
				},
			},
		},
		Concurrency: config.ConcurrencyConfig{
			Global:      2,
			DefaultRole: 1,
			Roles: map[string]int{
				"planner": 1,
				"worker":  2,
			},
		},
	}

	prompt, err := AssemblePrompt(root, cfg, RepoState{IsGreenfield: false}, nil)
	if err != nil {
		t.Fatalf("expected prompt, got error: %v", err)
	}

	if !strings.Contains(prompt, "global prompt") {
		t.Fatal("expected prompt to include global prompt content")
	}
	if !strings.Contains(prompt, `"kind": "planner_request"`) {
		t.Fatal("expected prompt to include planner request JSON")
	}

	archIndex := strings.Index(prompt, "Sub-job: architecture baseline")
	gapIndex := strings.Index(prompt, "Sub-job: gap analysis")
	roadmapIndex := strings.Index(prompt, "Sub-job: roadmap decomposition")
	tasksIndex := strings.Index(prompt, "Sub-job: task generation")
	if archIndex == -1 || gapIndex == -1 || roadmapIndex == -1 || tasksIndex == -1 {
		t.Fatal("expected prompt to include all sub-job prompts")
	}
	if !(archIndex < gapIndex && gapIndex < roadmapIndex && roadmapIndex < tasksIndex) {
		t.Fatal("expected sub-job prompts to be ordered")
	}

	again, err := AssemblePrompt(root, cfg, RepoState{IsGreenfield: false}, nil)
	if err != nil {
		t.Fatalf("expected prompt, got error: %v", err)
	}
	if prompt != again {
		t.Fatal("expected prompt assembly to be deterministic")
	}
}

func TestAssemblePromptMissingOptionalDocsWarns(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "GOVERNATOR.md"), "governator content")
	writePowerSixDocs(t, root, false)

	var warnings []string
	prompt, err := AssemblePrompt(root, config.Config{}, RepoState{IsGreenfield: false}, func(message string) {
		warnings = append(warnings, message)
	})
	if err != nil {
		t.Fatalf("expected prompt, got error: %v", err)
	}
	if prompt == "" {
		t.Fatal("expected prompt output")
	}
	if len(warnings) != 3 {
		t.Fatalf("expected 3 warnings, got %d", len(warnings))
	}
	for _, warning := range warnings {
		if !strings.Contains(warning, "missing optional Power Six doc") {
			t.Fatalf("unexpected warning: %s", warning)
		}
	}
}

func TestAssemblePromptSkipsGapAnalysisForGreenfield(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "GOVERNATOR.md"), "governator content")
	writePowerSixDocs(t, root, true)

	prompt, err := AssemblePrompt(root, config.Config{}, RepoState{IsGreenfield: true}, nil)
	if err != nil {
		t.Fatalf("expected prompt, got error: %v", err)
	}
	if strings.Contains(prompt, "Sub-job: gap analysis") {
		t.Fatal("expected gap analysis prompt to be skipped for greenfield repos")
	}
	if !strings.Contains(prompt, `"is_greenfield": true`) {
		t.Fatal("expected planner request to include repo state")
	}
}

func writePowerSixDocs(t *testing.T, root string, includeOptional bool) {
	t.Helper()
	for _, artifact := range bootstrap.Artifacts() {
		if !artifact.Required && !includeOptional {
			continue
		}
		path := filepath.Join(root, "_governator", "docs", artifact.Name)
		writeFile(t, path, "content for "+artifact.Name)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
