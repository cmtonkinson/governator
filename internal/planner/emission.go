// Package planner provides helpers for emitting planning artifacts and task indexes.
package planner

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cmtonkinson/governator/internal/digests"
	"github.com/cmtonkinson/governator/internal/index"
)

const (
	planDirName            = "_governator/plan"
	indexFilePath          = "_governator/task-index.json"
	planDirMode            = 0o755
	planFileMode           = 0o644
	indexSchemaVersion     = 1
	architecturePlanFile   = "architecture-baseline.json"
	gapAnalysisPlanFile    = "gap-analysis.json"
	roadmapPlanFile        = "roadmap.json"
	taskGenerationPlanFile = "tasks.json"
)

// PlanOptions configures planning emission behavior.
type PlanOptions struct {
	MaxAttempts int
}

// PlanResult captures the files written during planning emission.
type PlanResult struct {
	PlanFiles []string
	TaskFiles TaskFileResult
	IndexPath string
}

// EmitPlanFromJSON parses planner output JSON and emits plan artifacts, tasks, and the task index.
func EmitPlanFromJSON(repoRoot string, data []byte, options PlanOptions) (PlanResult, error) {
	output, err := ParsePlannerOutput(data)
	if err != nil {
		return PlanResult{}, err
	}
	return EmitPlan(repoRoot, output, options)
}

// EmitPlan writes planning artifacts, task files, and a task index from planner output.
func EmitPlan(repoRoot string, output PlannerOutput, options PlanOptions) (PlanResult, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return PlanResult{}, errors.New("repo root is required")
	}
	if options.MaxAttempts < 0 {
		return PlanResult{}, errors.New("max attempts must be zero or positive")
	}
	if err := output.Validate(); err != nil {
		return PlanResult{}, err
	}
	output.normalize()

	if err := ensurePlannerInputs(repoRoot); err != nil {
		return PlanResult{}, err
	}

	planFiles, err := writePlanArtifacts(repoRoot, output)
	if err != nil {
		return PlanResult{}, err
	}

	taskResult, err := WriteTaskFiles(repoRoot, output.Tasks.Tasks, TaskFileOptions{Force: true})
	if err != nil {
		return PlanResult{}, err
	}

	digestValues, err := digests.Compute(repoRoot)
	if err != nil {
		return PlanResult{}, err
	}

	indexTasks, err := BuildIndexTasks(output.Tasks.Tasks, IndexTaskOptions{MaxAttempts: options.MaxAttempts})
	if err != nil {
		return PlanResult{}, err
	}

	idx := index.Index{
		SchemaVersion: indexSchemaVersion,
		Digests:       digestValues,
		Tasks:         indexTasks,
	}
	indexPath := filepath.Join(repoRoot, indexFilePath)
	if err := index.Save(indexPath, idx); err != nil {
		return PlanResult{}, err
	}

	return PlanResult{
		PlanFiles: planFiles,
		TaskFiles: taskResult,
		IndexPath: repoRelativePath(repoRoot, indexPath),
	}, nil
}

// ensurePlannerInputs verifies required planner inputs are available on disk.
func ensurePlannerInputs(repoRoot string) error {
	if _, err := loadGovernatorDoc(repoRoot); err != nil {
		return err
	}
	if _, err := loadPowerSixDocs(repoRoot, nil); err != nil {
		return err
	}
	return nil
}

// planArtifact captures a named planning output payload.
type planArtifact struct {
	Name  string
	Value any
}

// writePlanArtifacts writes planning outputs into the plan directory.
func writePlanArtifacts(repoRoot string, output PlannerOutput) ([]string, error) {
	planDir := filepath.Join(repoRoot, planDirName)
	if err := os.MkdirAll(planDir, planDirMode); err != nil {
		return nil, fmt.Errorf("create plan directory %s: %w", planDir, err)
	}

	artifacts := []planArtifact{
		{Name: architecturePlanFile, Value: output.ArchitectureBaseline},
		{Name: roadmapPlanFile, Value: output.Roadmap},
		{Name: taskGenerationPlanFile, Value: output.Tasks},
	}
	if output.GapAnalysis != nil {
		artifacts = append(artifacts, planArtifact{
			Name:  gapAnalysisPlanFile,
			Value: *output.GapAnalysis,
		})
	} else {
		if err := removePlanArtifact(planDir, gapAnalysisPlanFile); err != nil {
			return nil, err
		}
	}

	written := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		path := filepath.Join(planDir, artifact.Name)
		data, err := json.MarshalIndent(artifact.Value, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("encode plan artifact %s: %w", artifact.Name, err)
		}
		if len(data) == 0 || data[len(data)-1] != '\n' {
			data = append(data, '\n')
		}
		if err := os.WriteFile(path, data, planFileMode); err != nil {
			return nil, fmt.Errorf("write plan artifact %s: %w", path, err)
		}
		written = append(written, repoRelativePath(repoRoot, path))
	}

	sort.Strings(written)
	return written, nil
}

// removePlanArtifact deletes an optional planning artifact if it exists.
func removePlanArtifact(planDir string, name string) error {
	path := filepath.Join(planDir, name)
	exists, err := fileExists(path)
	if err != nil {
		return fmt.Errorf("stat plan artifact %s: %w", path, err)
	}
	if !exists {
		return nil
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove plan artifact %s: %w", path, err)
	}
	return nil
}
